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

  form.addEventListener("submit", function (e) {
    if (broken) { e.preventDefault(); if (error) error.textContent = unavailable; }
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
  card.on("ready", function () {
    ready = true;
    clearTimeout(timer);
    if (!broken) mount.dataset.stripeReady = "true";
  });
  try {
    card.on("loaderror", function () {
      clearTimeout(timer);
      fail("stripe_load_error");
    });
  } catch (err) { /* older Stripe.js: the ready timer still covers it */ }
  card.on("change", function (e) { if (!broken) error.textContent = e.error ? e.error.message : ""; });
  form.addEventListener("submit", function (e) {
    if (broken || hidden.value) return;
    e.preventDefault();
    var button = form.querySelector("button");
    button.disabled = true;
    stripe.createPaymentMethod({ type: "card", card: card }).then(function (res) {
      if (res.error) {
        error.textContent = res.error.message; // the reason the processor gives
        button.disabled = false;
        return;
      }
      hidden.value = res.paymentMethod.id;
      form.submit();
    }, function () {
      error.textContent = "Couldn't reach the card processor. Check your connection and try again.";
      button.disabled = false;
      report("stripe_payment_method_failed");
    });
  });
})();
