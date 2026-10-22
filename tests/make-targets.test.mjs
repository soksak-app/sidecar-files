// The push-check target runs the checks that a push needs, and the pre-push hook runs push-check on the pushed range (S38).
import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

// Prints the recipe of a target without running it; the recipe lines name every command the target runs.
function recipe(target) {
  const result = spawnSync("make", ["-n", target], { cwd: root, encoding: "utf8", timeout: 30000 });
  assert.equal(result.status, 0, `make -n ${target} failed: ${result.stderr}`);
  return result.stdout;
}

test("make push-check runs the Go and Node tests and the commit message check of the pushed range (S38)", { timeout: 30000 }, () => {
  const commands = recipe("push-check");
  assert.match(commands, /go test \.\/\.\.\./);
  assert.match(commands, /pnpm exec soksak-commits "origin\/main\.\.HEAD"/);
});

test("the pre-push hook runs make push-check on the pushed range (S38)", { timeout: 30000 }, () => {
  const hook = readFileSync(path.join(root, ".githooks", "pre-push"), "utf8");
  assert.match(hook, /make -C "\$root" push-check RANGE="\$base\.\.\$local_sha"/);
});
