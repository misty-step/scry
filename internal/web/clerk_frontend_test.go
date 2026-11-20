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
async function run({signup = false, user = null, status = "active", target = true, hash = ""} = {}) {
  let load, submit, hashchange, calls = [];
  const heading = {}, lede = {};
  const location = {hash, replace(url) { calls.push(["redirect", url]); }};
  const document = {
    getElementById(id) { return id === "auth-heading" ? heading : id === "auth-lede" ? lede : target ? el : null; },
    createElement() { return {}; },
    querySelector() { return {addEventListener(event, callback) { submit = callback; }}; }
  };
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
    window: {Clerk: clerk, addEventListener(event, callback) {
      if (event === "load") load = callback;
      if (event === "hashchange") hashchange = callback;
    }},
    document, location
  });
  await load();
  return {calls, submit, el, heading, lede, document, navigate(hash) { location.hash = hash; hashchange(); }};
}
(async () => {
  for (const signup of [true, false]) {
    const r = await run({signup});
    assert.equal(r.heading.textContent, signup ? "Sign up" : "Sign in");
    assert.equal(r.document.title, (signup ? "Sign up" : "Sign in") + " · scry");
    assert.match(r.lede.textContent, signup ? /Create your account/ : /Use your email/);
    assert.equal(r.calls[1][0], signup ? "signup" : "signin");
    assert.equal(r.calls[1][1].signInUrl, "/signin");
    assert.equal(r.calls[1][1].signUpUrl, "/signup");
    assert.equal(r.calls[1][1].forceRedirectUrl, "/study");
  }
  for (const [verification, status] of [["unverified", "active"], ["verified", "pending"]]) {
    for (const [signup, hash, title, lede] of [
    [true, "#/verify-email-address", "Verify your email", "Verify your email before"],
    [false, "#/forgot-password", "Reset your password", "Reset your password by email"],
    [false, "#/reset-password", "Reset your password", "Reset your password by email"]
  ]) {
    const page = await run({signup, hash});
    assert.equal(page.heading.textContent, title);
    assert.equal(page.document.title, title + " · scry");
    assert.ok(page.lede.textContent.startsWith(lede));
    page.navigate("");
    assert.equal(page.heading.textContent, signup ? "Sign up" : "Sign in");
    page.navigate(hash);
    assert.equal(page.heading.textContent, title);
    assert.equal(page.document.title, title + " · scry");
    assert.equal(page.calls.length, 2, "copy changes must not remount Clerk or redirect");
  }
  const r = await run({user: {primaryEmailAddress: {verification: {status: verification}}}, status});
    assert.equal(r.heading.textContent, "Verify your email");
    assert.equal(r.document.title, "Verify your email · scry");
    assert.ok(r.calls.includes("verification"));
    assert.match(r.el.textContent, /Verify your email/);
    assert.ok(!r.calls.some(c => Array.isArray(c) && c[0] === "redirect"));
  }
  for (const [signup, hash, title, lede] of [
    [true, "#/verify-email-address", "Verify your email", "Verify your email before"],
    [false, "#/forgot-password", "Reset your password", "Reset your password by email"],
    [false, "#/reset-password", "Reset your password", "Reset your password by email"]
  ]) {
    const page = await run({signup, hash});
    assert.equal(page.heading.textContent, title);
    assert.equal(page.document.title, title + " · scry");
    assert.ok(page.lede.textContent.startsWith(lede));
    page.navigate("");
    assert.equal(page.heading.textContent, signup ? "Sign up" : "Sign in");
    page.navigate(hash);
    assert.equal(page.heading.textContent, title);
    assert.equal(page.document.title, title + " · scry");
    assert.equal(page.calls.length, 2, "copy changes must not remount Clerk or redirect");
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
