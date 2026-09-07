// Register the server-rendered shell as a PWA without persisting page state.
(function () {
  "use strict";
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", function () {
    navigator.serviceWorker.register("/sw.js", { scope: "/", updateViaCache: "none" }).catch(function () {});
  });
})();

// Review forms keep one active request and one correlated completion attempt.
// The server owns grading; JavaScript only acknowledges, swaps authoritative
// HTML, and observes visibility after paint. Native forms remain the no-JS path.
// A failed fetch retains the answer and idempotency key for an intentional retry.
(function () {
  "use strict";

  var HANDOFF_STORAGE_KEY = "memory-engine.submit-handoff.v1";
  var HANDOFF_VERSION = 1;
  var HANDOFF_ACTION = "review_submit";
  var BUSY_RECOVERY_MS = 30000;
  var MAX_DURATION_MS = 60000;
  var HANDOFF_TTL_MS = MAX_DURATION_MS + 5000;
  var MAX_SAFE_INTEGER = 9007199254740991;
  var REQUEST_ID_RE = /^req_[0-9a-f]{32}$/;
  var TRACE_ID_RE = /^trace_[0-9a-f]{32}$/;
  var perf = window.performance;
  var hasMonotonicClock =
    !!perf && typeof perf.now === "function";
  var presentedAt = hasMonotonicClock ? perf.now() : Date.now();
  var state = {
    busy: false,
    control: null,
    dimmed: [],
    request: null,
    timeoutId: null,
    landingAttempted: false,
    // Any hide, new action, or abandoned request invalidates queued paint work.
    landingEpoch: 0
  };


  function isFiniteNumber(value) {
    return typeof value === "number" && isFinite(value);
  }

  function isSafeInteger(value) {
    return (
      isFiniteNumber(value) &&
      Math.floor(value) === value &&
      value >= 0 &&
      value <= MAX_SAFE_INTEGER
    );
  }

  function hasClass(element, name) {
    return !!element && !!element.classList && element.classList.contains(name);
  }

  function isNativeForm(form) {
    return !!form && String(form.tagName || "").toLowerCase() === "form";
  }

  function isReviewSubmitForm(form) {
    return (
      isNativeForm(form) &&
      typeof form.getAttribute === "function" &&
      form.getAttribute("action") === "/app/submit"
    );
  }

  function isInPlaceActionForm(form) {
    if (!isNativeForm(form) || typeof form.getAttribute !== "function") return false;
    var action = form.getAttribute("action") || "";
    return (
      action === "/app/submit" ||
      action === "/app/next" ||
      action === "/app/draft/keep" ||
      action === "/app/draft/reject" ||
      action === "/app/content-feedback" ||
      action === "/app/skip" ||
      action === "/app/snooze" ||
      action === "/app/snooze-concept" ||
      action === "/app/reveal" ||
      action === "/app/reference" ||
      action === "/app/bridge"
    );
  }



  function responseClockNow() {
    return hasMonotonicClock ? perf.now() : Date.now();
  }

  // Handoff epochs deliberately use the absolute monotonic origin. Date.now()
  // is not a substitute: wall-clock changes could make a valid token appear
  // expired or let an expired token survive.
  function absoluteEpochNow() {
    if (
      !perf ||
      typeof perf.now !== "function" ||
      !isFiniteNumber(perf.timeOrigin)
    ) {
      return null;
    }
    var epoch = perf.timeOrigin + perf.now();
    return isSafeInteger(Math.round(epoch)) ? Math.round(epoch) : null;
  }

  function randomId(prefix) {
    if (
      !window.crypto ||
      typeof window.crypto.getRandomValues !== "function" ||
      typeof window.Uint8Array !== "function"
    ) {
      return null;
    }
    var bytes = new window.Uint8Array(16);
    try {
      window.crypto.getRandomValues(bytes);
    } catch (error) {
      return null;
    }
    var hex = "";
    for (var i = 0; i < bytes.length; i++) {
      hex += ("0" + bytes[i].toString(16)).slice(-2);
    }
    var value = prefix + hex;
    return prefix === "req_"
      ? REQUEST_ID_RE.test(value)
        ? value
        : null
      : TRACE_ID_RE.test(value)
      ? value
      : null;
  }

  function storage() {
    try {
      return window.sessionStorage || null;
    } catch (error) {
      return null;
    }
  }

  function removeHandoff() {
    var store = storage();
    if (!store) return;
    try {
      store.removeItem(HANDOFF_STORAGE_KEY);
    } catch (error) {
      // Storage can be unavailable or quota-blocked; telemetry fails closed.
    }
  }
  function removeHandoffIfToken(token) {
    var store = storage();
    if (!store) return;
    try {
      var raw = store.getItem(HANDOFF_STORAGE_KEY);
      if (!raw) return;
      var handoff = JSON.parse(raw);
      if (handoff && handoff.token === token) {
        store.removeItem(HANDOFF_STORAGE_KEY);
      }
    } catch (error) {
      // A malformed or unavailable store is never a reason to affect review.
      removeHandoff();
    }
  }


  function clearTimeoutIfAny() {
    if (state.timeoutId !== null) {
      if (typeof window.clearTimeout === "function") window.clearTimeout(state.timeoutId);
      state.timeoutId = null;
    }
  }

  function resetReviewUi() {
    document.documentElement.removeAttribute("data-busy");
    if (state.control) {
      state.control.removeAttribute("data-pressed");
      state.control.removeAttribute("aria-disabled");
      if (state.control.getAttribute("data-pending-label") === "1") {
        var original = state.control.getAttribute("data-original-label");
        if (typeof original === "string") state.control.textContent = original;
        state.control.removeAttribute("data-pending-label");
        state.control.removeAttribute("data-original-label");
      }
    }
    for (var i = 0; i < state.dimmed.length; i++) {
      state.dimmed[i].removeAttribute("data-dim");
    }
    state.control = null;
    state.dimmed = [];
    reviewStatus("", "idle");
  }

  function resetState() {
    clearTimeoutIfAny();
    resetReviewUi();
    state.busy = false;
    state.request = null;
  }

  function reviewStatus(message, status) {
    var element = document.querySelector("[data-review-status]");
    if (!element) return;
    element.textContent = message;
    element.setAttribute("data-state", status);
  }

  function abandonRequest() {
    var request = state.request;
    state.landingEpoch += 1;
    resetState();
    if (request && request.controller) {
      try {
        request.controller.abort();
      } catch (error) {
        // Identity checks also reject a late response without AbortController.
      }
    }
  }

  function failRequest(message) {
    abandonRequest();
    reviewStatus(message, "failed");
  }

  function submitControl(form, event) {
    var control = event && event.submitter;
    if (control) return control;
    if (!form || typeof form.querySelector !== "function") return null;
    return form.querySelector('button[type="submit"], button:not([type])');
  }

  // Immediate pending copy only — never a grade. Server still owns verdicts.
  function pendingLabelFor(form, control) {
    if (!form || typeof form.getAttribute !== "function") return null;
    if (control && hasClass(control, "me-choice")) return null;
    var action = form.getAttribute("action") || "";
    if (action === "/app/next") {
      var current = control && typeof control.textContent === "string" ? control.textContent : "";
      if (current.indexOf("Start") !== -1) return "Starting…";
      return "Loading…";
    }
    if (action === "/app/content-feedback") return "Sending…";
    if (action === "/app/reveal") return "Revealing…";
    if (action === "/app/skip") return "Skipping…";
    if (action === "/app/snooze" || action === "/app/snooze-concept") return "Snoozing…";
    if (action === "/app/draft/keep") return "Keeping…";
    if (action === "/app/draft/reject") return "Rejecting…";
    if (action === "/app/reference") return "Loading…";
    if (action === "/app/bridge") return "Building…";
    if (action === "/app/submit" && control && !hasClass(control, "me-choice")) {
      return "Checking…";
    }
    return null;
  }

  function setBusy(form, control) {
    state.busy = true;
    state.control = control;
    document.documentElement.setAttribute("data-busy", "");
    if (control) {
      control.setAttribute("data-pressed", "");
      control.setAttribute("aria-disabled", "true");
      var pending = pendingLabelFor(form, control);
      if (
        pending &&
        typeof control.textContent === "string" &&
        control.getAttribute("data-pending-label") !== "1"
      ) {
        control.setAttribute("data-pending-label", "1");
        control.setAttribute("data-original-label", control.textContent);
        control.textContent = pending;
      }
      if (hasClass(control, "me-choice") && typeof form.querySelectorAll === "function") {
        var choices = form.querySelectorAll(".me-choice");
        for (var i = 0; i < choices.length; i++) {
          if (choices[i] !== control) {
            choices[i].setAttribute("data-dim", "");
            state.dimmed.push(choices[i]);
          }
        }
      }
    }
    reviewStatus(pendingLabelFor(form, control) || "Checking your answer…", "pending");
    if (typeof window.setTimeout === "function") {
      state.timeoutId = window.setTimeout(function () {
        if (state.request) {
          failRequest("The response is taking too long. Your answer is still here; retry when ready.");
        } else {
          resetState();
        }
      }, BUSY_RECOVERY_MS);
    }
  }

  function traceInput(form, traceId) {
    if (!form || typeof form.querySelector !== "function") return false;
    var input = form.querySelector('input[name="performanceTraceId"]');
    if (!input) {
      if (typeof document.createElement !== "function" || typeof form.appendChild !== "function") return false;
      try {
        input = document.createElement("input");
        input.setAttribute("type", "hidden");
        input.setAttribute("name", "performanceTraceId");
        form.appendChild(input);
      } catch (error) {
        return false;
      }
    }
    input.value = traceId;
    return true;
  }

  function createHandoff(form, startedAtMs, acknowledgedAtMs) {
    removeHandoff();
    if (
      !isSafeInteger(startedAtMs) ||
      !isSafeInteger(acknowledgedAtMs) ||
      boundedDuration(startedAtMs, acknowledgedAtMs) === null
    ) return null;
    var expiresAtMs = startedAtMs + HANDOFF_TTL_MS;
    if (!isSafeInteger(expiresAtMs)) return null;
    var traceId = randomId("trace_");
    if (!traceId || !traceInput(form, traceId)) return null;
    return {
      version: HANDOFF_VERSION,
      action: HANDOFF_ACTION,
      token: traceId,
      startedAtMs: startedAtMs,
      acknowledgedAtMs: acknowledgedAtMs,
      expiresAtMs: expiresAtMs
    };
  }

  function storeHandoff(handoff) {
    if (!handoff) return;
    var store = storage();
    if (!store) return;
    try {
      store.setItem(HANDOFF_STORAGE_KEY, JSON.stringify(handoff));
      if (typeof window.setTimeout === "function") {
        window.setTimeout(function () {
          removeHandoffIfToken(handoff.token);
        }, HANDOFF_TTL_MS);
      }
    } catch (error) {
      removeHandoff();
    }
  }

  function formBody(form, control) {
    if (
      typeof window.FormData !== "function" ||
      typeof window.URLSearchParams !== "function"
    ) return null;
    var fields;
    try {
      fields = new window.FormData(form);
    } catch (error) {
      return null;
    }
    // Clicked submitters (MCQ choices) are not always in FormData(form).
    if (
      control &&
      typeof control.getAttribute === "function" &&
      typeof fields.append === "function"
    ) {
      var name = control.getAttribute("name") || control.name;
      var value =
        typeof control.value === "string"
          ? control.value
          : control.getAttribute("value");
      if (typeof name === "string" && name && typeof value === "string") {
        if (typeof fields.has !== "function" || !fields.has(name)) {
          fields.append(name, value);
        }
      }
    }
    var body = new window.URLSearchParams();
    try {
      fields.forEach(function (value, name) {
        body.append(name, String(value));
      });
    } catch (error) {
      return null;
    }
    return body;
  }

  function viewRoot() {
    return typeof document.querySelector === "function"
      ? document.querySelector(".ae-view")
      : null;
  }

  function replaceHeadMeta(doc, name) {
    var content = metaContent(name, doc);
    var current = document.querySelector('meta[name="' + name + '"]');
    if (typeof content === "string" && content) {
      if (current) {
        current.setAttribute("content", content);
      } else if (document.head && typeof document.createElement === "function") {
        var meta = document.createElement("meta");
        meta.setAttribute("name", name);
        meta.setAttribute("content", content);
        document.head.appendChild(meta);
      }
    } else if (current && current.parentNode) {
      current.parentNode.removeChild(current);
    }
  }

  function applyInPlaceDocument(html, phases) {
    if (typeof html !== "string" || !html) return false;
    if (typeof window.DOMParser !== "function") return false;
    var currentView = viewRoot();
    if (!currentView) return false;
    var doc;
    try {
      doc = new window.DOMParser().parseFromString(html, "text/html");
    } catch (error) {
      return false;
    }
    if (!doc || typeof doc.querySelector !== "function") return false;
    var nextView = doc.querySelector(".ae-view");
    if (!nextView) return false;
    var swapStartedAtMs = phases ? absoluteEpochNow() : null;
    currentView.innerHTML = nextView.innerHTML;
    // Keep header due count honest after a graded submit / continue.
    var currentDue = document.querySelector(".me-due");
    var nextDue = doc.querySelector(".me-due");
    if (currentDue && nextDue) currentDue.textContent = nextDue.textContent;
    // Home uses the four-tab nav footer; review uses the tagline. Crossing
    // those surfaces without swapping footer leaves the wrong chrome.
    var currentFooter = document.querySelector("footer.ae-bar");
    var nextFooter = doc.querySelector("footer.ae-bar");
    if (currentFooter && nextFooter) currentFooter.innerHTML = nextFooter.innerHTML;
    replaceHeadMeta(doc, "memory-engine-csrf-token");
    replaceHeadMeta(doc, "memory-engine-submit-request");
    replaceHeadMeta(doc, "memory-engine-submit-handoff");
    if (phases) phases.domSwapMs = boundedDuration(swapStartedAtMs, absoluteEpochNow());
    presentedAt = responseClockNow();
    var focusTarget =
      currentView.querySelector(".me-verdict") ||
      currentView.querySelector(".me-answer-input, .me-choice, .me-prompt");
    if (focusTarget && typeof focusTarget.focus === "function") {
      try {
        if (!hasClass(focusTarget, "me-choice") && !hasClass(focusTarget, "me-answer-input")) {
          focusTarget.setAttribute("tabindex", "-1");
        }
        focusTarget.focus();
      } catch (error) {
        // Focus is best-effort; it never gates grading or measurement.
      }
    }
    return true;
  }


  function fetchInPlace(form, control, handoff) {
    if (!window.fetch || typeof window.fetch !== "function") return false;
    if (typeof window.FormData !== "function") return false;
    if (typeof window.DOMParser !== "function") return false;
    var action = form.getAttribute("action") || "";
    var body = formBody(form, control);
    if (!action || !body) return false;
    var request = {
      epoch: state.landingEpoch,
      controller: typeof window.AbortController === "function" ? new window.AbortController() : null
    };
    state.request = request;
    var responseAtMs = null;
    var timing = null;
    var options = {
      method: "POST",
      body: body,
      credentials: "same-origin",
      headers: {
        Accept: "text/html",
        "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
        "X-Requested-With": "scry-inplace"
      },
      redirect: "follow"
    };
    if (request.controller) options.signal = request.controller.signal;
    var fetchRequest;
    try {
      fetchRequest = window.fetch(action, options);
    } catch (error) {
      failRequest("The response could not be loaded. Your answer is still here; retry when ready.");
      return true;
    }
    fetchRequest
      .then(function (response) {
        if (state.request !== request) return null;
        responseAtMs = handoff ? absoluteEpochNow() : null;
        if (!response) throw new Error("missing response");
        if (response.status === 401 || response.status === 403) throw new Error("auth");
        // Error pages may omit the learner's input. Keep the original form
        // instead of swapping/reposting an ambiguous failed answer.
        if (action === "/app/submit" && !response.ok) throw new Error("answer unavailable");
        var type = response.headers && response.headers.get
          ? response.headers.get("content-type") || ""
          : "";
        if (type.indexOf("text/html") === -1) throw new Error("not html");
        if (handoff) timing = fetchServerTiming(response);
        return response.text();
      })
      .then(function (html) {
        if (state.request !== request) return;
        var receivedAtMs = handoff ? absoluteEpochNow() : null;
        var phases = handoff ? {
          requestToResponseMs: boundedDuration(handoff.startedAtMs, responseAtMs),
          transferMs: boundedDuration(responseAtMs, receivedAtMs)
        } : null;
        if (!applyInPlaceDocument(html, phases)) throw new Error("swap failed");
        resetState();
        if (handoff) {
          scheduleCompletion(handoff, "in_place", timing, phases, null, request.epoch);
        }
      })
      .catch(function () {
        if (state.request !== request) return;
        failRequest("The response could not be loaded. Your answer is still here; retry when ready.");
      });
    return true;
  }

  document.addEventListener("submit", function (event) {
    var form = event && event.target;
    if (!isNativeForm(form) || event.defaultPrevented) return;
    if (state.busy) {
      event.preventDefault();
      return;
    }

    state.landingEpoch += 1;
    state.landingAttempted = true;
    var reviewSubmit = isReviewSubmitForm(form);
    var startedAtMs = reviewSubmit ? absoluteEpochNow() : null;
    if (reviewSubmit) {
      var responseInput = form.querySelector('input[name="responseTimeMs"]');
      if (responseInput) {
        var elapsed = responseClockNow() - presentedAt;
        responseInput.value = String(Math.max(1, Math.round(elapsed)));
      }
    }

    var control = submitControl(form, event);
    setBusy(form, control);
    var handoff = reviewSubmit
      ? createHandoff(form, startedAtMs, absoluteEpochNow())
      : null;
    if (isInPlaceActionForm(form) && fetchInPlace(form, control, handoff)) {
      event.preventDefault();
      return;
    }
    storeHandoff(handoff);
  });

  function metaContent(name, source) {
    var root = source || document;
    if (typeof root.querySelectorAll !== "function") return null;
    var metas = root.querySelectorAll('meta[name="' + name + '"]');
    if (!metas || metas.length !== 1) return null;
    var value = metas[0].getAttribute("content");
    return typeof value === "string" && value ? value : null;
  }

  function navigationTiming() {
    if (!perf || typeof perf.getEntriesByType !== "function") return null;
    var entries = perf.getEntriesByType("navigation");
    if (!entries || entries.length !== 1) return null;
    var navigation = entries[0];
    if (!navigation || navigation.type === "back_forward") return null;
    if (!navigation.serverTiming || typeof navigation.serverTiming.length !== "number") return null;
    return navigation;
  }

  function serverTimingDescription(navigation, name) {
    var match = null;
    var found = false;
    for (var i = 0; i < navigation.serverTiming.length; i++) {
      var entry = navigation.serverTiming[i];
      if (!entry || entry.name !== name) continue;
      if (found) return null;
      found = true;
      match = entry.description;
    }
    return found && typeof match === "string" ? match : null;
  }

  function fetchServerTiming(response) {
    try {
      var header = response.headers && response.headers.get("server-timing");
      if (typeof header !== "string" || header.length > 4096) return null;
      var entries = header.split(",");
      var ids = {};
      for (var i = 0; i < entries.length; i++) {
        var entry = entries[i].trim();
        var name = entry.split(";")[0].trim();
        if (name !== "request" && name !== "handoff") continue;
        if (Object.prototype.hasOwnProperty.call(ids, name)) return null;
        var match = /^(?:request|handoff)\s*;\s*desc="([a-z0-9_]+)"\s*$/.exec(entry);
        if (!match) return null;
        ids[name] = match[1];
      }
      return ids;
    } catch (error) {
      return null;
    }
  }

  function consumeHandoff() {
    var store = storage();
    if (!store) return null;
    var raw;
    try {
      raw = store.getItem(HANDOFF_STORAGE_KEY);
      // Consume before validation/emission, so retries cannot duplicate a tap.
      store.removeItem(HANDOFF_STORAGE_KEY);
    } catch (error) {
      return null;
    }
    if (!raw) return null;
    var handoff;
    try {
      handoff = JSON.parse(raw);
    } catch (error) {
      return null;
    }
    if (!handoff || typeof handoff !== "object" || Array.isArray(handoff)) return null;
    var keys = Object.keys(handoff);
    if (
      keys.length !== 6 ||
      keys.indexOf("version") < 0 ||
      keys.indexOf("action") < 0 ||
      keys.indexOf("token") < 0 ||
      keys.indexOf("startedAtMs") < 0 ||
      keys.indexOf("acknowledgedAtMs") < 0 ||
      keys.indexOf("expiresAtMs") < 0
    ) {
      return null;
    }
    if (
      handoff.version !== HANDOFF_VERSION ||
      handoff.action !== HANDOFF_ACTION ||
      typeof handoff.token !== "string" ||
      !TRACE_ID_RE.test(handoff.token) ||
      !isSafeInteger(handoff.startedAtMs) ||
      !isSafeInteger(handoff.acknowledgedAtMs) ||
      !isSafeInteger(handoff.expiresAtMs) ||
      handoff.expiresAtMs !== handoff.startedAtMs + HANDOFF_TTL_MS ||
      handoff.acknowledgedAtMs < handoff.startedAtMs ||
      handoff.acknowledgedAtMs > handoff.expiresAtMs
    ) {
      return null;
    }
    return handoff;
  }

  function boundedDuration(start, end) {
    if (!isFiniteNumber(start) || !isFiniteNumber(end) || end < start) return null;
    var duration = Math.round(end - start);
    return duration >= 0 && duration <= MAX_DURATION_MS ? duration : null;
  }

  function navigationEpoch(relativeMs) {
    // Navigation Timing uses zero when a phase is unavailable. It is not an
    // observed instant, and every timestamp is already relative to timeOrigin.
    if (!isFiniteNumber(relativeMs) || relativeMs <= 0 || !perf || !isFiniteNumber(perf.timeOrigin)) return null;
    var epoch = perf.timeOrigin + relativeMs;
    return isSafeInteger(Math.round(epoch)) ? Math.round(epoch) : null;
  }

  function viewportClass() {
    if (!isFiniteNumber(window.innerWidth) || window.innerWidth < 0) return null;
    if (window.innerWidth < 600) return "mobile";
    if (window.innerWidth < 1024) return "tablet";
    return "desktop";
  }

  function scheduleCompletion(handoff, navigation, timing, phases, responseEndAtMs, scheduledEpoch) {
    if (!window.fetch || typeof window.fetch !== "function") return;
    if (!window.requestAnimationFrame || typeof window.requestAnimationFrame !== "function") return;
    var verdict = document.querySelector(".me-verdict");
    var requestId = metaContent("memory-engine-submit-request");
    var renderedTraceId = metaContent("memory-engine-submit-handoff");
    var csrfToken = metaContent("memory-engine-csrf-token");
    if (
      !verdict ||
      !timing ||
      !REQUEST_ID_RE.test(requestId || "") ||
      !TRACE_ID_RE.test(renderedTraceId || "") ||
      !csrfToken ||
      timing.request !== requestId ||
      timing.handoff !== renderedTraceId ||
      handoff.token !== renderedTraceId
    ) return;
    var now = absoluteEpochNow();
    if (now === null || now > handoff.expiresAtMs) return;

    var attempted = false;
    function stillLive() {
      return (
        !attempted &&
        state.landingEpoch === scheduledEpoch &&
        document.visibilityState !== "hidden" &&
        document.querySelector(".me-verdict") === verdict &&
        metaContent("memory-engine-submit-request") === requestId &&
        metaContent("memory-engine-submit-handoff") === handoff.token &&
        metaContent("memory-engine-csrf-token") === csrfToken
      );
    }
    if (!stillLive()) return;
    window.requestAnimationFrame(function () {
      if (!stillLive()) return;
      window.requestAnimationFrame(function () {
        if (!stillLive()) return;
        attempted = true;
        var visibleAtMs = absoluteEpochNow();
        if (visibleAtMs === null || visibleAtMs > handoff.expiresAtMs) return;
        var tapToAckMs = boundedDuration(handoff.startedAtMs, handoff.acknowledgedAtMs);
        var gradedVisibleMs = boundedDuration(handoff.startedAtMs, visibleAtMs);
        var viewport = viewportClass();
        if (
          tapToAckMs === null ||
          gradedVisibleMs === null ||
          tapToAckMs > gradedVisibleMs ||
          !viewport
        ) return;
        var payload = {
          schema: "memory_engine.browser_submit.v2",
          csrfToken: csrfToken,
          requestId: requestId,
          traceId: handoff.token,
          navigation: navigation,
          tapToAckMs: tapToAckMs,
          gradedVisibleMs: gradedVisibleMs,
          viewport: viewport
        };
        var names = ["requestToResponseMs", "transferMs", "domSwapMs"];
        for (var i = 0; i < names.length; i++) {
          var duration = phases[names[i]];
          if (isSafeInteger(duration) && duration <= gradedVisibleMs) {
            payload[names[i]] = duration;
          }
        }
        if (navigation === "full_page") {
          var navigationMs = boundedDuration(responseEndAtMs, visibleAtMs);
          if (navigationMs !== null && navigationMs <= gradedVisibleMs) {
            payload.navigationMs = navigationMs;
          }
        }
        try {
          var request = window.fetch("/app/performance/submit", {
            method: "POST",
            credentials: "same-origin",
            keepalive: true,
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload)
          });
          if (request && typeof request.catch === "function") request.catch(function () {});
        } catch (error) {
          // Best effort and never retried: the learner already has the result.
        }
      });
    });
  }

  function emitLandingTelemetry() {
    if (state.landingAttempted) return;
    state.landingAttempted = true;
    var handoff = consumeHandoff();
    if (!handoff) return;
    var navigation = navigationTiming();
    if (!navigation) return;
    var responseStartAtMs = navigationEpoch(navigation.responseStart);
    var responseEndAtMs = navigationEpoch(navigation.responseEnd);
    scheduleCompletion(handoff, "full_page", {
      request: serverTimingDescription(navigation, "request"),
      handoff: serverTimingDescription(navigation, "handoff")
    }, {
      requestToResponseMs: boundedDuration(handoff.startedAtMs, responseStartAtMs),
      transferMs: boundedDuration(responseStartAtMs, responseEndAtMs)
    }, responseEndAtMs, state.landingEpoch);
  }

  window.addEventListener("pagehide", function () {
    abandonRequest();
  });
  window.addEventListener("pageshow", function (event) {
    if (event && event.persisted) {
      removeHandoff();
      abandonRequest();
      state.landingAttempted = true;
      return;
    }
    emitLandingTelemetry();
  });
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "hidden") state.landingEpoch += 1;
  });
})();

// Progressive enhancement for Create: an immediate in-page pending state.
//
// The server posts the plain form either way, so with JavaScript off (or if
// this script fails) capture still works exactly as before. This script only
// gives the tap instant feedback: disable the submit button and swap its
// label to a working state, so a slow network never looks inert and a
// second tap can never fire a duplicate capture.
(function () {
  "use strict";
  var form = document.querySelector("form.me-capture-form");
  if (!form) return;
  form.addEventListener("submit", function () {
    var button = form.querySelector('button[type="submit"]');
    if (!button || button.disabled) return;
    button.disabled = true;
    button.textContent = "Creating quizzes…";
    var status = form.querySelector(".me-live-hint");
    if (status) status.textContent = "Creating drafts for you to inspect before adding quizzes.";
  });
})();

// Acknowledge the one entry request before the native POST settles. The
// server still owns email normalization, admission, persistence, delivery,
// and the final response; this enhancement only prevents a second tap.
(function () {
  "use strict";
  var form = document.querySelector("form.me-entry-form");
  if (!form) return;
  form.addEventListener("submit", function () {
    var button = form.querySelector('button[type="submit"]');
    if (!button || button.disabled) return;
    button.disabled = true;
    button.textContent = "Checking…";
    var status = form.querySelector(".me-entry-status");
    if (status) status.textContent = "Checking…";
  });
})();

// Progressive enhancement for the generation activity log.
//
// The server renders the authoritative list of jobs on every full page load,
// so with JavaScript off (or if this script fails) the page is still correct
// and a normal navigation refreshes it. This script only *enhances* that list:
// it opens an SSE stream and patches a single <li> in place as each job's
// status changes — no framework, no full-list rebuild, no lost scroll.
(function () {
  "use strict";
  if (!("EventSource" in window)) return;


  // The human meta line, kept in sync with `job_meta` in render.rs.
  function metaFor(job) {
    switch (job.status) {
      case "queued":
        return "Queued for generation.";
      case "running":
        return "Generating quizzes…";
      case "retry":
        return "Retrying after a temporary failure…";
      case "succeeded":
        return "Generation finished. Check Library for drafts and notices.";
      case "failed":
        return job.error || "Generation failed. Try again.";
      default:
        return "";
    }
  }

  // Build a minimal row when a job arrives that isn't on the page yet (e.g. a
  // capture made in another tab). It carries status + meta only — the retry
  // control needs a server-issued CSRF token, so a job that fails here gets its
  // Retry button on the next full page load (the list is server-authoritative).
  function createRow(job) {
    var li = document.createElement("li");
    li.className = "me-job";
    li.dataset.jobId = job.id;
    li.innerHTML =
      '<span class="me-job-glyphs" aria-hidden="true">' +
      '<span class="g-queued"></span><span class="g-running"><span class="me-spinner"></span></span>' +
      '<span class="g-succeeded"></span><span class="g-failed"></span></span>' +
      '<div class="me-job-body"><p class="me-job-title"></p><p class="me-job-meta"></p></div>';
    li.querySelector(".me-job-title").textContent = job.title || "New material";
    return li;
  }

  function apply(list, job) {
    var li = list.querySelector('li[data-job-id="' + cssEscape(job.id) + '"]');
    if (!li) {
      li = createRow(job);
      list.insertBefore(li, list.firstChild);
    }
    li.dataset.status = job.status;
    var meta = li.querySelector(".me-job-meta");
    if (meta) meta.textContent = metaFor(job);
  }

  function cssEscape(value) {
    return window.CSS && CSS.escape ? CSS.escape(value) : String(value).replace(/"/g, '\\"');
  }

  var source = new EventSource("/app/jobs/events");
  var terminalNavigationStarted = false;
  // EventSource reconnects automatically; no manual retry needed.
  source.addEventListener("job", function (event) {
    try {
      // Resolve the live surface after every in-place swap. Library contains
      // editable forms: activity may update there, but never navigate over work.
      var list = document.getElementById("me-jobs");
      var waiting = document.querySelector("[data-generation-job-id][data-terminal-url]");
      if (!list && !waiting) return;
      var job = JSON.parse(event.data);
      if (list) apply(list, job);
      if (!waiting || waiting.getAttribute("data-generation-job-id") !== job.id) return;
      var status = waiting.querySelector("[data-generation-status]");
      if (status) status.textContent = metaFor(job);
      if (
        terminalNavigationStarted ||
        (job.status !== "succeeded" && job.status !== "failed")
      ) return;
      // Only an explicit, job-correlated waiting surface opts into a GET to
      // the server-owned destination. Replayed terminal events cannot repost
      // capture or redirect an unrelated quiz, draft edit, or new capture.
      var destination = waiting.getAttribute("data-terminal-url");
      if (
        !destination ||
        destination.charAt(0) !== "/" ||
        destination.charAt(1) === "/" ||
        destination.indexOf("\\") !== -1
      ) return;
      terminalNavigationStarted = true;
      window.location.assign(destination);
    } catch (err) {
      /* ignore a malformed frame; the next event or navigation corrects it */
    }
  });
})();
