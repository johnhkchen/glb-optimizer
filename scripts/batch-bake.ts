import { chromium } from "playwright";
import { readdirSync, mkdirSync, renameSync, existsSync } from "node:fs";
import { resolve, join, basename } from "node:path";
import {
  bakeOne,
  BakeError,
  type BakeResult,
  captureErrorScreenshot,
  formatBytes,
  log,
  setJsonMode,
} from "./headless-bake.ts";

const DEFAULT_PORT = 8787;
const PREFIX = "[batch-bake]";

let batchJsonMode = false;

function batchLog(msg: string) {
  if (batchJsonMode) {
    process.stderr.write(`${PREFIX} ${msg}\n`);
  } else {
    console.log(`${PREFIX} ${msg}`);
  }
}

interface ParsedArgs {
  inboxDir: string;
  port: number;
  headless: boolean;
  json: boolean;
}

function parseArgs(argv: string[]): ParsedArgs {
  const args = argv.slice(2);
  let inboxDir = "";
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
      inboxDir = args[i];
    }
  }

  if (!inboxDir) {
    inboxDir = "../inbox";
  }

  inboxDir = resolve(inboxDir);

  if (!existsSync(inboxDir)) {
    console.error(`${PREFIX} inbox directory not found: ${inboxDir}`);
    process.exit(1);
  }

  return { inboxDir, port, headless, json };
}

interface FileResult {
  filename: string;
  species: string;
  size: number;
  status: "ok" | "error";
  error?: string;
}

async function main() {
  const { inboxDir, port, headless, json } = parseArgs(process.argv);
  if (json) {
    batchJsonMode = true;
    setJsonMode(true);
  }
  const baseUrl = `http://localhost:${port}`;
  const doneDir = join(inboxDir, "done");

  // Scan for .glb files (skip directories)
  const entries = readdirSync(inboxDir, { withFileTypes: true });
  const glbFiles = entries
    .filter((e) => !e.isDirectory() && e.name.endsWith(".glb"))
    .map((e) => e.name)
    .sort();

  if (glbFiles.length === 0) {
    batchLog(`no .glb files in ${inboxDir}`);
    process.exit(0);
  }

  batchLog(`found ${glbFiles.length} .glb file(s) in ${inboxDir}`);
  batchLog(`server: ${baseUrl}`);
  batchLog(`mode: ${headless ? "headless" : "headed"}`);

  // Check server is reachable
  try {
    const res = await fetch(`${baseUrl}/api/status`);
    if (!res.ok) throw new Error(`status ${res.status}`);
  } catch {
    console.error(
      `${PREFIX} server not reachable at ${baseUrl}\n` +
        `Start the server with: just run`
    );
    process.exit(1);
  }
  batchLog("server reachable");

  const browser = await chromium.launch({ headless });
  const page = await browser.newPage();
  const results: FileResult[] = [];

  for (let i = 0; i < glbFiles.length; i++) {
    const filename = glbFiles[i];
    const filePath = join(inboxDir, filename);
    batchLog(`[${i + 1}/${glbFiles.length}] processing ${filename}...`);

    try {
      const result = await bakeOne(page, filePath, baseUrl);
      results.push({
        filename,
        species: result.species,
        size: result.size,
        status: "ok",
      });

      if (json) {
        console.log(JSON.stringify({
          source: filename,
          species: result.species,
          pack: result.packPath,
          size: result.size,
          status: "ok",
        }));
      }

      // Move to done/
      mkdirSync(doneDir, { recursive: true });
      renameSync(filePath, join(doneDir, filename));
      batchLog(`moved ${filename} → inbox/done/`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      const step = err instanceof BakeError ? err.step : "unknown";
      const screenshot = err instanceof BakeError ? err.screenshot : undefined;

      if (json) {
        console.log(JSON.stringify({
          source: filename,
          error: msg,
          step,
          ...(screenshot ? { screenshot } : {}),
          status: "error",
        }));
      } else {
        console.error(`${PREFIX} ERROR processing ${filename}: ${msg}`);
      }

      results.push({
        filename,
        species: "—",
        size: 0,
        status: "error",
        error: msg,
      });
    }
  }

  await browser.close();

  const okCount = results.filter((r) => r.status === "ok").length;
  const failCount = results.length - okCount;

  if (!json) {
    // Print summary table
    console.log("");
    batchLog("=== Summary ===");
    const colFile = 30;
    const colSpecies = 20;
    const colSize = 12;
    console.log(
      "FILENAME".padEnd(colFile) +
        "SPECIES".padEnd(colSpecies) +
        "SIZE".padEnd(colSize) +
        "STATUS"
    );
    console.log("-".repeat(colFile + colSpecies + colSize + 10));

    for (const r of results) {
      const sizeStr = r.status === "ok" ? formatBytes(r.size) : "—";
      const statusStr =
        r.status === "ok" ? "ok" : `ERROR: ${r.error ?? "unknown"}`;
      console.log(
        r.filename.padEnd(colFile) +
          r.species.padEnd(colSpecies) +
          sizeStr.padEnd(colSize) +
          statusStr
      );
    }
    console.log("");
  }

  batchLog(`${okCount} succeeded, ${failCount} failed out of ${results.length} total`);

  if (failCount > 0) {
    process.exit(1);
  }
}

main();
