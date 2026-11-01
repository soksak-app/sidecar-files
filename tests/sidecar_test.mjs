import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path) => JSON.parse(readFileSync(new URL(`../${path}`, import.meta.url), "utf8"));

// The application validates the format of sidecar.json when it installs the release (core docs/spec/installation.md); this
// test checks the values that this repository declares.
test("sidecar.json declares the message form, the address and the events of the sidecar, and the package publishes its executable", { timeout: 5000 }, () => {
  const declared = read("sidecar.json");
  const sidecar = declared;
  assert.deepEqual(sidecar.message, { form: "json-object", operationField: "operation" });
  // Core refuses hostReadOperations, which only its removed plugin host read.
  assert.equal(Object.hasOwn(declared, "hostReadOperations"), false);
  assert.equal(sidecar.displayName, "soksak://sidecar/files");
  const pkg = read("package.json");
  assert.ok(pkg.files.includes("sidecar.json"));
  assert.ok(pkg.files.includes(sidecar.executable));
  // The release packs the event catalog that the manifest names.
  assert.equal(sidecar.events, "events.json");
  assert.ok(pkg.files.includes(sidecar.events));
  // build 는 Makefile 의 build 목표가 sidecar.json 이 가리키는 실행 파일로 쓴다.
  const makefile = readFileSync(new URL("../Makefile", import.meta.url), "utf8");
  assert.match(makefile, new RegExp(`-o ${sidecar.executable} `));
});
