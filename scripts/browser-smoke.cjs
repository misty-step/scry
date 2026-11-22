// Repeatable Chromium smoke of the learner's main path, in Playwright with
// the system Chrome (npm ci in scripts/; nothing is downloaded). It drives
// the Go test server TestReviewBrowserSmoke starts, whose model is scripted
// to judge the first answer right and the second missed. Only that server is
// allowed: any other request fails the run.
//
//   SCRY_BROWSER_SMOKE=1 go test ./internal/web -run TestReviewBrowserSmoke -v
//
// It checks, in a real browser: the empty-library Study screen, the card
// step's processing state, the starter shelf (headline, order, in-place
// add with no scroll jump), the spoiler-free Study rail, voice answers, the
// post-answer review for a correct and an incorrect answer (no auto-skip,
// both answers, verdict, Ask Why?, schedule facts, inline edit, Next card
// by button, Enter and Space), exact texts, the navigation keys and
// Settings; with no console errors, page errors, failed or external
// requests, CSP violations or sideways scroll at desktop and phone widths
// in light and dark.
const { chromium } = require("playwright-core");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const base = process.argv[2];
const out = process.env.SCRY_SMOKE_DIR || "/tmp/scry-remediation-smoke";
const pause = ms => new Promise(r => setTimeout(r, ms));

(async () => {
  fs.mkdirSync(out, { recursive: true });
  const browser = await chromium.launch({
    executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome", headless: true,
    args: ["--no-sandbox", "--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required"],
  });
  const errors = [];
  let speech = 0, explains = 0, edits = 0, adds = 0;
  // newPage opens a fresh learner's browser that may only talk to the server.
  const newPage = async () => {
    const context = await browser.newContext({ viewport: { width: 1280, height: 1000 }, permissions: ["microphone"] });
    await context.addInitScript(() => {
      window.__csp = [];
      document.addEventListener("securitypolicyviolation", e => window.__csp.push(e.violatedDirective + " " + e.blockedURI), true);
    });
    const page = await context.newPage();
    page.on("console", m => { if (m.type() === "error") errors.push("console: " + m.text()); });
    page.on("response", r => { if (r.status() >= 400) errors.push("http " + r.status() + " " + r.url()); });
    page.on("pageerror", e => errors.push("pageerror: " + e.message));
    page.on("requestfailed", r => { if (!/ERR_ABORTED/.test(r.failure()?.errorText || "")) errors.push("failed: " + r.url()); });
    await page.route("**/*", route => {
      const url = route.request().url();
      if (url.startsWith(base + "/")) {
        if (url.endsWith("/study/speech")) speech++;
        if (url.endsWith("/study/explain")) explains++;
        if (url.endsWith("/study/edit")) edits++;
        if (url.endsWith("/shelf/add")) adds++;
        return route.continue();
      }
      if (url.startsWith("data:")) return route.continue();
      errors.push("external request: " + url);
      return route.abort();
    });
    return page;
  };
  const noCSP = async (p, label) => assert.deepEqual(await p.evaluate(() => window.__csp || []), [], label + ": CSP violations");
  const noOverflow = async (p, label) => {
    const m = await p.evaluate(() => ({ doc: document.documentElement.scrollWidth, win: innerWidth }));
    assert(m.doc <= m.win, label + ": sideways scroll " + JSON.stringify(m));
  };
  // word reports whether text shows word as a whole word, whatever its case.
  const word = (text, w) => new RegExp("(^|[^\\p{L}\\p{N}])" + w + "($|[^\\p{L}\\p{N}])", "iu").test(text);
  try {
    const page = await newPage();
    const go = async path => { const r = await page.goto(base + path, { waitUntil: "networkidle" }); assert.equal(r.status(), 200, path); };
    const submit = async selector => Promise.all([page.waitForNavigation({ waitUntil: "networkidle" }), page.click(selector)]);
    const text = (sel, p = page) => p.$eval(sel, e => e.textContent.replace(/\s+/g, " ").trim());
    // Sign-up starts the 7-day no-card trial and lands straight on the
    // starter shelf, with the trial chip in the chrome; no card step.
    const onTrialShelf = async (p, label) => {
      assert.equal(new URL(p.url()).pathname, "/shelf", label + ": sign-up must land on /shelf, not the card step");
      const chip = await p.$eval("[data-trial]", e => ({ text: e.textContent.trim(), href: e.getAttribute("href"), shown: e.getClientRects().length > 0 }));
      assert(chip.shown && chip.text.startsWith("Trial · 7 days left") && chip.href === "/signup/card", label + ": trial chip " + JSON.stringify(chip));
      assert.equal(await p.$("#number"), null, label + ": no card fields on the shelf");
      await p.waitForSelector("#nato-alphabet");
    };
    await go("/signup"); await page.fill("#email", "browser@example.com"); await page.fill("#password", "browser-password");
    await submit("main form button");
    await onTrialShelf(page, "signup");

    // Slice 41 (3): Study with an empty library is its own screen, reached
    // by pressing Study, never a silent redirect back to the library.
    await submit(".nav a[href='/study']");
    assert.equal(new URL(page.url()).pathname, "/study", "empty library: Study stays on /study");
    assert.equal(await text("[data-study-empty] h1"), "No cards to study yet.");
    assert.deepEqual(await page.$$eval("[data-study-empty] .actions a", as => as.map(a => [a.textContent.trim(), a.getAttribute("href")])),
      [["Pick a starter set", "/shelf"], ["Create cards with AI", "/learn"]]);
    // Slice 41 (4): Study and Create are told apart: label, icon, purpose.
    const nav = await page.$$eval(".nav a", as => as.map(a => ({ label: a.querySelector("span").textContent, href: a.getAttribute("href"), icon: (a.querySelector("svg[aria-hidden=true]") || {}).className?.baseVal || "" })));
    assert.deepEqual(nav.map(n => n.label), ["Study", "Library", "Inbox", "Create", "Billing", "Settings"]);
    assert(nav.every(n => n.icon.includes("icon-")), "every destination has an icon: " + JSON.stringify(nav));
    assert.notEqual(nav[0].icon, nav[3].icon, "Study and Create have different icons");
    assert.equal(nav[3].href, "/learn");
    assert((await text("[data-keys]")).includes("g n create"));
    await page.screenshot({ path: out + "/study-empty.png" });

    // Slice 41 (7): the card step shows it is working the moment it is
    // submitted: the button is disabled and busy, with words and a spinner.
    await go("/signup/card");
    assert((await page.$eval(".trial-terms", e => e.textContent)).includes("Nothing is charged today"));
    assert.equal(await page.$eval('input[name="billing_terms"]', e => e.value), "trial");
    assert(await page.$eval("[data-busy]", e => e.hidden), "no processing state before submitting");
    await page.fill("#number", "4242424242424242"); await page.fill("#expiry", "12/30"); await page.fill("#cvc", "123");
    // The state is read from inside the page 150 ms after the submit, while
    // the request is still in flight (it is held back for 700 ms here), and
    // kept in sessionStorage across the navigation: a page cannot be
    // inspected from outside while its navigation is paused.
    let cardPosts = 0;
    await page.route(base + "/signup/card", async route => {
      if (route.request().method() === "POST") { cardPosts++; await pause(700); }
      await route.continue();
    });
    await page.$eval("[data-card-form]", f => f.addEventListener("submit", () => setTimeout(() => {
      const b = f.querySelector("button"), box = f.querySelector("[data-busy]"), again = new Event("submit", { cancelable: true });
      f.dispatchEvent(again); // a second submit while busy
      sessionStorage.setItem("busy", JSON.stringify({
        disabled: b.disabled, aria: b.getAttribute("aria-busy"), form: f.getAttribute("aria-busy"), shown: !box.hidden && box.getClientRects().length > 0,
        text: box.textContent.trim(), spinner: !!box.querySelector("svg.icon-spinner"), bar: !!box.querySelector(".busy-bar"),
        status: box.querySelector("[role=status]") !== null, secondSubmitRefused: again.defaultPrevented,
        spinning: getComputedStyle(box.querySelector("svg.icon-spinner")).animationName, dimmed: getComputedStyle(f.querySelector(".field")).opacity < 1,
      }));
    }, 150)));
    await submit("[data-card-form] button");
    await page.unroute(base + "/signup/card");
    assert.deepEqual(JSON.parse(await page.evaluate(() => sessionStorage.getItem("busy"))),
      { disabled: true, aria: "true", form: "true", shown: true, text: "Confirming your card…", spinner: true, bar: true, status: true, secondSubmitRefused: true, spinning: "spin", dimmed: true },
      "the card step's processing state");
    assert.equal(cardPosts, 1, "one card submission");
    assert.equal(new URL(page.url()).pathname, "/shelf", "card added: back to the shelf");
    assert.equal(await page.$("[data-trial]"), null, "no no-card trial chip once a card is on file");

    // Slice 41 (6, 9): the shelf's headline, its first row and its tiles.
    assert.equal(await text(".shelf-head h1"), "Pick a deck and start recalling in seconds.");
    const tiles = await page.$$eval("[data-tile]", ts => ts.map(t => ({ id: t.id, top: Math.round(t.getBoundingClientRect().top), icons: t.querySelectorAll("svg[aria-hidden=true]").length, count: t.querySelector(".tile-count .num").textContent })));
    assert.equal(tiles.length, 30);
    const row1 = tiles.filter(t => t.top === tiles[0].top).map(t => t.id);
    assert.deepEqual(row1, ["nato-alphabet", "mental-models", "kitchen-numbers", "world-geography"], "row 1");
    assert(!row1.some(id => /creed|passages/.test(id)), "no Creeds in row 1");
    assert(tiles.findIndex(t => t.id === "the-creeds") >= 28, "the Creeds are kept, at the end of the shelf");
    assert(tiles.every(t => t.icons >= 3 && /^\d+$/.test(t.count)), "every tile has its subject icon, a card-stack count and an Add icon");
    // Slice 41 (5): adding a set far down the shelf confirms in place.
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.$eval("#great-books", e => e.scrollIntoView({ block: "center" }));
    await page.evaluate(() => { window.__sameDocument = true; });
    const before = await page.evaluate(() => scrollY);
    assert(before > 400, "the tile is below the fold: scrollY " + before);
    await page.click("#great-books [data-add] button");
    await page.waitForSelector("#great-books [data-added]");
    const after = await page.evaluate(() => ({ y: scrollY, same: window.__sameDocument === true, added: document.querySelector("#great-books").classList.contains("is-added"), form: !!document.querySelector("#great-books form"), bar: !document.querySelector("[data-shelf-bar]").hidden, summary: document.querySelector("[data-shelf-summary]").textContent.trim(), check: !!document.querySelector("#great-books [data-added] svg.icon-check") }));
    assert.deepEqual(after, { y: before, same: true, added: true, form: false, bar: true, summary: "1 set added · 24 cards", check: true }, "in-place add");
    assert.equal(adds, 1); assert.equal(new URL(page.url()).pathname + new URL(page.url()).search, "/shelf");
    const bar = await page.$eval("[data-shelf-bar] a", a => { const r = a.getBoundingClientRect(); return { text: a.textContent.trim(), href: a.getAttribute("href"), visible: r.top >= 0 && r.bottom <= innerHeight }; });
    assert.deepEqual(bar, { text: "Start studying", href: "/study", visible: true }, "the Start studying bar is on screen without scrolling");
    await page.screenshot({ path: out + "/shelf-added-in-place.png" });
    await noCSP(page, "shelf");

    // The learner on this path studies the NATO set alone.
    const study = await newPage();
    const sgo = async path => { const r = await study.goto(base + path, { waitUntil: "networkidle" }); assert.equal(r.status(), 200, path); };
    await sgo("/signup"); await study.fill("#email", "study-browser@example.com"); await study.fill("#password", "browser-password");
    await Promise.all([study.waitForNavigation({ waitUntil: "networkidle" }), study.click("main form button")]);
    await onTrialShelf(study, "second signup");
    await study.click("#nato-alphabet [data-add] button"); await study.waitForSelector("#nato-alphabet [data-added]");
    await Promise.all([study.waitForNavigation({ waitUntil: "networkidle" }), study.click("[data-shelf-bar] a")]);
    assert.equal(new URL(study.url()).pathname, "/study");
    const s = study; // the rest of the study path runs in this browser

    // Slice 41 (1): nothing on the Study screen names an answer.
    const answers = ["Alfa", "Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India", "Juliett", "Romeo", "Sierra", "Zulu"];
    const spoilers = async label => {
      const seen = await s.evaluate(() => ({ rail: document.querySelector(".session-margin").textContent, header: document.querySelector("[data-study-header]").textContent.trim(), main: document.querySelector("main").innerText, title: document.title, html: document.querySelector("main").innerHTML }));
      for (const a of answers) {
        assert(!word(seen.rail, a), label + ": the rail shows " + a + ": " + seen.rail);
        assert(!word(seen.main, a), label + ": the page shows " + a);
        assert(!word(seen.title, a), label + ": the title shows " + a);
      }
      assert(!/Alfa|India|Letters A/.test(seen.html), label + ": an answer or section title is in the markup");
      return seen.header;
    };
    assert.equal(await spoilers("first card"), "NATO phonetic alphabet · Set 1");
    assert.equal(await text(".margin-purpose", s), "Recall what's due today");
    for (const width of [1280, 390]) for (const theme of ["light", "dark"]) {
      await s.setViewportSize({ width, height: 1000 });
      await s.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
      const metrics = await s.$eval("[data-check]", b => {
        const st = getComputedStyle(b);
        const lum = color => { const v = color.match(/[\d.]+/g).slice(0, 3).map(Number).map(x => { x /= 255; return x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; }); return .2126 * v[0] + .7152 * v[1] + .0722 * v[2]; };
        const a = lum(st.color), c = lum(st.backgroundColor);
        return { contrast: (Math.max(a, c) + .05) / (Math.min(a, c) + .05), height: b.getBoundingClientRect().height };
      });
      assert(metrics.contrast >= 4.5 && metrics.height >= 44, JSON.stringify(metrics));
      await noOverflow(s, "study " + width + " " + theme);
      await s.screenshot({ path: out + "/study-" + width + "-" + theme + ".png", fullPage: true });
    }
    await s.setViewportSize({ width: 1280, height: 1000 });

    // Voice: start and immediately stop gives DESIGN's "couldn't hear" words.
    await s.keyboard.press("Escape"); await s.keyboard.press("Space"); await s.waitForSelector(".is-listening");
    await s.keyboard.press("Space");
    await s.waitForFunction(() => !document.querySelector(".is-listening"));
    assert.equal(await s.$eval("[data-status]", e => e.textContent), "Didn't catch that. Try again or type it.");
    assert.equal(speech, 1); assert.equal(await s.$eval("#answer", e => e.value), "");
    await s.keyboard.press("Escape"); await s.keyboard.press("Space"); await s.waitForSelector(".is-listening");
    await pause(1700); assert.equal(speech, 1, "must not upload while recording");
    assert.equal(await s.$eval("#answer", e => e.value), "", "no live transcription");
    assert((await s.$eval("[data-status]", e => e.textContent)).includes("listening"));
    await s.keyboard.press("Space");
    await s.waitForFunction(() => !document.querySelector(".is-listening"));
    assert((await s.$eval("[data-status]", e => e.textContent)).includes("heard"));
    assert.equal(speech, 2);
    const spoken = await s.$eval("#answer", e => e.value); assert(spoken.length > 0);

    // Slice 41 (2): a correct answer stops on its review; nothing skips ahead.
    const review = () => s.evaluate(() => {
      const q = sel => document.querySelector(sel), t = sel => (q(sel)?.textContent || "").replace(/\s+/g, " ").trim();
      return {
        kind: q("[data-review]")?.dataset.kind, verdict: q("[data-verdict]")?.dataset.verdict, label: t("[data-verdict-label]"),
        badgeIcon: !!q("[data-verdict] svg[aria-hidden=true]"), why: t(".verdict .why"), prompt: t("[data-prompt]"),
        submitted: t("[data-submitted]"), canonical: t("[data-canonical]"), askWhy: t("[data-explain-label]"),
        next: t("[data-next]"), nextReview: t("[data-next-review]"), iso: q("[data-next-review] time")?.getAttribute("datetime"),
        interval: t("[data-interval]"), stability: t("[data-stability]"), difficulty: t("[data-difficulty]"), reviews: t("[data-reviews]"),
        total: q("[data-reviews]")?.dataset.reviewTotal, edit: t("[data-edit-toggle]"), asking: !!q("[data-answer-form]"),
        header: t("[data-study-header]"), count: t(".margin-count"),
      };
    });
    await s.keyboard.press("Enter"); await s.waitForSelector("[data-review]");
    let r = await review();
    assert.equal(r.kind, "right"); assert.equal(r.verdict, "right"); assert.equal(r.label, "Correct"); assert(r.badgeIcon);
    assert.equal(r.why, "Your answer expresses the same idea.");
    assert.equal(r.submitted, spoken); assert.equal(r.canonical, "Alfa"); assert.equal(r.prompt, "NATO alphabet: A");
    assert.equal(r.askWhy, "Ask Why?"); assert.equal(r.next, "Next card"); assert.equal(r.edit, "Edit card"); assert(!r.asking);
    assert(/^[A-Z][a-z]{2} \d{1,2}(, \d{4})? · (in \d+ days|tomorrow)$/.test(r.nextReview), "next review: " + r.nextReview);
    // The date, the relative time and the interval say the same thing (the
    // test server runs on its own clock, so only their agreement is checked).
    assert(/^\d{4}-\d\d-\d\d$/.test(r.iso), "next review date: " + r.iso);
    const shown = new Date(r.iso + "T00:00:00Z").toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
    assert(r.nextReview.startsWith(shown + " · "), "date " + r.iso + " shown as " + r.nextReview);
    assert.equal(r.nextReview.split(" · ")[1], "in " + r.interval, "relative time and interval agree");
    assert(/^\d+ days?$/.test(r.interval), r.interval); assert(/^\d+\.\d days$/.test(r.stability), r.stability);
    assert(/^\d+\.\d \/ 10$/.test(r.difficulty), r.difficulty);
    assert.equal(r.reviews, "1 · 1 correct · 0 almost · 0 incorrect"); assert.equal(r.total, "1");
    assert.equal(r.header, "NATO phonetic alphabet · Letters A–I", "the real place shows once answered");
    assert.equal(r.count, "1 / 15");
    assert(!/streak|points|confetti|🔥/i.test(await s.$eval("main", m => m.innerText)), "facts only");
    await pause(900);
    assert.equal((await review()).kind, "right", "no auto-advance after a correct answer");
    await s.reload({ waitUntil: "networkidle" });
    assert.equal((await review()).kind, "right", "the review survives a reload");
    assert.equal(explains, 0, "no automatic explanation call");
    for (const width of [1280, 390]) for (const theme of ["light", "dark"]) {
      await s.setViewportSize({ width, height: width === 390 ? 844 : 900 });
      await s.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
      await noOverflow(s, "correct review " + width + " " + theme);
      const next = await s.$eval("[data-next]", b => { const x = b.getBoundingClientRect(); return { h: x.height, onScreen: x.bottom <= innerHeight + 1 && x.top >= 0 }; });
      assert(next.h >= 44 && (width !== 390 || next.onScreen), "Next card " + width + JSON.stringify(next));
      await s.screenshot({ path: out + "/review-correct-" + width + "-" + theme + ".png" });
    }
    await s.setViewportSize({ width: 1280, height: 900 });
    // Space moves on when focus is not in a field or on a control.
    await s.evaluate(() => document.activeElement.blur());
    await s.keyboard.press("Space"); await s.waitForSelector("[data-answer-form]");
    assert.equal(await s.$$eval(".answered", es => es.length), 1);
    assert.equal(await s.$(".answered-why"), null, "the scrollback is one line per card");
    assert.equal(await spoilers("second card"), "NATO phonetic alphabet · Set 1");

    // Slice 41 (2): an incorrect answer: the loud moment, then the same review.
    await s.fill("#answer", "This deliberately incorrect answer wraps across several lines on a small phone.");
    await s.click("[data-check]"); await s.waitForSelector(".struck-text");
    r = await review();
    assert.equal(r.kind, "missed"); assert.equal(r.label, "Incorrect"); assert(r.badgeIcon);
    assert.equal(r.why, "The expected answer is different."); assert.equal(r.canonical, "Bravo");
    assert(r.submitted.startsWith("This deliberately incorrect answer")); assert.equal(r.next, "Next card");
    assert.equal(r.reviews, "1 · 0 correct · 0 almost · 1 incorrect"); assert(/· tomorrow$/.test(r.nextReview), r.nextReview);
    assert.equal(r.interval, "1 day");
    for (const width of [1280, 900, 600, 390]) for (const theme of ["light", "dark"]) {
      await s.setViewportSize({ width, height: width === 390 ? 844 : 800 });
      await s.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
      await s.evaluate(() => document.fonts.ready);
      await pause(300);
      const pair = await s.$eval(".judgment-pair", e => { const [a, b] = [...e.children].map(x => x.getBoundingClientRect()); return { left: a.left, otherLeft: b.left, bottom: a.bottom, top: b.top }; });
      assert(Math.abs(pair.left - pair.otherLeft) < 1 && pair.top >= pair.bottom + 15, "the pair must stack at " + width + JSON.stringify(pair));
      assert.equal(await s.$eval(".struck-text", e => getComputedStyle(e).textDecorationLine), "line-through");
      await noOverflow(s, "missed " + width + " " + theme);
      await s.screenshot({ path: out + "/missed-" + width + "-" + theme + ".png", fullPage: true });
    }
    await s.setViewportSize({ width: 1280, height: 800 });
    // Ask Why?: nothing is explained until the learner asks.
    assert.equal(explains, 0, "no automatic explanation call");
    const why = await s.$eval("[data-explain]", b => ({ h: b.getBoundingClientRect().height, w: b.getBoundingClientRect().width, text: b.textContent.trim() }));
    assert(why.h >= 44 && why.w >= 44 && why.text.startsWith("Ask Why?"), JSON.stringify(why));
    assert(await s.$eval("[data-explanation]", e => e.hidden));
    await s.click("[data-explain]");
    await s.waitForFunction(() => { const e = document.querySelector("[data-explanation]"); return e && !e.hidden && e.textContent.length > 0; });
    const explained = await s.$eval("[data-explanation]", e => e.textContent);
    assert(explained.startsWith("What differed:"), explained);
    assert.equal(explains, 1);
    r = await review();
    assert.equal(r.label, "Incorrect", "verdict unchanged"); assert.equal(r.why, "The expected answer is different.");
    assert.equal(r.reviews, "1 · 0 correct · 0 almost · 1 incorrect", "asking changed no history");
    assert(await s.$eval("[data-explain-note]", e => e.hidden), "not a fallback");
    for (const width of [1280, 390]) for (const theme of ["light", "dark"]) {
      await s.setViewportSize({ width, height: width === 390 ? 844 : 800 });
      await s.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
      const m = await s.$eval("[data-explanation]", e => { const x = e.getBoundingClientRect(); return { right: x.right, vw: innerWidth }; });
      assert(m.right <= m.vw + 1, JSON.stringify(m));
      await noOverflow(s, "explanation " + width + " " + theme);
      await s.screenshot({ path: out + "/explain-" + width + "-" + theme + ".png", fullPage: true });
    }
    await s.setViewportSize({ width: 1280, height: 800 });
    // A short pair fits beside itself on desktop, but still stacks on phones.
    await s.$eval(".struck-text", e => { e.textContent = "No"; });
    await s.$eval(".judgment-right", e => { e.textContent = "Yes"; });
    for (const width of [1280, 390]) {
      await s.setViewportSize({ width, height: width === 390 ? 844 : 800 });
      const rects = await s.$eval(".judgment-pair", e => [...e.children].map(x => { const b = x.getBoundingClientRect(); return { top: b.top, bottom: b.bottom }; }));
      if (width === 1280) assert(Math.abs(rects[0].top - rects[1].top) < 1, "short pair should fit side by side");
      else assert(rects[1].top >= rects[0].bottom + 15, "phones always stack");
    }
    await s.setViewportSize({ width: 1280, height: 800 });
    await s.reload({ waitUntil: "networkidle" });
    assert.equal((await review()).kind, "missed", "still on the review");

    // Inline edit: saved in place, kept after a reload, and it moves nothing.
    assert.equal(await s.$eval("[data-edit]", d => d.open), false);
    await s.click("[data-edit-toggle]");
    assert.equal(await s.$eval("#edit-prompt", e => e.value), "NATO alphabet: B");
    assert.equal(await s.$eval("#edit-answer", e => e.value), "Bravo");
    await s.fill("#edit-answer", "");
    await s.$eval("[data-edit-form]", f => { f.noValidate = true; });
    await s.click("[data-edit-save]");
    await s.waitForSelector("[data-edit-error]:not([hidden])");
    assert.equal(await text("[data-edit-error]", s), "The correct answer can't be empty.");
    assert.equal(edits, 1);
    // A fresh review (no refusal on screen) for the edit that is saved.
    await s.reload({ waitUntil: "networkidle" });
    await s.click("[data-edit-toggle]");
    await s.fill("#edit-prompt", "NATO alphabet: the letter B");
    await s.fill("#edit-answer", "Bravo (as in applause)");
    await s.focus("#edit-answer"); await s.keyboard.press("Enter"); await pause(300);
    assert.equal((await review()).kind, "missed", "Enter in the edit field does not move on");
    await s.fill("#edit-answer", "Bravo (as in applause)");
    // The learner keeps their place: the Save button stays where it is in
    // the window, even though the answer above it grew.
    const place = () => s.$eval("[data-edit-save]", b => Math.round(b.getBoundingClientRect().top));
    const y = await place();
    await s.click("[data-edit-save]");
    await s.waitForSelector("[data-edit-status]:not([hidden])");
    assert.equal(await text("[data-edit-status]", s), "Saved");
    r = await review();
    assert.equal(r.canonical, "Bravo (as in applause)"); assert.equal(r.prompt, "NATO alphabet: the letter B");
    assert.equal(r.kind, "missed"); assert.equal(r.label, "Incorrect", "the grade stands");
    assert.equal(r.reviews, "1 · 0 correct · 0 almost · 1 incorrect"); assert.equal(r.interval, "1 day", "the schedule stands");
    assert(Math.abs(await place() - y) <= 2, "saving moved the learner's place: " + y + " -> " + await place());
    assert.equal(edits, 2);
    await s.screenshot({ path: out + "/review-missed-edited.png", fullPage: true });
    await s.reload({ waitUntil: "networkidle" });
    r = await review();
    assert.equal(r.canonical, "Bravo (as in applause)", "the edit is persisted"); assert.equal(r.prompt, "NATO alphabet: the letter B");
    assert.equal(r.kind, "missed", "still the same review after the edit and a reload");
    await noCSP(s, "review");
    // Enter moves on.
    await s.evaluate(() => document.activeElement.blur());
    await s.keyboard.press("Enter");
    await s.waitForSelector("[data-answer-form]");
    assert.equal(await s.$$eval(".answered", es => es.length), 2);
    assert.equal(await text(".margin-count", s), "3 / 15");

    // Navigation keys, and Settings in plain words (slice 41 (8)).
    for (const path of ["/library", "/inbox", "/billing", "/settings"]) {
      if (path === "/settings") { await sgo(path); } else {
        await s.keyboard.press("Escape");
        const key = { "/library": "l", "/inbox": "i", "/billing": "b" }[path];
        await Promise.all([s.waitForNavigation({ waitUntil: "networkidle" }), (async () => { await s.keyboard.press("g"); await s.keyboard.press(key); })()]);
        assert.equal(new URL(s.url()).pathname, path);
      }
      if (path === "/library") {
        await s.keyboard.press("/");
        assert(await s.$eval("[data-search]", e => e === document.activeElement));
        assert.equal(await text(".page-head .purpose", s), "Every card you have, grouped by idea.");
        assert((await s.$eval(".outline", e => e.textContent)).includes("Letters A–I"));
      }
      if (path === "/inbox") assert.equal(await text(".list-purpose", s), "Cards suggested by external scripts or tools wait here until you approve them.");
      if (path === "/billing" || path === "/settings") {
        const tooWide = await s.$$eval("main p", es => es.filter(e => e.getBoundingClientRect().width > parseFloat(getComputedStyle(e).maxWidth) + 1).map(e => e.className));
        assert.deepEqual(tooWide, [], "reading paragraphs exceed measure");
      }
      if (path === "/settings") {
        const set = await s.evaluate(() => ({ titles: [...document.querySelectorAll(".setting h2")].map(h => h.textContent), canAdd: !!document.querySelector("[name=can_add]"), boxes: document.querySelectorAll("#api-tokens input[type=checkbox]").length, body: document.querySelector("main").innerText }));
        assert(set.titles.includes("API Tokens") && set.titles.includes("Study profile") && !set.titles.includes("Agents") && !set.titles.includes("About you"), JSON.stringify(set.titles));
        assert(!set.canAdd && set.boxes === 0, "no can_add checkbox");
        assert(set.body.includes("Programmatic access to your account.") && set.body.includes("Background and goals used to tailor AI-generated cards."));
        assert(!/May add straight|without the inbox/.test(set.body));
      }
      const short = await s.$$eval("a.link", els => els.filter(e => e.getClientRects().length && e.getBoundingClientRect().height < 44).map(e => e.textContent));
      assert.deepEqual(short, [], "short touch targets on " + path);
      await noOverflow(s, path); await noCSP(s, path);
    }
    await s.click("input[name=theme][value=light]");
    await Promise.all([s.waitForNavigation({ waitUntil: "networkidle" }), s.click('form[action="/settings/theme"] button')]);
    assert.equal(await s.evaluate(() => document.documentElement.dataset.theme), "light");
    for (const [key, path] of [["n", "/learn"], ["s", "/study"]]) {
      await s.keyboard.press("Escape");
      await Promise.all([s.waitForNavigation({ waitUntil: "networkidle" }), (async () => { await s.keyboard.press("g"); await s.keyboard.press(key); })()]);
      assert.equal(new URL(s.url()).pathname, path);
      if (path === "/learn") {
        assert.equal(await text(".page-title", s), "Create"); assert.equal(await text(".page-head .purpose", s), "Generate cards with AI");
        assert(await s.$(".page-title svg.icon-create"), "Create has its own icon");
      }
    }

    // An exact text: the word-by-word review, Again (a) and Finish session.
    const exact = await newPage();
    const exactSubmit = selector => Promise.all([exact.waitForNavigation({ waitUntil: "networkidle" }), exact.click(selector)]);
    await exact.goto(base + "/signup", { waitUntil: "networkidle" });
    await exact.fill("#email", "exact-browser@example.com"); await exact.fill("#password", "browser-password");
    await exactSubmit("main form button");
    await onTrialShelf(exact, "third signup"); // studies on the no-card trial
    await exact.goto(base + "/library/texts/new", { waitUntil: "networkidle" });
    await exact.fill("#title", "Key exercise"); await exact.fill("#text", "Remember this line.");
    await exactSubmit("main form button");
    await exact.goto(base + "/study", { waitUntil: "networkidle" });
    await exact.fill("#answer", "wrong"); await exact.keyboard.press("Enter"); await exact.waitForSelector("[data-again]");
    assert.equal(await exact.$eval("[data-review]", e => e.dataset.kind), "exact");
    assert.equal(await text("[data-verdict-label]", exact), "Incorrect");
    assert.equal(await text("[data-canonical]", exact), "Remember this line.");
    assert.equal(await exact.$("[data-explain]"), null, "an exact text has no Ask Why?");
    assert.equal(await text("[data-next]", exact), "Finish session");
    await exact.evaluate(() => document.activeElement.blur()); await exact.keyboard.press("a");
    await exact.waitForSelector("[data-answer-form]");
    assert((await exact.$eval(".exact-label", e => e.textContent)).includes("again"));
    await exact.fill("#answer", "Remember this line."); await exact.keyboard.press("Enter"); await exact.waitForSelector("[data-next]");
    assert.equal(await text("[data-verdict-label]", exact), "Correct");
    assert((await exact.$eval("main", e => e.innerText)).includes("Practice attempt: the schedule did not move."));
    await exact.evaluate(() => document.activeElement.blur()); await exact.keyboard.press("Enter");
    await exact.waitForSelector(".done");
    assert.equal(await text(".done a", exact), "Create new cards");
    await noCSP(exact, "exact");
    assert.deepEqual(errors, []);
    console.log("empty Study, card processing state, in-place starter add, spoiler-free rail, typed/spoken study, post-answer review (correct, incorrect, exact), Ask Why?, inline edit, Enter/Space, library, inbox, billing, settings, create: PASS");
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
