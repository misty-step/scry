window.addEventListener("load", async function () {
 if (!window.Clerk) return;
 await window.Clerk.load();
 var target = document.getElementById("clerk-auth");
 if (target) {
 if (window.Clerk.user) {
 var email = window.Clerk.user.primaryEmailAddress;
 if (email && email.verification.status === "verified" && window.Clerk.session && window.Clerk.session.status === "active") {
 location.replace("/study"); return;
 }
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
