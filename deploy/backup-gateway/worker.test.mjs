import assert from "node:assert/strict";
import test from "node:test";

import worker from "./worker.mjs";

const vmToken = "v".repeat(64);
const containerToken = "c".repeat(64);
const key = "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip";

function env() {
  return {
    BACKUP_TOKEN: vmToken,
    SCRY_CONTAINER_BACKUP_TOKEN: containerToken,
    RECOVERY: {
      put: async () => ({ httpEtag: '"synthetic"' }),
    },
  };
}

function upload(token) {
  return new Request(`https://backup.example/${key}`, {
    method: "PUT",
    headers: { authorization: `Bearer ${token}`, "content-type": "application/zip" },
    body: new Uint8Array([1, 2, 3]),
  });
}

test("backup gateway keeps VM authority and accepts the separate container capability", async () => {
  for (const token of [vmToken, containerToken]) {
    const response = await worker.fetch(upload(token), env());
    assert.equal(response.status, 201);
  }
});

test("backup gateway denies any other bearer", async () => {
  const response = await worker.fetch(upload("wrong"), env());
  assert.equal(response.status, 401);
});

test("backup gateway never overwrites an immutable snapshot", async () => {
  const fixture = env();
  fixture.RECOVERY.put = async () => null;
  const response = await worker.fetch(upload(containerToken), fixture);
  assert.equal(response.status, 412);
});

test("backup gateway rejects a mismatched caller checksum before writing", async () => {
  let wrote = false;
  const fixture = env();
  fixture.RECOVERY.put = async () => {
    wrote = true;
    return { httpEtag: '"unexpected"' };
  };
  const request = upload(containerToken);
  request.headers.set("x-amz-content-sha256", "0".repeat(64));
  const response = await worker.fetch(request, fixture);
  assert.equal(response.status, 422);
  assert.equal(wrote, false);
});

test("backup gateway denies destructive methods", async () => {
  const response = await worker.fetch(new Request(`https://backup.example/${key}`, {
    method: "DELETE",
    headers: { authorization: `Bearer ${containerToken}` },
  }), env());
  assert.equal(response.status, 405);
  assert.equal(response.headers.get("allow"), "PUT, GET, HEAD");
});
