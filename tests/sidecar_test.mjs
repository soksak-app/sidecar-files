import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { validateSidecar } from "@soksak/plugin-api";

const read = (path) => JSON.parse(readFileSync(new URL(`../${path}`, import.meta.url), "utf8"));

test("sidecar.json satisfies the sidecar format and the package publishes its executable", () => {
  const declared = read("sidecar.json");
  const { message, hostReadOperations, displayName, ...legacy } = declared;
  const sidecar = validateSidecar(legacy);
  // v0.0.11 predates the F238-d-16 host fields; the host route and next SDK tag own their full validation.
  assert.deepEqual(message, { form: "json-object", operationField: "operation" });
  assert.deepEqual(hostReadOperations, ["snapshot", "expand", "collapse", "select"]);
  assert.equal(displayName, "soksak://sidecar/files");
  assert.ok(hostReadOperations.every((operation) => /^[a-z][a-z0-9]*$/.test(operation)));
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
