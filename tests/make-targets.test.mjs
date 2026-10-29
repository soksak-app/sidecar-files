// The push-check target runs the checks that a push needs, and the pre-push hook runs push-check on the pushed range (S38).
import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

// Prints the recipe of a target without running it; the recipe lines name every command the target runs.
// MAKEFLAGS and RANGE come from an outer make (the pre-push hook passes RANGE), so they are removed to print the default range.
function recipe(target) {
  const env = { ...process.env };
  delete env.MAKEFLAGS;
  delete env.RANGE;
  const result = spawnSync("make", ["-n", target], { cwd: root, encoding: "utf8", timeout: 30000, env });
  assert.equal(result.status, 0, `make -n ${target} failed: ${result.stderr}`);
  return result.stdout;
}

// A published commit message is kept, so the push checks it; CI runs the tests on the pushed commits.
test("make push-check runs only the commit message check of the pushed range (F274-files-sidecar)", { timeout: 30000 }, () => {
  const commands = recipe("push-check");
  assert.deepEqual(commands.trim().split("\n"), ['pnpm exec soksak-commits "origin/main..HEAD"']);
});

test("the pre-push hook runs make push-check on the pushed range (S38)", { timeout: 30000 }, () => {
  const hook = readFileSync(path.join(root, ".githooks", "pre-push"), "utf8");
  assert.match(hook, /make -C "\$root" push-check RANGE="\$base\.\.\$local_sha"/);
});
