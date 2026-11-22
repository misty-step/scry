// The card step with Stripe (test mode). Card details go from the
// browser straight to Stripe; Scry's server only ever sees the resulting
// payment method id.
(function () {
  "use strict";
  var mount = document.getElementById("card-element");
  var form = document.querySelector("[data-card-form]");
  if (!mount || !form) return;
  var error = document.getElementById("card-error");
  var hidden = document.getElementById("payment_method");
  var unavailable = "The card form couldn't load. Check your connection, allow js.stripe.com in any content blocker, then reload this page.";
  var broken = false;

  // report sends one of the fixed integration codes the server accepts;
  // never Stripe's or the browser's error text.
  function report(code) {
    if (window.scryReport) window.scryReport("integration_error", code);
  }

  // fail shows an actionable message in place of a silent empty field, and
  // keeps the form from posting without a card.
  function fail(code) {
    if (broken) return;
    broken = true;
    if (error) error.textContent = unavailable;
    report(code);
  }

  // The processing state (app.js) starts on submit; every path that keeps
  // the learner on this page gives the form back as it was.
  function giveBack() {
    if (window.scryBusy) window.scryBusy.stop(form);
  }

  form.addEventListener("submit", function (e) {
    if (broken) { e.preventDefault(); if (error) error.textContent = unavailable; giveBack(); }
  });

  if (!window.Stripe) { fail("stripe_script_missing"); return; }

  var stripe, card;
  try {
    stripe = window.Stripe(mount.dataset.key);
    // 16px text on a 24px line: the field's frame is a full line tall and
    // phones don't zoom into it.
    card = stripe.elements().create("card", { style: { base: { fontSize: "16px", lineHeight: "24px" } } });
    card.mount(mount);
  } catch (err) {
    fail("stripe_mount_failed");
    return;
  }
  // The card field is an iframe from js.stripe.com; if it never becomes
  // ready (blocked by policy or network), say so rather than leave a blank.
  var ready = false;
  var timer = setTimeout(function () { if (!ready) fail("stripe_ready_timeout"); }, 15000);
  card.on("ready", function () { ready = true; clearTimeout(timer); });
  try {
    card.on("loaderror", function () {
      clearTimeout(timer);
      fail("stripe_load_error");
    });
  } catch (err) { /* older Stripe.js: the ready timer still covers it */ }
  card.on("change", function (e) { if (!broken) error.textContent = e.error ? e.error.message : ""; });
  // Each submission is one numbered attempt on the form
  // (form.dataset.submitAttempt, kept by app.js's processing state: a new
  // submit and every give-back, the 30 second timeout included, move it
  // on). begin starts this script's attempt and returns its number.
  function current() {
    return Number(form.dataset.submitAttempt) || 0;
  }
  function begin() {
    if (window.scryBusy) window.scryBusy.start(form);
    else form.dataset.submitAttempt = String(current() + 1);
    return current();
  }

  // waiting is the attempt this script is carrying: its card is with the
  // processor, or its form is on the way to Scry.
  var waiting = 0;
  form.addEventListener("submit", function (e) {
    if (broken) return;
    var again = e.defaultPrevented || (waiting !== 0 && waiting === current());
    e.preventDefault();
    // A second submit while this attempt is still under way changes
    // nothing: one attempt, one request to the processor, one post.
    if (again) return;
    var button = form.querySelector("button");
    button.disabled = true;
    // A new attempt confirms the card that is in the field now. Whatever an
    // earlier attempt left behind (it was given back after the timeout) is
    // dropped, so a retry can never post the card it replaced.
    hidden.value = "";
    var attempt = waiting = begin();
    stripe.createPaymentMethod({ type: "card", card: card }).then(function (res) {
      // Superseded (the form was given back after the timeout, or a newer
      // submit started): this answer is for a card the learner may have
      // replaced. It is never submitted and changes nothing on the page.
      if (current() !== attempt) return;
      if (res.error) {
        waiting = 0;
        error.textContent = res.error.message; // the reason the processor gives
        button.disabled = false;
        giveBack();
        return;
      }
      hidden.value = res.paymentMethod.id;
      form.submit();
    }, function () {
      if (current() !== attempt) return; // superseded: see above
      waiting = 0;
      error.textContent = "Couldn't reach the card processor. Check your connection and try again.";
      button.disabled = false;
      giveBack();
      report("stripe_payment_method_failed");
    });
  });
})();
