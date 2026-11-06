#!/usr/bin/env node
// Runs a command and fails it when the host linker reports an object built for a newer macOS than the one it links for (S57).
// The linker only warns, so without this check the build passes and the executable holds code compiled for a newer macOS than
// it declares. Each output line passes through unchanged; the status is that of the command, or 1 when a command that
// succeeded printed the warning.
//
//   node scripts/check-link.mjs go build -o build/sok-files ./src
import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

const NEWER = "was built for newer";

async function run([command, ...args]) {
  if (!command) {
    process.stderr.write("usage: node scripts/check-link.mjs <command> [arguments]\n");
    return 2;
  }
  const child = spawn(command, args, { stdio: ["inherit", "pipe", "pipe"] });
  let newer = 0;
  const outputs = [[child.stdout, process.stdout], [child.stderr, process.stderr]].map(([input, output]) => new Promise((resolve) => {
    const lines = createInterface({ input, crlfDelay: Infinity });
    lines.on("line", (line) => {
      if (line.includes(NEWER)) newer += 1;
      output.write(`${line}\n`);
    });
    lines.on("close", resolve);
  }));
  const status = await new Promise((resolve, reject) => {
    child.on("error", reject);
    child.on("close", (code, signal) => resolve(signal ? 1 : code));
  });
  await Promise.all(outputs);
  if (newer > 0) {
    process.stderr.write(`check-link: the linker linked ${newer} object${newer === 1 ? "" : "s"} built for a newer macOS than the executable declares\n`);
    return status === 0 ? 1 : status;
  }
  return status;
}

run(process.argv.slice(2)).then((status) => {
  process.exitCode = status;
}, (error) => {
  process.stderr.write(`check-link: ${error.message}\n`);
  process.exitCode = 1;
});
