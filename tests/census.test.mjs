// Checks that the census and the catalog guard of soksak-events (core docs/spec/logging.md#census) pass for this repository and
// fail for a source that has neither an event nor a reason.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const root = new URL("../", import.meta.url).pathname;
const command = join(root, "node_modules/.bin/soksak-events");

/** Runs the guard on the repository at `repository` with this repository's declarations and returns its status and standard error. */
function guard(repository) {
  const result = spawnSync(command, ["--census", "census.json", "--catalog", "events.json", repository], { encoding: "utf8" });
  return { status: result.status, stderr: result.stderr, stdout: result.stdout };
}

/** A tracked copy of the declarations with one extra Go source. */
function repositoryWith(source) {
  const directory = mkdtempSync(join(tmpdir(), "files-census-"));
  for (const file of ["census.json", "events.json"]) copyFileSync(join(root, file), join(directory, file));
  mkdirSync(join(directory, "src"));
  writeFileSync(join(directory, "src/extra.go"), source);
  execFileSync("git", ["init", "-q"], { cwd: directory });
  execFileSync("git", ["add", "."], { cwd: directory });
  return directory;
}

test("the census and the catalog guard pass for this repository", () => {
  const result = guard(root);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Census passed/);
  assert.match(result.stdout, /Event catalog checks passed/);
});

test("a goroutine outside the entrance files fails the census", () => {
  const result = guard(repositoryWith("package x\n\nfunc start() {\n\tgo func() {}()\n}\n"));
  assert.equal(result.status, 1);
  assert.match(result.stderr, /src\/extra\.go:4: go\.go-statement is outside its entrance files/);
});

test("a recovered panic outside the entrance files fails the census", () => {
  const result = guard(repositoryWith("package x\n\nfunc keep() {\n\tdefer func() { recover() }()\n}\n"));
  assert.equal(result.status, 1);
  assert.match(result.stderr, /go\.recover is outside its entrance files/);
});

test("an event that the catalog does not declare fails the catalog guard", () => {
  const result = guard(repositoryWith('package x\n\nfunc emit(log *Log) {\n\tlog.EmitHost(LevelInfo, "sidecar.files.undeclared", nil)\n}\n'));
  assert.equal(result.status, 1);
  assert.match(result.stderr, /sidecar\.files\.undeclared is emitted and events\.json does not declare it/);
});

test("a reason comment in another form fails the census", () => {
  const result = guard(repositoryWith("package x\n\n// 기본값: 비어 있으면 빈 목록이다.\nvar empty []string\n"));
  assert.equal(result.status, 1);
  assert.match(result.stderr, /src\/extra\.go:3: reason marker/);
});
