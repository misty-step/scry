// token reads one of Scry's color tokens as the browser resolved it for
// the current mode (light or dark), as a plain rgb()/rgba() string Clerk can
// parse; outside a browser it falls back to the light token.
function token(name, fallback) {
 try {
 var probe = document.createElement("span");
 probe.style.color = "var(--" + name + ")";
 document.body.appendChild(probe);
 var value = getComputedStyle(probe).color;
 probe.remove();
 return value || fallback;
 } catch (err) { return fallback; }
}

// appearance makes the embedded Clerk panel one of Scry's surfaces in both
// modes (DESIGN.md, Sign-up and trial): page/raised surface, a 1 px line
// border, square card, 4 px controls, Geist, 16 px inputs, and no shadows
// or gradients, so dark mode never flashes a bright white card.
function appearance() {
 var page = token("page", "#FFFFFF"), raised = token("raised", "#FFFFFF"), ink = token("ink", "#0B1015"),
 muted = token("ink-muted", "rgba(11, 16, 21, 0.6)"), line = token("line", "rgba(11, 16, 21, 0.12)"),
 accent = token("accent", "#0562EF"), onAccent = token("on-accent", "#FFFFFF"), sunken = token("sunken", "#F4F4F6");
 var flat = {boxShadow: "none", backgroundImage: "none"};
 function with_(o) { var r = {}; for (var k in flat) r[k] = flat[k]; for (var j in o) r[j] = o[j]; return r; }
 return {
 variables: {fontFamily: "Geist, ui-sans-serif, system-ui, sans-serif", fontFamilyButtons: "Geist, ui-sans-serif, system-ui, sans-serif",
 fontSize: "16px", borderRadius: "4px", colorPrimary: accent, colorTextOnPrimaryBackground: onAccent,
 colorBackground: raised, colorText: ink, colorTextSecondary: muted, colorInputBackground: page,
 colorInputText: ink, colorNeutral: ink, colorShimmer: "transparent"},
 elements: {
 rootBox: {width: "100%"},
 cardBox: with_({width: "100%", maxWidth: "none", border: "1px solid " + line, borderRadius: "0", background: raised}),
 card: with_({borderRadius: "0", background: raised, border: "0"}),
 footer: with_({background: page, borderTop: "1px solid " + line, borderRadius: "0"}),
 formFieldInput: with_({fontSize: "16px", borderRadius: "4px", border: "1px solid " + line, background: page, color: ink}),
 formButtonPrimary: with_({borderRadius: "4px", background: accent, color: onAccent}),
 socialButtonsBlockButton: with_({borderRadius: "4px", border: "1px solid " + line}),
 otpCodeFieldInput: with_({fontSize: "16px", border: "1px solid " + line}),
 badge: with_({borderRadius: "4px"}),
 dividerLine: {background: line},
 identityPreview: with_({background: sunken, borderRadius: "4px"})
 }
 };
}

window.addEventListener("load", async function () {
 // Clerk owns the hash-routed steps; update only Scry's surrounding copy.
 var target = document.getElementById("clerk-auth");
 var verifying = false;
 function authCopy() {
 if (!target) return;
 var signup = target.dataset.signup === "true";
 var title = signup ? "Sign up" : "Sign in";
 var lede = signup
 ? "Create your account with email and password. Verify your email before continuing to study or billing."
 : "Use your email and password: enter your email first, then your password. Forgotten your password? After you enter your email, choose “Forgot password” on the password step to reset it by email.";
 var step = (location.hash || "").split("?")[0].replace(/\/$/, "");
 if (verifying || step === "#/verify-email-address") {
 title = "Verify your email";
 lede = "Verify your email before continuing to study or billing.";
 } else if (!signup && (step === "#/forgot-password" || step === "#/reset-password")) {
 title = "Reset your password";
 lede = "Reset your password by email, then sign in with your new password.";
 }
 document.getElementById("auth-heading").textContent = title;
 document.getElementById("auth-lede").textContent = lede;
 document.title = title + " · scry";
 }
 authCopy();
 window.addEventListener("hashchange", authCopy);
 if (!window.Clerk) return;
 await window.Clerk.load();
 if (target) {
 if (window.Clerk.user) {
 var email = window.Clerk.user.primaryEmailAddress;
 if (email && email.verification.status === "verified" && window.Clerk.session && window.Clerk.session.status === "active") {
 location.replace("/study"); return;
 }
 verifying = true; authCopy();
 target.textContent = "Verify your email before continuing. After verification, reload this page.";
 var profile = document.createElement("div"); target.appendChild(profile);
 window.Clerk.mountUserProfile(profile);
 return;
 }
 var options = {signInUrl:"/signin", signUpUrl:"/signup", forceRedirectUrl:"/study", appearance:appearance()};
 if (target.dataset.signup === "true") window.Clerk.mountSignUp(target,options);
 else window.Clerk.mountSignIn(target,options);
 }
 var form=document.querySelector('form[action="/signout"]');
 if(form) form.addEventListener("submit",function(e){e.preventDefault();window.Clerk.signOut({redirectUrl:"/"});});
});
