// The check-push target runs the checks that a push needs, and the pre-push hook runs check-push on the pushed range (S38).
import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

// Prints the recipe of a target without running it; the recipe lines name every command the target runs.
// MAKEFLAGS and RANGE come from an outer make (the pre-push hook passes RANGE), so they are removed to print the default range.
// A make run inside another make prints its directory, so the recipe is printed without it.
function recipe(target) {
  const env = { ...process.env };
  delete env.MAKEFLAGS;
  delete env.RANGE;
  const result = spawnSync("make", ["-n", "--no-print-directory", target], { cwd: root, encoding: "utf8", timeout: 30000, env });
  assert.equal(result.status, 0, `make -n ${target} failed: ${result.stderr}`);
  return result.stdout;
}

// A published commit is kept, so the push checks its message and the checklist history it records; CI runs the tests on
// the pushed commits.
test("make check-push runs the commit message check and the checklist history check of the pushed range (S53)", { timeout: 30000 }, () => {
  const commands = recipe("check-push");
  assert.deepEqual(commands.trim().split("\n"), [
    'pnpm exec sok-check commits "origin/main..HEAD"',
    'pnpm exec sok-check docs --range "origin/main..HEAD"',
  ]);
});

test("the pre-push hook runs make check-push on the pushed range (S38)", { timeout: 30000 }, () => {
  const hook = readFileSync(path.join(root, ".githooks", "pre-push"), "utf8");
  assert.match(hook, /make -C "\$root" check-push RANGE="\$base\.\.\$local_sha"/);
});

// The installation stage is make install (core docs/spec/plugins.md#repositories); the build and the tests consume what it installed.
test("make install installs the locked dependencies, and no other target installs (F275-files-sidecar)", { timeout: 30000 }, () => {
  assert.deepEqual(recipe("install").trim().split("\n"), ["pnpm install --frozen-lockfile"]);
  const makefile = readFileSync(path.join(root, "Makefile"), "utf8");
  assert.deepEqual(makefile.split("\n").filter((line) => /^[^\t#][^:=]*:(?!=).*\bnode_modules\b/.test(line)), [], "a target depends on node_modules");
});
