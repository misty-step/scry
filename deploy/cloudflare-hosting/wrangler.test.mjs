import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./wrangler.jsonc", import.meta.url), "utf8");
const config = JSON.parse(source.split("\n").filter(line => !/^\s*\/\//.test(line)).join("\n"));

for (const [name, environment] of [["default", config], ...Object.entries(config.env)]) {
  test(`retired Go host ${name} leaves hostname routing unclaimed`, () => {
    assert.deepEqual(environment.routes ?? [], [], `${name} must not claim hostnames`);
    assert.equal(environment.route, undefined, `${name} must not claim a hostname`);
  });
}
