// scripts/check-link.mjs fails a command whose linker reports an object built for a newer macOS than it links for (S57).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const warning = "ld: warning: object file (/tmp/go-link-1/000002.o) was built for newer 'macOS' version (27.0) than being linked (14.4)";

// Runs the check on a node program that writes to its outputs and ends with the given status.
function check(program) {
  return spawnSync("node", ["scripts/check-link.mjs", process.execPath, "-e", program], { cwd: root, encoding: "utf8", timeout: 10000 });
}

test("a linker warning of a newer object fails a command that succeeds, and the warning stays in the output", { timeout: 15000 }, () => {
  const result = check(`console.log("# github.com/soksak-app/sidecar-files/src"); console.error(${JSON.stringify(warning)});`);
  assert.equal(result.status, 1, `${result.stdout}\n${result.stderr}`);
  assert.equal(result.stdout, "# github.com/soksak-app/sidecar-files/src\n");
  assert.ok(result.stderr.startsWith(`${warning}\n`), result.stderr);
  assert.match(result.stderr, /check-link: the linker linked 1 object built for a newer macOS than the executable declares/);
});

test("a command without the warning keeps its output and its status", { timeout: 15000 }, () => {
  const passed = check(`console.log("ok"); console.error("ld: warning: other");`);
  assert.equal(passed.status, 0, passed.stderr);
  assert.equal(passed.stdout, "ok\n");
  assert.equal(passed.stderr, "ld: warning: other\n");
  const failed = check(`console.error("FAIL"); process.exit(3);`);
  assert.equal(failed.status, 3, failed.stderr);
  assert.equal(failed.stderr, "FAIL\n");
});

test("a command that fails with the warning fails with its own status", { timeout: 15000 }, () => {
  const result = check(`console.error(${JSON.stringify(warning)}); process.exit(2);`);
  assert.equal(result.status, 2, result.stderr);
  assert.match(result.stderr, /check-link: the linker linked 1 object built for a newer macOS/);
});
