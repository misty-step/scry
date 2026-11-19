package web_test

import (
	"os/exec"
	"testing"
)

// Exercise our component wiring offline, not Clerk's hosted UI or email delivery.
func TestClerkFrontendStandIn(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is needed for the Clerk frontend stand-in")
	}
	cmd := exec.Command("node", "-e", `
const vm = require("node:vm"), fs = require("node:fs"), assert = require("node:assert/strict");
const source = fs.readFileSync("static/clerk.js", "utf8");
async function run({signup = false, user = null, status = "active", target = true} = {}) {
  let load, submit, calls = [];
  const el = {dataset: {signup: String(signup)}, appendChild() {}};
  const clerk = {
    user, session: {status},
    async load() { calls.push("load"); },
    mountSignUp(node, options) { calls.push(["signup", options]); },
    // Clerk's SignIn component owns its Forgot password flow.
    mountSignIn(node, options) { calls.push(["signin", options]); },
    mountUserProfile() { calls.push("verification"); },
    signOut(options) { calls.push(["signout", options.redirectUrl]); }
  };
  vm.runInNewContext(source, {
    window: {Clerk: clerk, addEventListener(event, callback) { load = callback; }},
    document: {
      getElementById() { return target ? el : null; },
      createElement() { return {}; },
      querySelector() { return {addEventListener(event, callback) { submit = callback; }}; }
    },
    location: {replace(url) { calls.push(["redirect", url]); }}
  });
  await load();
  return {calls, submit, el};
}
(async () => {
  for (const signup of [true, false]) {
    const r = await run({signup});
    assert.equal(r.calls[1][0], signup ? "signup" : "signin");
    assert.equal(r.calls[1][1].signInUrl, "/signin");
    assert.equal(r.calls[1][1].signUpUrl, "/signup");
    assert.equal(r.calls[1][1].forceRedirectUrl, "/study");
  }
  for (const [verification, status] of [["unverified", "active"], ["verified", "pending"]]) {
    const r = await run({user: {primaryEmailAddress: {verification: {status: verification}}}, status});
    assert.ok(r.calls.includes("verification"));
    assert.match(r.el.textContent, /Verify your email/);
    assert.ok(!r.calls.some(c => Array.isArray(c) && c[0] === "redirect"));
  }
  const r = await run({user: {primaryEmailAddress: {verification: {status: "verified"}}}});
  assert.equal(r.calls[1][0], "redirect");
  assert.equal(r.calls[1][1], "/study");
  const out = await run({target: false});
  let prevented = false;
  out.submit({preventDefault() { prevented = true; }});
  assert.ok(prevented);
  assert.equal(out.calls[1][0], "signout");
  assert.equal(out.calls[1][1], "/");
})().catch(err => { console.error(err); process.exit(1); });
`)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Clerk frontend: %v\n%s", err, output)
	}
}
