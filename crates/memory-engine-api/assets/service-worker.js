// Only public shell assets enter this cache. Mutable app assets revalidate
// on every load, so a CSS/JS-only release never depends on a manual version bump.
const CACHE_NAME = "scry-shell-v6";
const OFFLINE_URL = "/offline.html";
const IMMUTABLE_SHELL_URLS = Object.freeze([
  OFFLINE_URL,
  "/static/ledger.css",
  "/static/app.js",
  "/static/fonts/literata-latin-variable.woff2",
  "/static/fonts/manrope-latin-variable.woff2",
  "/static/fonts/OFL.txt",
  "/manifest.webmanifest",
  "/favicon.png",
  "/icon-192.png",
  "/icon-512.png",
  "/apple-touch-icon.png",
]);
const REVALIDATED_SHELL_URLS = new Set([
  OFFLINE_URL, "/static/ledger.css", "/static/app.js", "/manifest.webmanifest",
]);

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then(async (cache) => {
      for (const url of IMMUTABLE_SHELL_URLS) {
        const response = await fetch(url, { cache: "reload" });
        if (!response.ok) throw new Error("shell asset failed: " + url);
        await cache.put(url, response);
      }
      await self.skipWaiting();
    }),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => key.startsWith("scry-shell-") && key !== CACHE_NAME)
          .map((key) => caches.delete(key)),
      ),
    ).then(() => self.clients.claim()),
  );
});

function isMagicLinkUrl(url) {
  return (
    url.pathname === "/app/login/verify" ||
    url.pathname === "/app/return-notifications" ||
    url.searchParams.has("token") ||
    url.searchParams.has("unsubscribeToken")
  );
}

function isAuthenticatedOrLearnerRequest(url) {
  return (
    url.pathname === "/" ||
    url.pathname.startsWith("/app/") ||
    url.pathname === "/accounts" ||
    url.pathname.startsWith("/accounts/") ||
    url.pathname.startsWith("/v1/") ||
    url.pathname.startsWith("/internal/")
  );
}

function isImmutableShellRequest(request, url) {
  return (
    request.method === "GET" &&
    url.origin === self.location.origin &&
    !url.search &&
    IMMUTABLE_SHELL_URLS.includes(url.pathname)
  );
}

async function networkFirstNavigation(request) {
  try {
    return await fetch(request);
  } catch {
    return caches.match(OFFLINE_URL);
  }
}

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Mutations, API responses, learner/session pages, and magic links stay on the network.
  // No request or response from these paths enters Cache Storage.
  if (request.method !== "GET" || isMagicLinkUrl(url)) return;
  if (request.mode === "navigate") {
    event.respondWith(networkFirstNavigation(request));
    return;
  }
  if (isAuthenticatedOrLearnerRequest(url) || !isImmutableShellRequest(request, url)) return;

  if (REVALIDATED_SHELL_URLS.has(url.pathname)) {
    event.respondWith(
      fetch(request, { cache: "no-cache" }).then((response) => {
        if (response.ok) {
          event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.put(request, response.clone())));
        }
        return response;
      }).catch(async () => (await caches.match(request)) || Response.error()),
    );
    return;
  }

  event.respondWith(
    caches.match(request).then((cached) =>
      cached || fetch(request).then((response) => {
        if (!response.ok) return response;
        return caches.open(CACHE_NAME).then((cache) => {
          cache.put(request, response.clone());
          return response;
        });
      }),
    ),
  );
});
