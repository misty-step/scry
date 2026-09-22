import assert from "node:assert/strict";
import test from "node:test";

import { timingSafeStringEqual } from "./timing-safe-equal.mjs";

test("timing-safe string comparison uses the runtime primitive for equal lengths", () => {
  const calls = [];
  const subtle = {
    timingSafeEqual(left, right) {
      calls.push([left, right]);
      return Buffer.from(left).equals(Buffer.from(right));
    },
  };

  assert.equal(timingSafeStringEqual("Bearer secret", "Bearer secret", subtle), true);
  assert.equal(timingSafeStringEqual("Bearer aaaaaa", "Bearer bbbbbb", subtle), false);
  assert.equal(calls.length, 2);
});

test("timing-safe string comparison still invokes the runtime primitive when lengths differ", () => {
  const calls = [];
  const subtle = {
    timingSafeEqual(left, right) {
      calls.push([left, right]);
      return true;
    },
  };

  assert.equal(timingSafeStringEqual("short", "a much longer secret", subtle), false);
  assert.equal(calls.length, 1);
  assert.strictEqual(calls[0][0], calls[0][1]);
});