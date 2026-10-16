// Disposable upstream DO for the Go edge's exact-module verification only.
// No learner, model, recovery or production bindings.
const revision = "candidate";
export class FixtureContainer {
  constructor(ctx) { this.ctx = ctx; }
  async fetch(request) {
    const mode = new URL(request.url).searchParams.get("mode");
    if (mode === "container-status") return Response.json({ enabled: !!this.ctx.container,
      running: this.ctx.container?.running ?? false });
    if (mode === "throw") throw new Error("PRIVATE-error-canary");
    if (mode === "fail") return new Response("PRIVATE-response-canary", { status: 503 });
    return new Response("<main>Authored synthetic study: held feedback; deliberate Next.</main>", {
      headers: { "content-type": "text/html" },
    });
  }
}
export default { async fetch(request, env) {
  if (new URL(request.url).searchParams.get("mode") !== "container-status") return new Response("Not found", { status: 404 });
  const response = await env.FIXTURE.get(env.FIXTURE.idFromName("singleton")).fetch(request);
  return Response.json({ ...await response.json(), revision });
} };
