import assert from "node:assert/strict";
import test from "node:test";

import { latestSnapshotKey } from "./recovery-key.mjs";

const older = "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip";
const newer = "scry-20260922T150000.000000000Z-11111111111111111111111111111111.scry-backup.zip";

test("recovered boot refuses a missing recovery binding", async () => {
  await assert.rejects(latestSnapshotKey(undefined), /RECOVERY binding is required/);
});

test("recovered boot refuses a bucket without a complete snapshot key", async () => {
  const recovery = { list: async () => ({ objects: [{ key: "partial-upload.tmp" }], truncated: false }) };
  await assert.rejects(latestSnapshotKey(recovery), /no complete recovery snapshot/);
});

test("recovered boot selects the newest immutable completed snapshot", async () => {
  const recovery = {
    list: async () => ({
      objects: [{ key: older }, { key: "partial-upload.tmp" }, { key: newer }],
      truncated: false,
    }),
  };
  assert.equal(await latestSnapshotKey(recovery), newer);
});
