import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { validateSidecar } from "@soksak/plugin-api";

const read = (path) => JSON.parse(readFileSync(new URL(`../${path}`, import.meta.url), "utf8"));

test("sidecar.json satisfies the sidecar format and the package publishes its executable", () => {
  const sidecar = validateSidecar(read("sidecar.json"));
  const pkg = read("package.json");
  assert.ok(pkg.files.includes("sidecar.json"));
  assert.ok(pkg.files.includes(sidecar.executable));
  // build 는 Makefile 의 build 목표가 sidecar.json 이 가리키는 실행 파일로 쓴다.
  const makefile = readFileSync(new URL("../Makefile", import.meta.url), "utf8");
  assert.match(makefile, new RegExp(`-o ${sidecar.executable} `));
});
