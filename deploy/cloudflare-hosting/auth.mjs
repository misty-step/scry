import { createRemoteJWKSet, jwtVerify } from "jose";

let jwksCache = null;

function remoteJWKS(teamDomain) {
  const url = new URL(`${teamDomain}/cdn-cgi/access/certs`);
  if (!jwksCache || jwksCache.url !== url.href) {
    jwksCache = { url: url.href, set: createRemoteJWKSet(url) };
  }
  return jwksCache.set;
}

export async function accessAuthorized(request, env, keySetFactory = remoteJWKS) {
  const teamDomain = env.SCRY_ACCESS_TEAM_DOMAIN;
  const audience = env.SCRY_ACCESS_AUD;
  const ownerSubject = env.SCRY_ACCESS_OWNER_SUB;
  if (!teamDomain || !audience || !ownerSubject) return false;
  const token = request.headers.get("cf-access-jwt-assertion");
  if (!token) return false;
  try {
    const { payload } = await jwtVerify(token, keySetFactory(teamDomain), {
      issuer: teamDomain,
      audience,
    });
    return typeof payload.sub === "string" && payload.sub === ownerSubject;
  } catch {
    return false;
  }
}
