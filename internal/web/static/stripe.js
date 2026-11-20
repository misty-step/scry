// The card step with Stripe (test mode). Card details go from the
// browser straight to Stripe; Scry's server only ever sees the resulting
// payment method id.
(function () {
  "use strict";
  var mount = document.getElementById("card-element");
  var form = document.querySelector("[data-card-form]");
  if (!mount || !form || !window.Stripe) return;
  var stripe = window.Stripe(mount.dataset.key);
  var card = stripe.elements().create("card");
  card.mount(mount);
  var error = document.getElementById("card-error");
  var hidden = document.getElementById("payment_method");
  card.on("change", function (e) { error.textContent = e.error ? e.error.message : ""; });
  form.addEventListener("submit", function (e) {
    if (hidden.value) return;
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
    });
  });
})();
