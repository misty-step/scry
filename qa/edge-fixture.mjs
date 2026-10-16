// Disposable upstream DO for the Go edge's exact-module verification only.
// No learner, model, recovery or production bindings.
export class FixtureContainer {
  async fetch(request) {
    const mode = new URL(request.url).searchParams.get("mode");
    if (mode === "throw") throw new Error("PRIVATE-error-canary");
    if (mode === "fail") return new Response("PRIVATE-response-canary", { status: 503 });
    return new Response("<main>Authored synthetic study: held feedback; deliberate Next.</main>", {
      headers: { "content-type": "text/html" },
    });
  }
}
export default { fetch() { return new Response("Not found", { status: 404 }); } };
