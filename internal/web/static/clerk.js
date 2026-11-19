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
 : "Use your email and password. Forgotten your password? Choose “Forgot password” below to reset it by email.";
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
 var options = {signInUrl:"/signin", signUpUrl:"/signup", forceRedirectUrl:"/study",
 appearance:{variables:{fontFamily:"Geist, sans-serif",colorPrimary:"#0562EF",borderRadius:"4px"}}};
 if (target.dataset.signup === "true") window.Clerk.mountSignUp(target,options);
 else window.Clerk.mountSignIn(target,options);
 }
 var form=document.querySelector('form[action="/signout"]');
 if(form) form.addEventListener("submit",function(e){e.preventDefault();window.Clerk.signOut({redirectUrl:"/"});});
});
