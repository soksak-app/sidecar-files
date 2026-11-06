import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = fileURLToPath(new URL("../", import.meta.url));
const sourcePath = join(root, "src/files/text.go");
const require = createRequire(import.meta.url);

function goEnvironment() {
  const env = { ...process.env };
  if (process.platform === "darwin") {
    const native = dirname(require.resolve("@soksak/native-process-identity-darwin/package.json"));
    env.PKG_CONFIG_PATH = [join(native, "lib/pkgconfig"), env.PKG_CONFIG_PATH].filter(Boolean).join(":");
  }
  return env;
}

function runCase(pattern, overlay) {
  const args = ["test", "-p", "2", "-count=1", "-v", "-run", pattern];
  if (overlay) args.push("-overlay", overlay);
  args.push("./tests");
  const result = spawnSync("go", args, { cwd: root, env: goEnvironment(), encoding: "utf8", timeout: 120000 });
  assert.ifError(result.error);
  assert.equal(result.signal, null, `Go test received ${result.signal}`);
  return { status: result.status, output: result.stdout + result.stderr };
}

const mutations = [
  {
    name: "a partial WriteAt error is discarded",
    from: "written, err := file.WriteAt(content, 0)",
    to: "written, err := file.WriteAt(content, 0)\n\terr = nil",
    test: "TestWriteOpenedFileFailures/partial_write",
    assertion: "failed file operation returned no error",
  },
  {
    name: "a Truncate error is discarded",
    from: "err = file.Truncate(int64(written))",
    to: "file.Truncate(int64(written))",
    test: "TestWriteOpenedFileFailures/truncate",
    assertion: "failed file operation returned no error",
  },
  {
    name: "a Sync error is discarded",
    from: "err = file.Sync()",
    to: "file.Sync()",
    test: "TestWriteOpenedFileFailures/sync",
    assertion: "failed file operation returned no error",
  },
  {
    name: "a Close error is discarded",
    from: "err = errors.Join(err, file.Close())",
    to: "file.Close()",
    test: "TestWriteOpenedFileFailures/close",
    assertion: "failed file operation returned no error",
  },
  {
    name: "a partial write loses the incomplete error",
    from: "if err != nil && written > 0 {",
    to: "if false {",
    test: "TestWriteOpenedFileFailures/partial_write",
    assertion: "want prefix",
  },
  {
    name: "a failed write returns a successful version",
    from: 'return "", fmt.Errorf("write incomplete: %s: %w", path, err)',
    to: 'return version(content), fmt.Errorf("write incomplete: %s: %w", path, err)',
    test: "TestWriteOpenedFileFailures/partial_write",
    assertion: "failed file operation returned version",
  },
  {
    name: "a failed write does not close its file",
    from: "err = errors.Join(err, file.Close())",
    to: "// The close operation is omitted.",
    test: "TestWriteOpenedFileFailures/partial_write",
    assertion: "operations = [write:0], want [write:0 close]",
  },
  {
    name: "a successful shorter write does not truncate its file",
    from: "err = file.Truncate(int64(written))",
    to: "err = nil",
    test: "TestWriteOpenedFileCompletesBeforeReturningAVersion/length_3",
    assertion: "operations = [write:0 sync close], want [write:0 truncate:3 sync close]",
  },
  {
    name: "a close error replaces the original write error",
    from: "err = errors.Join(err, file.Close())",
    to: "err = file.Close()",
    test: "TestWriteOpenedFileFailures/partial_write_and_close",
    assertion: "lost cause injected operation failure",
  },
  {
    name: "a write changes the actual bytes without changing its result",
    from: "file.WriteAt(content, 0)",
    to: "file.WriteAt(make([]byte, len(content)), 0)",
    test: "TestWriteOpenedFileCompletesBeforeReturningAVersion/length_3",
    assertion: 'want "NEW"',
  },
];

test("the file-operation assertions reject each omitted failure contract", async (t) => {
  const source = await readFile(sourcePath, "utf8");
  let baselinePassed = false;
  await t.test("the unchanged writer passes every file-operation case", { timeout: 130000 }, () => {
    const result = runCase("^TestWriteOpenedFile");
    assert.equal(result.status, 0, result.output);
    assert.match(result.output, /--- PASS: TestWriteOpenedFileFailures/);
    assert.match(result.output, /--- PASS: TestWriteOpenedFileCompletesBeforeReturningAVersion/);
    baselinePassed = true;
  });
  // A failed baseline is not evidence that a mutation was detected.
  if (!baselinePassed) return;
  for (const mutation of mutations) {
    await t.test(mutation.name, { timeout: 130000 }, async (t) => {
      assert.equal(source.split(mutation.from).length - 1, 1, `${mutation.name}: expected one mutation site`);
      const directory = await mkdtemp(join(tmpdir(), "files-write-failure-"));
      t.after(() => rm(directory, { recursive: true, force: true }));
      const replacement = join(directory, "text.go");
      const overlay = join(directory, "overlay.json");
      await writeFile(replacement, source.replace(mutation.from, mutation.to));
      await writeFile(overlay, JSON.stringify({ Replace: { [sourcePath]: replacement } }));
      const pattern = mutation.test.split("/").map((name) => `^${name}$`).join("/");
      const result = runCase(pattern, overlay);
      assert.equal(result.status, 1, result.output);
      assert.ok(result.output.includes(`--- FAIL: ${mutation.test} `), result.output);
      assert.ok(result.output.includes(mutation.assertion), result.output);
      t.diagnostic(result.output.trim());
    });
  }
});
