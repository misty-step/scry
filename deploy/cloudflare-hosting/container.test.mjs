import assert from "node:assert/strict";
import test from "node:test";

import { fatalContainerError } from "./container-lifecycle.mjs";

test("container lifecycle errors remain fatal", () => {
  const failure = new Error("synthetic startup failure");
  const messages = [];
  assert.throws(
    () => fatalContainerError(failure, (...values) => messages.push(values)),
    error => error === failure,
  );
  assert.deepEqual(messages, [["[scry-container] error:", "Error: synthetic startup failure"]]);
});
