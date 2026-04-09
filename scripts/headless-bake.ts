import { chromium, type Page } from "playwright";
import { existsSync, mkdirSync } from "node:fs";
import { resolve, basename } from "node:path";

const DEFAULT_PORT = 8787;
const BAKE_TIMEOUT_MS = 900_000; // 15 minutes — large models in headless Chromium are slow
const POLL_INTERVAL_MS = 2_000;
const PREFIX = "[headless-bake]";

let jsonMode = false;

export function setJsonMode(flag: boolean) {
  jsonMode = flag;
}

export function log(msg: string) {
  if (jsonMode) {
    process.stderr.write(`${PREFIX} ${msg}\n`);
  } else {
    console.log(`${PREFIX} ${msg}`);
  }
}

interface ParsedArgs {
  glbPath: string;
  port: number;
  headless: boolean;
  json: boolean;
}

function parseArgs(argv: string[]): ParsedArgs {
  const args = argv.slice(2); // skip node + script
  let glbPath = "";
  let port = DEFAULT_PORT;
  let headless = false;
  let json = false;

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--port" && i + 1 < args.length) {
      port = parseInt(args[++i], 10);
      if (isNaN(port)) throw new Error(`Invalid port: ${args[i]}`);
    } else if (args[i] === "--headless") {
      headless = true;
    } else if (args[i] === "--json") {
      json = true;
    } else if (!args[i].startsWith("--")) {
      glbPath = args[i];
    }
  }

  if (!glbPath) {
    console.error(
      `Usage: npx tsx headless-bake.ts <source.glb> [--port ${DEFAULT_PORT}] [--headless] [--json]`
    );
    process.exit(1);
  }

  glbPath = resolve(glbPath);
  if (!existsSync(glbPath)) {
    console.error(`${PREFIX} file not found: ${glbPath}`);
    process.exit(1);
  }

  return { glbPath, port, headless, json };
}

async function checkServer(baseUrl: string): Promise<void> {
  try {
    const res = await fetch(`${baseUrl}/api/status`);
    if (!res.ok) throw new Error(`status ${res.status}`);
  } catch {
    console.error(
      `${PREFIX} server not reachable at ${baseUrl}\n` +
        `Start the server with: go run . (or just run)`
    );
    process.exit(1);
  }
}

export async function captureErrorScreenshot(
  page: Page,
  filename: string
): Promise<string> {
  const dir = resolve("dist", "bake-errors");
  mkdirSync(dir, { recursive: true });
  const ts = new Date().toISOString().replace(/[:.]/g, "-");
  const stem = filename.replace(/\.glb$/i, "");
  const outPath = resolve(dir, `${stem}-${ts}.png`);
  await page.screenshot({ path: outPath, fullPage: true });
  log(`screenshot saved: ${outPath}`);
  return outPath;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

export interface BakeResult {
  filename: string;
  species: string;
  size: number;
  packPath: string;
}

export class BakeError extends Error {
  step: string;
  screenshot?: string;
  constructor(message: string, step: string, screenshot?: string) {
    super(message);
    this.name = "BakeError";
    this.step = step;
    this.screenshot = screenshot;
  }
}

/**
 * Bake a single GLB file using the web UI. The caller owns the browser
 * and page lifecycle — this function navigates, uploads, bakes, packs,
 * and returns the result. Throws on any failure.
 */
export async function bakeOne(
  page: Page,
  glbPath: string,
  baseUrl: string
): Promise<BakeResult> {
  const filename = basename(glbPath);
  let currentStep = "upload";

  try {
    // Navigate to UI (resets state for batch reuse)
    await page.goto(baseUrl, { waitUntil: "networkidle" });
    log("UI loaded");

    // Upload the GLB via the hidden file input
    log(`uploading ${filename}...`);
    const uploadResponsePromise = page.waitForResponse(
      (r) => r.url().includes("/api/upload") && r.request().method() === "POST"
    );
    await page.setInputFiles("#fileInput", glbPath);
    const uploadResponse = await uploadResponsePromise;

    if (!uploadResponse.ok()) {
      throw new Error(
        `upload failed: ${uploadResponse.status()} ${await uploadResponse.text()}`
      );
    }

    const uploadedFiles = await uploadResponse.json();
    if (!Array.isArray(uploadedFiles) || uploadedFiles.length === 0) {
      throw new Error("upload returned no files");
    }
    const fileId = uploadedFiles[0].id;
    log(`file uploaded: id=${fileId}`);

    // Click the file card to select it
    currentStep = "select";
    await page.click(`.file-item:has(.filename[title="${filename}"])`);
    // Wait for the 3D model to load (prepareForSceneBtn becomes enabled)
    // OR the classification comparison modal to appear (low-confidence
    // shape → user must pick a strategy before the pipeline can run).
    await page.waitForFunction(
      () => {
        const btn = document.getElementById(
          "prepareForSceneBtn"
        ) as HTMLButtonElement | null;
        const modal = document.getElementById("comparisonModal");
        const modalVisible =
          modal && modal.style.display !== "none";
        return (btn && !btn.disabled) || modalVisible;
      },
      { timeout: 60_000 }
    );

    // Auto-dismiss the classification modal if it appeared. Force
    // "round-bush" via pickCandidate(id, category) which POSTs the
    // classification override regardless of which options the modal
    // offered. This is the right default for organic plant models
    // (dahlias, yarrow, coffeeberry, etc.).
    const modalVisible = await page.evaluate(() => {
      const modal = document.getElementById("comparisonModal");
      return modal ? modal.style.display !== "none" : false;
    });
    if (modalVisible) {
      log("classification modal detected — forcing round-bush via API");
      // POST the override directly to the Go server, bypassing the
      // modal UI. The endpoint stamps the settings and closes the
      // classification flow server-side.
      const overrideUrl = `${baseUrl}/api/classify/${fileId}?override=round-bush`;
      const overrideResp = await page.evaluate(async (url) => {
        const r = await fetch(url, { method: "POST" });
        return { ok: r.ok, status: r.status, text: await r.text() };
      }, overrideUrl);
      if (!overrideResp.ok) {
        throw new Error(
          `classification override failed: ${overrideResp.status} ${overrideResp.text}`
        );
      }
      log("round-bush classification applied via API");
      // Close the modal from the UI side + refresh settings display
      await page.evaluate(() => {
        const modal = document.getElementById("comparisonModal");
        if (modal) modal.style.display = "none";
      });
      // Re-select the file to trigger a full UI refresh with the new
      // classification. This re-loads the model and eventually enables
      // prepareForSceneBtn.
      await page.click(`.file-item:has(.filename[title="${filename}"])`);
      log("round-bush applied, re-selected file");
    }
    // (Re-)wait for the Prepare for Scene button to become enabled.
    // After classification override the model reloads, which can take
    // a while for large GLBs (28 MB dahlia → gltfpack re-processes).
    await page.waitForFunction(
      () => {
        const btn = document.getElementById(
          "prepareForSceneBtn"
        ) as HTMLButtonElement | null;
        return btn && !btn.disabled;
      },
      { timeout: 120_000 }
    );
    log("file selected, model loaded");

    // Click "Prepare for Scene" to run the full pipeline
    currentStep = "pipeline";
    log("starting pipeline...");
    await page.click("#prepareForSceneBtn");

    // Monitor stage completion
    const stageLabels = ["Optimize", "Classify", "LOD", "Production asset"];
    const stageCount = stageLabels.length;

    await page.waitForFunction(
      (timeout: number) => {
        const stages = document.querySelectorAll("#prepareStages li");
        if (stages.length === 0) return false;
        for (const li of stages) {
          if (li.classList.contains("error")) return true;
        }
        const allDone = Array.from(stages).every(
          (li) => li.classList.contains("ok")
        );
        return allDone;
      },
      BAKE_TIMEOUT_MS,
      { timeout: BAKE_TIMEOUT_MS }
    );

    // Check if any stage failed
    const failedStage = await page.$eval(
      "#prepareStages",
      (ul) => {
        const errLi = ul.querySelector("li.error");
        return errLi ? errLi.textContent : null;
      }
    );

    if (failedStage) {
      // Try to identify which pipeline stage failed
      const stageName = failedStage.toLowerCase();
      if (stageName.includes("billboard")) currentStep = "billboard";
      else if (stageName.includes("lod")) currentStep = "lod";
      else if (stageName.includes("classify")) currentStep = "classify";
      else if (stageName.includes("optimize")) currentStep = "optimize";
      else currentStep = "pipeline";
      throw new Error(`pipeline stage failed: ${failedStage}`);
    }

    // Log stage results
    const stageResults = await page.$$eval("#prepareStages li", (items) =>
      items.map((li) => li.textContent || "")
    );
    for (let i = 0; i < stageResults.length; i++) {
      log(`stage ${i + 1}/${stageCount}: ${stageResults[i]}`);
    }

    // Build asset pack
    currentStep = "pack";
    log("building pack...");
    const packResponsePromise = page.waitForResponse(
      (r) => r.url().includes("/api/pack/") && r.request().method() === "POST"
    );
    await page.click("#buildPackBtn");
    const packResponse = await packResponsePromise;

    if (!packResponse.ok()) {
      const errorText = await page.$eval(
        "#prepareError",
        (el) => el.textContent || ""
      );
      throw new Error(`pack build failed (${packResponse.status()}): ${errorText}`);
    }

    const packResult = await packResponse.json();
    const { species, size, pack_path } = packResult;

    // Verify pack file exists on disk
    if (!existsSync(pack_path)) {
      throw new Error(`pack file not found at: ${pack_path}`);
    }

    log(`pack built: ${pack_path}`);
    log(`species: ${species}`);
    log(`size: ${formatBytes(size)}`);

    return { filename, species, size, packPath: pack_path };
  } catch (err: unknown) {
    // Wrap non-BakeError exceptions with step info
    if (err instanceof BakeError) throw err;
    const msg = err instanceof Error ? err.message : String(err);
    let screenshot: string | undefined;
    try {
      screenshot = await captureErrorScreenshot(page, filename);
    } catch {
      // screenshot capture failed — continue without it
    }
    throw new BakeError(msg, currentStep, screenshot);
  }
}

async function main() {
  const { glbPath, port, headless, json } = parseArgs(process.argv);
  if (json) setJsonMode(true);
  const baseUrl = `http://localhost:${port}`;
  const source = basename(glbPath);

  log(`source: ${glbPath}`);
  log(`server: ${baseUrl}`);
  log(`mode: ${headless ? "headless" : "headed"}`);

  await checkServer(baseUrl);
  log("server reachable");

  const browser = await chromium.launch({ headless });
  const page = await browser.newPage();

  try {
    const result = await bakeOne(page, glbPath, baseUrl);
    if (json) {
      console.log(JSON.stringify({
        source,
        species: result.species,
        pack: result.packPath,
        size: result.size,
        status: "ok",
      }));
    } else {
      log("done");
    }
  } catch (err: unknown) {
    if (json) {
      const step = err instanceof BakeError ? err.step : "unknown";
      const screenshot = err instanceof BakeError ? err.screenshot : undefined;
      const msg = err instanceof Error ? err.message : String(err);
      console.log(JSON.stringify({
        source,
        error: msg,
        step,
        ...(screenshot ? { screenshot } : {}),
        status: "error",
      }));
    } else {
      const msg = err instanceof Error ? err.message : String(err);
      console.error(`${PREFIX} ERROR: ${msg}`);
    }
    await browser.close();
    process.exit(1);
  }

  await browser.close();
}

// Only run when this file is the entry point (not when imported by batch-bake.ts)
const isMain =
  process.argv[1] &&
  resolve(process.argv[1]).replace(/\.ts$/, "") ===
    new URL(import.meta.url).pathname.replace(/\.ts$/, "");
if (isMain) {
  main();
}
