// Scry's progressive enhancement. Every page works without this file;
// with it, the keyboard does more and the study session never reloads.
(function () {
  "use strict";

  var doc = document;
  var $ = function (sel, root) { return (root || doc).querySelector(sel); };
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || doc).querySelectorAll(sel)); };
  var typing = function (el) {
    return el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable);
  };

  // ---- Swap <main> from a fetched page -----------------------------------

  function swap(html, url) {
    var next = new DOMParser().parseFromString(html, "text/html");
    var main = $("main", next);
    if (!main) { location.href = url; return; }
    $("main").innerHTML = main.innerHTML;
    doc.title = next.title;
    doc.body.className = next.body.className;
    var nav = $(".nav", next), here = $(".nav");
    if (nav && here) here.innerHTML = nav.innerHTML;
    if (url && url !== location.href) history.replaceState(null, "", url);
    init();
    var focus = $("main [autofocus]");
    if (focus) focus.focus();
  }

  function post(form, extra) {
    var body = new URLSearchParams(new FormData(form));
    if (extra) Object.keys(extra).forEach(function (k) { body.set(k, extra[k]); });
    return fetch(form.action, {
      method: "POST", body: body, credentials: "same-origin",
      headers: { "X-Requested-With": "fetch" }
    }).then(function (res) {
      return res.text().then(function (html) { return { html: html, url: res.url }; });
    });
  }

  // ---- Global keys -------------------------------------------------------

  var go = { s: "/study", l: "/library", i: "/inbox", n: "/learn", b: "/billing" };
  var pendingG = 0;
  doc.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || typing(e.target)) return;
    if (!$(".nav a[href='/study']")) return; // signed out
    if (pendingG && Date.now() - pendingG < 1200 && go[e.key]) {
      pendingG = 0;
      e.preventDefault();
      location.href = go[e.key];
      return;
    }
    pendingG = e.key === "g" ? Date.now() : 0;
    if (e.key === "/") {
      var search = $("[data-search]");
      if (search) { e.preventDefault(); search.focus(); search.select(); }
    }
  });

  // ---- Time zone ---------------------------------------------------------

  function initTZ() {
    var tz = "";
    try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ""; } catch (err) { /* keep UTC */ }
    $$("[data-tz-form] input[name=tz]").forEach(function (el) { el.value = tz; });
  }

  // ---- Starter shelf: the adding state -----------------------------------

  function initShelf() {
    $$("form[data-add]").forEach(function (form) {
      form.addEventListener("submit", function () {
        var b = $("button", form);
        b.textContent = "Adding…";
        // Disable after the submit has been dispatched.
        setTimeout(function () { b.disabled = true; }, 0);
      });
    });
  }

  // ---- Study -------------------------------------------------------------

  var voice = null; // the recording in progress, if any

  function initStudy() {
    var form = $("[data-answer-form]");
    var nextForm = $("[data-next-form]");
    if (nextForm) initNext(nextForm);
    if (!form) return;

    var input = $("#answer", form);
    var line = $("[data-answer-line]", form);
    var caret = $("[data-caret]", form);
    var status = $("[data-status]", form);
    var via = $("[data-via]", form);
    var mic = $("[data-mic]", form);
    var busy = false;

    // Block caret: a block that sits just after the text while the cursor
    // is at the end; otherwise the native caret takes over.
    var ruler = doc.createElement("span");
    ruler.className = "display";
    ruler.style.cssText = "position:absolute;visibility:hidden;white-space:pre;left:0;top:0";
    line.appendChild(ruler);
    doc.documentElement.classList.add("has-caret");
    function placeCaret() {
      var atEnd = input.selectionStart === input.value.length && input.selectionEnd === input.value.length;
      ruler.textContent = input.value;
      var x = ruler.getBoundingClientRect().width;
      var fits = x < input.clientWidth - 8;
      line.classList.toggle("caret-off", !(atEnd && fits));
      caret.style.left = x + "px";
    }
    ["input", "keyup", "click", "focus", "select"].forEach(function (ev) { input.addEventListener(ev, placeCaret); });
    if (doc.fonts && doc.fonts.ready) doc.fonts.ready.then(placeCaret);
    placeCaret();

    function say(text) { status.textContent = text || ""; }

    input.addEventListener("input", function () {
      if (!voice) via.value = "typed";
    });

    // Space on an empty answer line starts listening; a second press stops.
    input.addEventListener("keydown", function (e) {
      if (e.key === " " && (voice || input.value === "")) {
        e.preventDefault();
        toggleVoice();
      }
    });
    mic.addEventListener("click", function () { toggleVoice(); input.focus(); });

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      var send = function () {
        busy = true;
        // Checking is instant; the tag only appears if it takes over 400ms.
        var slow = setTimeout(function () { say("checking"); }, 400);
        post(form).then(function (r) {
          clearTimeout(slow);
          swap(r.html, r.url);
        }).catch(function () {
          clearTimeout(slow);
          busy = false;
          say("Couldn't reach Scry. Check your connection and press Enter to try again.");
        });
      };
      if (voice) stopVoice().then(send); else send();
    });

    function toggleVoice() {
      if (voice) { stopVoice(); return; }
      if (!navigator.mediaDevices || !window.MediaRecorder) {
        say("This browser can't record. Type your answer instead.");
        return;
      }
      navigator.mediaDevices.getUserMedia({ audio: true }).then(function (stream) {
        startVoice(stream);
      }).catch(function (err) {
        if (err && (err.name === "NotAllowedError" || err.name === "SecurityError")) {
          say("The microphone is blocked. Allow it for this site from the address bar, then press space again.");
        } else {
          say("No microphone was found. Type your answer instead.");
        }
      });
    }

    function startVoice(stream) {
      var rec = new MediaRecorder(stream);
      voice = { rec: rec, stream: stream, chunks: [], started: Date.now(), seq: 0, shown: 0, done: null };
      form.classList.add("is-listening");
      mic.setAttribute("aria-pressed", "true");
      say("listening");
      rec.addEventListener("dataavailable", function (e) {
        if (e.data && e.data.size) voice.chunks.push(e.data);
        if (rec.state === "recording") hear(false);
      });
      rec.start(1200); // a slice every 1.2s gives live text
      voice.timer = setTimeout(function () { stopVoice(); }, 20000);
    }

    // hear sends what has been recorded so far and shows the words heard.
    function hear(final) {
      var v = voice;
      if (!v || !v.chunks.length) return Promise.resolve("");
      var seq = ++v.seq;
      var blob = new Blob(v.chunks, { type: v.rec.mimeType || "audio/webm" });
      return fetch("/study/speech", {
        method: "POST", body: blob, credentials: "same-origin",
        headers: {
          "Content-Type": blob.type || "audio/webm",
          "X-CSRF-Token": form.elements.csrf.value,
          "X-Audio-Seconds": String((Date.now() - v.started) / 1000)
        }
      }).then(function (res) { return res.json().then(function (j) { return { ok: res.ok, j: j }; }); })
        .then(function (r) {
          if (!r.ok) { v.error = r.j.error || "Scry can't listen just now. Type your answer instead."; return ""; }
          if (seq > v.shown && (final || voice === v)) {
            v.shown = seq;
            if (r.j.text) { input.value = r.j.text; via.value = "spoken"; placeCaret(); }
          }
          return r.j.text || "";
        }).catch(function () { v.error = "Couldn't reach Scry. Type your answer instead."; return ""; });
    }

    function stopVoice() {
      var v = voice;
      if (!v) return Promise.resolve();
      if (v.done) return v.done;
      clearTimeout(v.timer);
      v.done = new Promise(function (resolve) {
        v.rec.addEventListener("stop", function () {
          v.stream.getTracks().forEach(function (t) { t.stop(); });
          hear(true).then(function (text) {
            voice = null;
            form.classList.remove("is-listening");
            mic.setAttribute("aria-pressed", "false");
            if (v.error) say(v.error);
            else if (!text && !input.value) say("Didn't catch that. Try again or type it.");
            else say("heard · enter to check");
            resolve();
          });
        });
        if (v.rec.state !== "inactive") v.rec.stop(); else v.rec.dispatchEvent(new Event("stop"));
      });
      return v.done;
    }
  }

  function initNext(form) {
    var again = $("[data-again]", form);
    var busy = false;
    form.addEventListener("submit", function (e) {
      e.preventDefault();
      if (busy) return;
      busy = true;
      var extra = e.submitter && e.submitter.name === "again" ? { again: "1" } : null;
      post(form, extra).then(function (r) { swap(r.html, r.url); }).catch(function () { form.submit(); });
    });
    form.addEventListener("keydown", function (e) {
      if (again && (e.key === "a" || e.key === "A")) { e.preventDefault(); again.click(); }
    });
  }

  // ---- Inbox and drafts --------------------------------------------------

  function initRows() {
    var rows = $$("[data-row]");
    if (!rows.length) return;
    rows.forEach(function (row) {
      row.addEventListener("keydown", function (e) {
        if (e.target !== row || e.metaKey || e.ctrlKey || e.altKey) return;
        var all = $$("[data-row]");
        var i = all.indexOf(row);
        var key = e.key.toLowerCase();
        if (key === "j" || e.key === "ArrowDown") { e.preventDefault(); (all[i + 1] || row).focus(); }
        else if (key === "k" || e.key === "ArrowUp") { e.preventDefault(); (all[i - 1] || row).focus(); }
        else if (key === "a") { e.preventDefault(); decide(row, $("form[data-approve]", row), i); }
        else if (key === "d") { e.preventDefault(); decide(row, $("form[data-dismiss]", row), i); }
      });
      $$("form", row).forEach(function (form) {
        form.addEventListener("submit", function (e) {
          e.preventDefault();
          decide(row, form, $$("[data-row]").indexOf(row));
        });
      });
    });
  }

  // decide approves or dismisses a row: it collapses upward, then the
  // page is brought in line with the server.
  function decide(row, form, index) {
    if (row.dataset.busy) return;
    row.dataset.busy = "1";
    row.style.maxHeight = row.offsetHeight + "px";
    row.offsetHeight; // settle the starting height before collapsing
    row.classList.add("is-leaving");
    var wait = new Promise(function (resolve) { setTimeout(resolve, 160); });
    Promise.all([post(form), wait]).then(function (r) {
      swap(r[0].html, r[0].url);
      var all = $$("[data-row]");
      if (all.length) all[Math.min(index, all.length - 1)].focus();
    }).catch(function () { form.submit(); });
  }

  // A draft being written: bring in the ideas as they are ready.
  var polling = 0;
  function initDraft() {
    clearTimeout(polling);
    if (!$("[data-drafting]")) return;
    polling = setTimeout(function () {
      fetch(location.href, { credentials: "same-origin" }).then(function (res) { return res.text(); }).then(function (html) {
        if (!$("[data-drafting]")) return;
        var focused = doc.activeElement && doc.activeElement.closest && doc.activeElement.closest("[data-row]");
        var index = focused ? $$("[data-row]").indexOf(focused) : -1;
        swap(html, location.href);
        if (index >= 0) { var all = $$("[data-row]"); if (all[index]) all[index].focus(); }
      }).catch(function () { initDraft(); });
    }, 700);
  }

  function init() {
    initTZ();
    initShelf();
    initStudy();
    initRows();
    initDraft();
  }

  init();
})();
