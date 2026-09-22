import assert from "node:assert/strict";
import { createLocalJWKSet, exportJWK, generateKeyPair, SignJWT } from "jose";
import test from "node:test";

import { accessAuthorized } from "./auth.mjs";

const issuer = "https://misty-step.cloudflareaccess.com";
const audience = "staging-audience";
const owner = "immutable-owner-subject";

async function signedRequest(subject) {
  const { privateKey, publicKey } = await generateKeyPair("RS256", { extractable: true });
  const jwk = await exportJWK(publicKey);
  jwk.alg = "RS256";
  jwk.kid = "test-key";
  const token = await new SignJWT({ sub: subject, email: "owner@example.test" })
    .setProtectedHeader({ alg: "RS256", kid: jwk.kid })
    .setIssuer(issuer)
    .setAudience(audience)
    .setIssuedAt()
    .setExpirationTime("5m")
    .sign(privateKey);
  const request = new Request("https://scry.example/", {
    headers: { "cf-access-jwt-assertion": token },
  });
  return { request, keySet: createLocalJWKSet({ keys: [jwk] }) };
}

const env = {
  SCRY_ACCESS_TEAM_DOMAIN: issuer,
  SCRY_ACCESS_AUD: audience,
  SCRY_ACCESS_OWNER_SUB: owner,
};

test("Access JWT grants the fixed app owner only to the immutable owner subject", async () => {
  const accepted = await signedRequest(owner);
  assert.equal(await accessAuthorized(accepted.request, env, () => accepted.keySet), true);

  const denied = await signedRequest("different-account-subject");
  assert.equal(await accessAuthorized(denied.request, env, () => denied.keySet), false);
});

test("Access JWT fails closed when the immutable owner subject is not configured", async () => {
  const accepted = await signedRequest(owner);
  const { SCRY_ACCESS_OWNER_SUB: _removed, ...missingOwner } = env;
  assert.equal(await accessAuthorized(accepted.request, missingOwner, () => accepted.keySet), false);
});
