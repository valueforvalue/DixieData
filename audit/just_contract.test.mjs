import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

const justfile = new URL("../justfile", import.meta.url);

test("Just exposes Windows debug build contract", async () => {
  await access(justfile);
  const source = await readFile(justfile, "utf8");

  assert.match(source, /^generate:/m);
  assert.match(source, /^debug: generate/m);
  assert.match(source, /scripts\/probe-clean\.ps1/);
  assert.match(source, /scripts\/build-debug\.ps1/);
  assert.match(source, /cmd\/dixiedata-web/);
  assert.match(source, /cmd\/seed-data/);
  assert.match(source, /cmd\/gold-master/);
  assert.match(source, /tools\/tune/);
});

test("Just debug recipe preserves explicit PowerShell invocation", async () => {
  const source = await readFile(justfile, "utf8");
  assert.match(source, /pwsh\s+-NoLogo\s+-NoProfile/);
  assert.match(source, /-File\s+scripts\/build-debug\.ps1/);
});
