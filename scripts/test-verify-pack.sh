#!/usr/bin/env bash
# test-verify-pack.sh — Shell harness for scripts/verify-pack.mjs.
# Asserts the verifier passes a known-good pack and rejects a
# pack missing view_top with a useful message. Wired into
# `just verify-pack-test`.

set -u
cd "$(dirname "$0")/.."

fail=0

if ! node scripts/verify-pack.mjs scripts/fixtures/valid-pack.glb >/dev/null; then
  echo "FAIL: valid-pack rejected by verifier"
  fail=1
fi

out=$(node scripts/verify-pack.mjs scripts/fixtures/broken-pack-no-top.glb 2>&1)
rc=$?
if [ "$rc" -eq 0 ]; then
  echo "FAIL: broken-pack accepted by verifier (exit 0)"
  echo "$out"
  fail=1
fi
if ! echo "$out" | grep -q view_top; then
  echo "FAIL: broken-pack output missing view_top mention"
  echo "$out"
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "PASS: verify-pack tests"
fi
exit "$fail"
