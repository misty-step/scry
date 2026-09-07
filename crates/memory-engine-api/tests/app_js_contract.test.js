import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const script = readFileSync(new URL("../assets/app.js", import.meta.url), "utf8");
const HANDOFF_KEY = "memory-engine.submit-handoff.v1";

function browserHarness(options = {}) {
  const documentEvents = new Map();
  const windowEvents = new Map();
  const formEvents = new Map();
  const storage = options.storage ?? new Map();
  const timers = new Map();
  const fetches = [];
  const rootAttributes = new Set();
  let nextTimerId = 1;
  let performanceTraceInput = null;
  let prevented = 0;
  // Explicit clock advances and optional ticking model distinct observable
  // boundaries without pretending that a fetch creates navigation entries.
  let clock = options.now ?? 100;
  const tick = options.tick ?? 0;
  // requestAnimationFrame is queued, never fired inline: one call to
  // runAnimationFrame() below drains exactly the callbacks queued *before*
  // that call, matching the browser's one-callback-per-paint semantics. A
  // nested requestAnimationFrame scheduled while draining queues for the
  // *next* runAnimationFrame(), never the current one.
  let frameQueue = [];
  let verdictPresent = options.verdict ?? false;
  let verdictElement = { focus: () => focused.push("verdict"), setAttribute() {} };
  const focused = [];
  const statusAttributes = new Map();
  const reviewStatus = {
    textContent: "",
    setAttribute: (name, value) => statusAttributes.set(name, value),
  };
  const nativeStatus = { textContent: "" };
  const generationStatus = { textContent: "" };
  let jobsList = options.jobsList ?? null;
  let waitingJob = options.waitingJob ?? null;
  let randomSequence = 0;
  const sseHandlers = new Map();
  const navigations = [];
  const eventSource = options.eventSource;
  if (eventSource) {
    eventSource.addEventListener = (name, handler) => {
      const handlers = sseHandlers.get(name) ?? [];
      handlers.push(handler);
      sseHandlers.set(name, handlers);
    };
  }

  const addListener = (listeners, name, handler) => {
    const handlers = listeners.get(name) ?? [];
    handlers.push(handler);
    listeners.set(name, handlers);
  };
  const classes = (...names) => ({ contains: (name) => names.includes(name) });
  const controlAttributes = new Map();
  const control = {
    classList: classes(...(options.controlClasses ?? ["me-choice"])),
    textContent: options.controlLabel ?? "Answer",
    name: options.controlName ?? "answer",
    value: options.controlValue ?? options.controlLabel ?? "Answer",
    setAttribute: (name, value) => controlAttributes.set(name, value ?? ""),
    getAttribute: (name) => {
      if (controlAttributes.has(name)) return controlAttributes.get(name);
      if (name === "name") return control.name;
      if (name === "value") return control.value;
      return null;
    },
    removeAttribute: (name) => controlAttributes.delete(name),
  };
  const responseInput = { value: "" };
  const formFields = options.formFields ?? {
    csrfToken: "csrf-test",
    reviewUnitId: "unit-1",
    responseTimeMs: "",
    idempotencyKey: "review-unit-1-0",
  };
  let nativeSubmitCount = 0;
  let viewHtml = options.viewHtml ?? '<p class="me-prompt">Q</p>';
  let dueText = options.dueText ?? "1 due";
  let footerHtml = options.footerHtml ?? '<nav class="me-nav">Home</nav>';
  const form = {
    tagName: "FORM",
    classList: classes(options.formClass ?? "me-choices-form"),
    action: options.action ?? "/app/submit",
    getAttribute: (name) => (name === "action" ? options.action ?? "/app/submit" : null),
    addEventListener: (name, handler) => addListener(formEvents, name, handler),
    querySelector(selector) {
      if (selector === 'input[name="responseTimeMs"]') return responseInput;
      if (selector === 'input[name="performanceTraceId"]') return performanceTraceInput;
      if (selector === 'button[type="submit"], button:not([type])') return control;
      if (selector === 'button[type="submit"]') return control;
      if (selector === ".me-entry-status" || selector === ".me-live-hint") return nativeStatus;
      return null;
    },
    querySelectorAll: (selector) => (selector === ".me-choice" ? [control] : []),
    appendChild: (input) => {
      performanceTraceInput = input;
    },
    submit() {
      nativeSubmitCount += 1;
    },
  };
  const headMetas = new Map(Object.entries(options.metas ?? {}));
  const document = {
    documentElement: {
      setAttribute: (name) => rootAttributes.add(name),
      removeAttribute: (name) => rootAttributes.delete(name),
      hasAttribute: (name) => rootAttributes.has(name),
    },
    head: {
      appendChild(meta) {
        if (meta && meta.name) headMetas.set(meta.name, meta.content ?? "");
      },
    },
    visibilityState: options.visibilityState ?? "visible",
    addEventListener: (name, handler) => addListener(documentEvents, name, handler),
    querySelector(selector) {
      if (selector === `form.${options.formClass}`) return form;
      if (selector === ".me-verdict" && verdictPresent) return verdictElement;
      if (selector === "[data-review-status]") return reviewStatus;
      if (selector === "[data-generation-job-id][data-terminal-url]") {
        return waitingJob ? {
          getAttribute(name) {
            if (name === "data-generation-job-id") return waitingJob.id;
            if (name === "data-terminal-url") return waitingJob.destination;
            return null;
          },
          querySelector: (name) => name === "[data-generation-status]" ? generationStatus : null,
        } : null;
      }
      if (selector === ".ae-view") {
        return {
          get innerHTML() {
            return viewHtml;
          },
          set innerHTML(value) {
            clock += options.swapDelayMs ?? 0;
            viewHtml = String(value);
            verdictPresent = viewHtml.includes("me-verdict");
            verdictElement = { focus: () => focused.push("verdict"), setAttribute() {} };
            reviewStatus.textContent = "";
            statusAttributes.clear();
          },
          querySelector(inner) {
            if (inner === ".me-verdict" && verdictPresent) return verdictElement;
            if (
              inner ===
                'form.me-next button[type="submit"], form.me-next button:not([type])' &&
              viewHtml.includes("me-next")
            ) {
              return { focus() {} };
            }
            return null;
          },
        };
      }
      if (selector === ".me-due") {
        return {
          get textContent() {
            return dueText;
          },
          set textContent(value) {
            dueText = String(value);
          },
        };
      }
      if (selector === "footer.ae-bar") {
        return {
          get innerHTML() {
            return footerHtml;
          },
          set innerHTML(value) {
            footerHtml = String(value);
          },
        };
      }
      const metaMatch = selector.match(/^meta\[name="([^"]+)"\]$/);
      if (metaMatch) {
        const value = headMetas.get(metaMatch[1]);
        if (value === undefined) return null;
        return {
          getAttribute: (name) => (name === "content" ? value : null),
          setAttribute: (name, next) => {
            if (name === "content") headMetas.set(metaMatch[1], next);
          },
          parentNode: {
            removeChild() {
              headMetas.delete(metaMatch[1]);
            },
          },
        };
      }
      return null;
    },
    getElementById: (id) => (id === "me-jobs" ? jobsList : null),
    querySelectorAll(selector) {
      if (selector === ".me-more-sheet button, .me-hatch-row button") {
        return options.hatchButtons ?? [];
      }
      const match = selector.match(/^meta\[name="([^"]+)"\]$/);
      if (!match) return [];
      const value = headMetas.get(match[1]);
      return value === undefined
        ? []
        : [{ getAttribute: () => value }];
    },

    createElement: (tag) => {
      if (tag === "meta") {
        return {
          name: "",
          content: "",
          setAttribute(name, value) {
            if (name === "name") this.name = value;
            if (name === "content") this.content = value;
          },
        };
      }
      const el = {
        name: "",
        value: "",
        attrs: {},
        setAttribute(name, value) {
          this.attrs[name] = value;
          if (name === "name") this.name = value;
          if (name === "value") this.value = value;
        },
        getAttribute(name) {
          return this.attrs[name] ?? null;
        },
      };
      return el;
    },
  };
  const sessionStorage = {
    getItem: (key) => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: (key) => storage.delete(key),
  };
  const fetchImpl = options.fetchImpl;
  const enableInPlace = options.inPlace === true;
  class FakeFormData {
    constructor(source) {
      this.map = new Map();
      if (source === form) {
        for (const [key, value] of Object.entries(formFields)) {
          this.map.set(key, value);
        }
        if (responseInput.value !== "") this.map.set("responseTimeMs", responseInput.value);
        if (performanceTraceInput) this.map.set("performanceTraceId", performanceTraceInput.value);
      }
    }
    append(name, value) {
      this.map.set(name, value);
    }
    has(name) {
      return this.map.has(name);
    }
    get(name) {
      return this.map.has(name) ? this.map.get(name) : null;
    }
    entries() {
      return this.map.entries();
    }
    forEach(callback) {
      this.map.forEach((value, name) => callback(value, name, this));
    }
  }
  class FakeDOMParser {
    parseFromString(html) {
      clock += options.parseDelayMs ?? 0;
      const viewMatch = html.match(/<div class="ae-view">([\s\S]*?)<\/div>/);
      const dueMatch = html.match(/<span class="me-due">([^<]*)<\/span>/);
      const footerMatch = html.match(/<footer class="ae-bar">([\s\S]*?)<\/footer>/);
      const meta = {};
      for (const match of html.matchAll(
        /<meta name="([^"]+)" content="([^"]*)">/g,
      )) {
        (meta[match[1]] ??= []).push(match[2]);
      }
      const viewHtmlNext = viewMatch ? viewMatch[1] : "";
      return {
        querySelector(selector) {
          if (selector === ".ae-view") {
            return viewMatch ? { innerHTML: viewHtmlNext } : null;
          }
          if (selector === ".me-due" && dueMatch) {
            return { textContent: dueMatch[1] };
          }
          if (selector === "footer.ae-bar" && footerMatch) {
            return { innerHTML: footerMatch[1] };
          }
          const metaMatch = selector.match(/^meta\[name="([^"]+)"\]$/);
          if (metaMatch && meta[metaMatch[1]] !== undefined) {
            return {
              getAttribute: (name) =>
                name === "content" ? meta[metaMatch[1]][0] : null,
            };
          }
          return null;
        },
        querySelectorAll(selector) {
          const match = selector.match(/^meta\[name="([^"]+)"\]$/);
          return (match ? meta[match[1]] ?? [] : []).map((content) => ({
            getAttribute: (name) => name === "content" ? content : null,
          }));
        },
      };
    }
  }
  const window = {
    sessionStorage,
    performance: {
      timeOrigin: 1_000_000,
      now: () => {
        const value = clock;
        clock += tick;
        return value;
      },
      getEntriesByType: (type) =>
        type === "navigation" && options.navigation ? [options.navigation] : [],
    },
    crypto: {
      getRandomValues(values) {
        for (let index = 0; index < values.length; index += 1) values[index] = (index + 1 + randomSequence) % 256;
        randomSequence += 16;
        return values;
      },
    },
    innerWidth: 390,
    Uint8Array,
    AbortController,
    addEventListener: (name, handler) => addListener(windowEvents, name, handler),
    requestAnimationFrame: (handler) => {
      frameQueue.push(handler);
      return frameQueue.length;
    },
    fetch(url, request) {
      fetches.push({ url, request });
      if (typeof fetchImpl === "function") {
        // Always re-wrap so the VM sees a real thenable even when the
        // implementation returns a bare value or a foreign-realm Promise.
        return Promise.resolve().then(() => fetchImpl(url, request));
      }
      return Promise.resolve({ catch() {} });
    },
    setTimeout(handler, duration) {
      const id = nextTimerId;
      nextTimerId += 1;
      timers.set(id, { handler, duration });
      return id;
    },
    clearTimeout: (id) => timers.delete(id),
    location: {
      assign: (url) => navigations.push(url),
      reload() {
        navigations.push("reload");
      },
    },
  };
  if (enableInPlace) {
    window.FormData = FakeFormData;
    window.URLSearchParams = URLSearchParams;
    window.DOMParser = FakeDOMParser;
  }
  window.window = window;
  if (eventSource) window.EventSource = function EventSource() { return eventSource; };
  if (options.timeOriginUnavailable) delete window.performance.timeOrigin;
  if (options.storageUnavailable) {
    Object.defineProperty(window, "sessionStorage", {
      get() { throw new Error("storage unavailable"); },
    });
  }

  vm.runInNewContext(script, {
    console,
    document,
    window,
    EventSource: eventSource ? window.EventSource : undefined,
    URL,
    Promise,
    setTimeout,
    clearTimeout,
  });

  return {
    dispatchSubmit() {
      const event = {
        target: form,
        submitter: control,
        preventDefault() {
          prevented += 1;
        },
      };
      for (const handler of formEvents.get("submit") ?? []) handler(event);
      for (const handler of documentEvents.get("submit") ?? []) handler(event);
    },
    controlLabel: () => control.textContent,
    controlAttr: (name) => controlAttributes.get(name),
    controlDisabled: () => control.disabled === true,
    viewHtml: () => viewHtml,
    dueText: () => dueText,
    footerHtml: () => footerHtml,
    focused,
    statusState: () => statusAttributes.get("data-state"),
    statusText: () => reviewStatus.textContent,
    setJobsList: (list) => { jobsList = list; },
    generationStatus,
    setWaitingJob: (job) => { waitingJob = job; },
    nativeSubmits: () => nativeSubmitCount,
    responseTimeMs: () => responseInput.value,
    meta: (name) => headMetas.get(name) ?? null,
    dispatchWindow(name, event = {}) {
      for (const handler of windowEvents.get(name) ?? []) handler(event);
    },
    runTimer(duration) {
      const entry = [...timers.entries()].find(([, timer]) => timer.duration === duration);
      if (!entry) throw new Error(`missing ${duration}ms timer`);
      timers.delete(entry[0]);
      entry[1].handler();
    },
    // Drains exactly the callbacks queued before this call — one real frame.
    runAnimationFrame() {
      const queue = frameQueue;
      frameQueue = [];
      for (const handler of queue) handler();
    },
    pendingFrames() {
      return frameQueue.length;
    },
    advanceClock(ms) {
      clock += ms;
    },
    setVerdictPresent(present) {
      verdictPresent = present;
    },
    setVisibility(visibility) {
      document.visibilityState = visibility;
      for (const handler of documentEvents.get("visibilitychange") ?? []) handler();
    },
    setMeta(name, value) {
      headMetas.set(name, value);
    },
    storage,
    fetches,
    handoff: () => sessionStorage.getItem(HANDOFF_KEY),
    busy: () => document.documentElement.hasAttribute("data-busy"),
    prevented: () => prevented,
    emitJob(job) {
      for (const handler of sseHandlers.get("job") ?? []) {
        handler({ data: JSON.stringify(job) });
      }
    },
    navigations,
  };
}

// Shared fixture: a source document that just submitted a review and left
// the handoff behind in (shared) sessionStorage for a landing document to
// consume, plus the strict request id the server would have rendered.
function submittedHandoff(options = {}) {
  const source = browserHarness(options);
  source.dispatchSubmit();
  const handoff = JSON.parse(source.handoff());
  source.dispatchWindow("pagehide");
  return { source, handoff, requestId: "req_0123456789abcdef0123456789abcdef" };
}

function matchingNavigation(requestId, traceToken) {
  return {
    type: "navigate",
    startTime: 0,
    responseStart: 110,
    responseEnd: 120,
    serverTiming: [
      { name: "request", description: requestId },
      { name: "handoff", description: traceToken },
    ],
  };
}

function matchingLandingOptions(source, handoff, requestId, extra = {}) {
  return {
    storage: source.storage,
    now: 130,
    verdict: true,
    metas: {
      "memory-engine-submit-request": requestId,
      "memory-engine-submit-handoff": handoff.token,
      "memory-engine-csrf-token": "csrf-test",
    },
    navigation: matchingNavigation(requestId, handoff.token),
    ...extra,
  };
}

test("pagehide preserves a native submit handoff for the landing document", () => {
  const browser = browserHarness();
  browser.dispatchSubmit();
  const handoff = browser.handoff();
  expect(handoff).not.toBeNull();

  browser.dispatchWindow("pagehide");

  expect(browser.handoff()).toBe(handoff);
  expect(browser.busy()).toBeFalse();
});

test("busy recovery preserves a slow submit handoff until its TTL", () => {
  const browser = browserHarness();
  browser.dispatchSubmit();
  const handoff = browser.handoff();
  expect(handoff).not.toBeNull();
  expect(browser.busy()).toBeTrue();

  browser.runTimer(30_000);

  expect(browser.handoff()).toBe(handoff);
  expect(browser.busy()).toBeFalse();

  browser.runTimer(65_000);
  expect(browser.handoff()).toBeNull();
});

test("pending review actions announce state without changing the submitted answer", () => {
  const browser = browserHarness({
    action: "/app/submit",
    controlClasses: ["me-choice"],
    controlLabel: "42",
    controlValue: "42",
  });
  browser.dispatchSubmit();
  expect(browser.busy()).toBeTrue();
  expect(browser.statusState()).toBe("pending");
  expect(browser.controlAttr("aria-disabled")).toBe("true");
  expect(browser.controlLabel()).toBe("42");
  browser.runTimer(30_000);
  expect(browser.controlAttr("aria-disabled")).toBeUndefined();
});

async function flushMicrotasks() {
  for (let i = 0; i < 8; i += 1) await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function gradedResponse(request, options = {}) {
  const requestId = options.requestId ?? "req_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
  const traceId = options.traceId ?? request.body.get("performanceTraceId");
  const serverTiming = options.serverTiming ??
    `request;desc="${requestId}", handoff;desc="${traceId}", total;dur=12`;
  return {
    ok: true,
    status: 200,
    headers: {
      get(name) {
        if (name === "content-type") return "text/html; charset=utf-8";
        if (name === "server-timing") return serverTiming;
        return null;
      },
    },
    text: () => Promise.resolve(`<!doctype html><html><head>
<meta name="memory-engine-csrf-token" content="csrf-next">
<meta name="memory-engine-submit-request" content="${requestId}">
<meta name="memory-engine-submit-handoff" content="${traceId}">
</head><body>
<span class="me-due">0 due</span>
<div class="ae-view">
<p class="me-prompt">What is 6*7?</p>
<p class="me-result"><span class="me-verdict">Correct</span></p>
<form class="me-next" action="/app/next" method="post"><button type="submit">Continue</button></form>
</div>
<footer class="ae-bar"><p class="me-tagline">review</p></footer>
</body></html>`),
  };
}

test("enhanced answer acknowledges once and reports only correlated observed phases after paint", async () => {
  let resolveFetch;
  let resolveBody;
  const fetchPromise = new Promise((resolve) => { resolveFetch = resolve; });
  const bodyPromise = new Promise((resolve) => { resolveBody = resolve; });
  const browser = browserHarness({
    inPlace: true,
    tick: 1,
    parseDelayMs: 50,
    swapDelayMs: 7,
    controlClasses: ["me-choice"],
    controlLabel: "42",
    controlValue: "42",
    viewHtml: '<p class="me-prompt">What is 6*7?</p><form class="me-choices-form"></form>',
    fetchImpl: () => fetchPromise,
  });

  browser.dispatchSubmit();
  browser.dispatchSubmit();
  expect(browser.busy()).toBeTrue();
  expect(browser.statusState()).toBe("pending");
  expect(browser.handoff()).toBeNull();
  expect(browser.controlLabel()).toBe("42");
  expect(browser.fetches).toHaveLength(1);
  expect(browser.viewHtml()).not.toContain("me-verdict");
  const request = browser.fetches[0].request;
  expect(request.body.get("answer")).toBe("42");
  expect(Number(request.body.get("responseTimeMs"))).toBeGreaterThan(0);
  const traceId = request.body.get("performanceTraceId");
  expect(traceId).toMatch(/^trace_[0-9a-f]{32}$/);

  const response = gradedResponse(request);
  browser.advanceClock(20);
  resolveFetch({ ...response, text: () => bodyPromise });
  await flushMicrotasks();
  expect(browser.busy()).toBeTrue();
  expect(browser.viewHtml()).not.toContain("me-verdict");
  expect(browser.pendingFrames()).toBe(0);
  browser.advanceClock(13);
  resolveBody(await response.text());
  await flushMicrotasks();
  expect(browser.busy()).toBeFalse();
  expect(browser.viewHtml()).toContain('class="me-verdict">Correct<');
  expect(browser.viewHtml()).toContain("me-next");
  expect(browser.focused).toEqual(["verdict"]);
  expect(browser.dueText()).toBe("0 due");
  expect(browser.meta("memory-engine-csrf-token")).toBe("csrf-next");
  expect(browser.fetches).toHaveLength(1);
  browser.advanceClock(16);
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(1);
  browser.advanceClock(16);
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(2);
  expect(browser.fetches[1].url).toBe("/app/performance/submit");
  const payload = JSON.parse(browser.fetches[1].request.body);
  expect(payload).toMatchObject({
    schema: "memory_engine.browser_submit.v2",
    requestId: "req_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    traceId,
    navigation: "in_place",
  });
  expect(payload.tapToAckMs).toBeGreaterThan(0);
  expect(payload.requestToResponseMs).toBeGreaterThanOrEqual(20);
  expect(payload.domSwapMs).toBe(8);
  expect(payload.transferMs).toBeGreaterThanOrEqual(13);
  expect(payload.gradedVisibleMs).toBeGreaterThan(
    payload.requestToResponseMs + payload.transferMs + payload.domSwapMs,
  );
  expect(Object.keys(payload).sort()).toEqual([
    "schema", "csrfToken", "requestId", "traceId", "navigation",
    "tapToAckMs", "requestToResponseMs", "transferMs", "domSwapMs",
    "gradedVisibleMs", "viewport",
  ].sort());
  browser.dispatchWindow("pageshow");
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(2);
  expect(browser.navigations).toEqual([]);
  expect(browser.nativeSubmits()).toBe(0);
});

test("a timed-out response cannot replace or complete a retry on the same form", async () => {
  const pending = [];
  const browser = browserHarness({
    inPlace: true,
    fetchImpl(url, request) {
      if (url !== "/app/submit") return {};
      return new Promise((resolve) => pending.push({ request, resolve }));
    },
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runTimer(30_000);
  expect(browser.fetches[0].request.signal.aborted).toBeTrue();
  browser.dispatchSubmit();
  await flushMicrotasks();
  pending[0].resolve(gradedResponse(pending[0].request));
  await flushMicrotasks();
  expect(browser.viewHtml()).not.toContain("me-verdict");
  expect(browser.busy()).toBeTrue();
  expect(browser.pendingFrames()).toBe(0);
  pending[1].resolve(gradedResponse(pending[1].request));
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.runAnimationFrame();
  const receipts = browser.fetches.filter(({ url }) => url === "/app/performance/submit");
  expect(receipts).toHaveLength(1);
  expect(JSON.parse(receipts[0].request.body).traceId).toBe(
    pending[1].request.body.get("performanceTraceId"),
  );
});

test("hiding between paints cancels completion even when visible again before the callback", async () => {
  const browser = browserHarness({
    inPlace: true,
    fetchImpl: (_url, request) => gradedResponse(request),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.setVisibility("hidden");
  browser.setVisibility("visible");
  browser.runAnimationFrame();
  expect(browser.viewHtml()).toContain("me-verdict");
  expect(browser.fetches).toHaveLength(1);
  browser.dispatchWindow("pageshow");
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(1);
});

test("BFCache restore never applies a response that belonged to the abandoned document state", async () => {
  let resolveFetch;
  const browser = browserHarness({
    inPlace: true,
    viewHtml: '<input class="me-answer-input" value="draft answer">',
    fetchImpl: () => new Promise((resolve) => { resolveFetch = resolve; }),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.dispatchWindow("pagehide");
  browser.dispatchWindow("pageshow", { persisted: true });
  resolveFetch(gradedResponse(browser.fetches[0].request));
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.runAnimationFrame();
  expect(browser.viewHtml()).toContain('value="draft answer"');
  expect(browser.busy()).toBeFalse();
  expect(browser.fetches).toHaveLength(1);
  expect(browser.handoff()).toBeNull();
});

test("a response from another handoff still grades but never produces a mismatched receipt", async () => {
  const browser = browserHarness({
    inPlace: true,
    fetchImpl: (_url, request) => gradedResponse(request, {
      serverTiming: 'request;desc="req_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", handoff;desc="trace_ffffffffffffffffffffffffffffffff"',
    }),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.runAnimationFrame();
  expect(browser.viewHtml()).toContain("me-verdict");
  expect(browser.busy()).toBeFalse();
  expect(browser.fetches).toHaveLength(1);
});

test("duplicate correlation headers fail closed without blocking the graded view", async () => {
  const browser = browserHarness({
    inPlace: true,
    fetchImpl(_url, request) {
      const response = gradedResponse(request);
      return gradedResponse(request, {
        serverTiming: `${response.headers.get("server-timing")}, request;desc="req_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
      });
    },
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("me-verdict");
  expect(browser.pendingFrames()).toBe(0);
  expect(browser.fetches).toHaveLength(1);
});

test("duplicate rendered handoff metadata cannot be collapsed into a trusted receipt", async () => {
  const browser = browserHarness({
    inPlace: true,
    fetchImpl(_url, request) {
      const response = gradedResponse(request);
      return {
        ...response,
        text: async () => (await response.text()).replace(
          "</head>",
          '<meta name="memory-engine-submit-handoff" content="trace_ffffffffffffffffffffffffffffffff"></head>',
        ),
      };
    },
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("me-verdict");
  expect(browser.pendingFrames()).toBe(0);
  expect(browser.fetches).toHaveLength(1);
});

test("replacing correlation metadata before paint suppresses the queued receipt", async () => {
  const browser = browserHarness({
    inPlace: true,
    fetchImpl: (_url, request) => gradedResponse(request),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.setMeta("memory-engine-submit-request", "req_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(1);
});

test("in-place completion does not require session storage", async () => {
  const browser = browserHarness({
    inPlace: true,
    storageUnavailable: true,
    fetchImpl(url, request) {
      return url === "/app/submit" ? gradedResponse(request) : {};
    },
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.runAnimationFrame();
  expect(browser.fetches).toHaveLength(2);
  expect(JSON.parse(browser.fetches[1].request.body).navigation).toBe("in_place");
});

test("missing monotonic origin disables measurement rather than fabricating phases or blocking grading", async () => {
  const browser = browserHarness({
    inPlace: true,
    timeOriginUnavailable: true,
    fetchImpl: (_url, request) => gradedResponse(request),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  browser.runAnimationFrame();
  browser.runAnimationFrame();
  expect(browser.viewHtml()).toContain("me-verdict");
  expect(browser.fetches).toHaveLength(1);
  expect(browser.busy()).toBeFalse();
});

test("an unsuccessful answer response cannot erase typed input or silently repost", async () => {
  const browser = browserHarness({
    inPlace: true,
    viewHtml: '<input class="me-answer-input" value="my answer">',
    fetchImpl: (_url, request) => ({ ...gradedResponse(request), ok: false, status: 500 }),
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain('value="my answer"');
  expect(browser.statusState()).toBe("failed");
  expect(browser.fetches).toHaveLength(1);
  expect(browser.pendingFrames()).toBe(0);
  expect(browser.nativeSubmits()).toBe(0);
});

test("an uncertain fetch failure preserves the answer for an intentional idempotent retry", async () => {
  const browser = browserHarness({
    inPlace: true,
    controlClasses: ["me-choice"],
    controlLabel: "42",
    controlValue: "42",
    viewHtml: '<input class="me-answer-input" value="typed">',
    fetchImpl() {
      return Promise.reject(new Error("network down"));
    },
  });
  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
  expect(browser.viewHtml()).toContain('value="typed"');
  expect(browser.statusState()).toBe("failed");
  expect(browser.busy()).toBeFalse();
  browser.dispatchSubmit();
  expect(browser.fetches).toHaveLength(2);
  expect(browser.fetches[1].request.body.get("answer")).toBe("42");
  expect(browser.fetches[1].request.body.get("idempotencyKey")).toBe(
    browser.fetches[0].request.body.get("idempotencyKey"),
  );
  expect(browser.fetches[1].request.body.get("performanceTraceId")).not.toBe(
    browser.fetches[0].request.body.get("performanceTraceId"),
  );
  await flushMicrotasks();
});

test("continue uses in-place fetch and does not write a submit handoff", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/next",
    formClass: "me-next",
    controlClasses: ["ae-button"],
    controlLabel: "Continue →",
    fetchImpl() {
      return Promise.resolve({
        ok: true,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            `<div class="ae-screen"><span class="me-due">1 due</span><div class="ae-view"><p class="me-prompt">Next card</p></div><footer class="ae-bar"><p class="me-tagline">review</p></footer></div>`,
          ),
      });
    },
  });

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.handoff()).toBeNull();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Next card");
  expect(browser.footerHtml()).toContain("review");
  expect(browser.busy()).toBeFalse();
});

function htmlReviewLanding(prompt, due = "1 due", notice = "") {
  const banner = notice
    ? `<p class="me-notice" role="status">${notice}</p>`
    : "";
  return `<!doctype html><html><body>
<span class="me-due">${due}</span>
<div class="ae-view">${banner}<p class="me-prompt">${prompt}</p></div>
<footer class="ae-bar"><p class="me-tagline">review</p></footer>
</body></html>`;
}

test("skip fetches in place and keeps the server confirm", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/skip",
    formClass: "me-hatch",
    controlClasses: ["ae-button"],
    controlLabel: "Skip",
    fetchImpl() {
      return Promise.resolve({
        ok: true,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            htmlReviewLanding(
              "Letter O",
              "1 due",
              "You'll see this later this session.",
            ),
          ),
      });
    },
  });

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.handoff()).toBeNull();
  expect(browser.fetches[0].url).toBe("/app/skip");
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Letter O");
  expect(browser.viewHtml()).toContain("later this session");
  expect(browser.viewHtml()).toContain('role="status"');
  expect(browser.busy()).toBeFalse();
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
});

test("snooze fetches in place and keeps the server tomorrow notice", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/snooze",
    formClass: "me-hatch",
    controlClasses: ["ae-button"],
    controlLabel: "Snooze",
    fetchImpl() {
      return Promise.resolve({
        ok: true,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            htmlReviewLanding("Letter P", "1 due", "You'll see this tomorrow."),
          ),
      });
    },
  });

  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("tomorrow");
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
});

test("keep draft fetches in place and updates the due count", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/draft/keep",
    formClass: "me-keep",
    controlClasses: ["ae-button-quiet"],
    controlLabel: "Keep as written",
    dueText: "0 due",
    viewHtml: '<article class="me-pending-draft"><p>Draft</p></article>',
    fetchImpl() {
      return Promise.resolve({
        ok: true,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            htmlReviewLanding("Kept card", "1 due").replace(
              '<p class="me-prompt">Kept card</p>',
              "<p>Queue ready</p>",
            ),
          ),
      });
    },
  });

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.handoff()).toBeNull();
  expect(browser.fetches[0].request.headers["Content-Type"]).toBe(
    "application/x-www-form-urlencoded;charset=UTF-8",
  );
  expect(browser.fetches[0].request.body.get("csrfToken")).toBe("csrf-test");
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Queue ready");
  expect(browser.viewHtml()).not.toContain("me-pending-draft");
  expect(browser.dueText()).toBe("1 due");
  expect(browser.nativeSubmits()).toBe(0);
});

test("card quality saves in place with the clicked verdict", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/content-feedback",
    formClass: "me-content-feedback",
    controlClasses: ["ae-button"],
    controlLabel: "Keep this card",
    controlName: "verdict",
    controlValue: "kept",
    formFields: {
      csrfToken: "csrf-test",
      reviewUnitId: "unit-1",
      idempotencyKey: "feedback-unit-1-1-new",
      rationale: "The accepted answer is clear.",
    },
    fetchImpl() {
      return Promise.resolve({
        ok: true,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            htmlReviewLanding(
              "Same graded card",
              "0 due",
              "Saved. This card will help improve future generation.",
            ),
          ),
      });
    },
  });

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.fetches[0].url).toBe("/app/content-feedback");
  expect(browser.fetches[0].request.body.get("verdict")).toBe("kept");
  expect(browser.fetches[0].request.body.get("rationale")).toBe(
    "The accepted answer is clear.",
  );
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Same graded card");
  expect(browser.viewHtml()).toContain("Saved. This card will help improve");
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
});

test("keep error HTML swaps without a second POST", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/draft/keep",
    formClass: "me-keep",
    controlClasses: ["ae-button-quiet"],
    controlLabel: "Keep as written",
    viewHtml: '<article class="me-pending-draft"><p>Draft</p></article>',
    fetchImpl() {
      return Promise.resolve({
        ok: false,
        status: 409,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(htmlReviewLanding("", "0 due", "Draft already decided.")),
      });
    },
  });

  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Draft already decided.");
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
  expect(browser.busy()).toBeFalse();
});

test("skip fetch failure keeps unsent input and permits an intentional retry", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/skip",
    formClass: "me-hatch",
    controlClasses: ["ae-button"],
    controlLabel: "Skip",
    viewHtml: '<input class="me-answer-input" value="typed"><p class="me-prompt">Letter N</p>',
    fetchImpl() {
      return Promise.reject(new Error("network down"));
    },
  });

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  await flushMicrotasks();
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
  expect(browser.viewHtml()).toContain('value="typed"');
  expect(browser.statusState()).toBe("failed");
  expect(browser.busy()).toBeFalse();

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(2);
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.fetches).toHaveLength(2);
  await flushMicrotasks();
});

test("expired-session responses keep the current question instead of replacing unsent input", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/skip",
    formClass: "me-hatch",
    controlClasses: ["ae-button"],
    controlLabel: "Skip",
    viewHtml: '<p class="me-prompt">Letter N</p>',
    fetchImpl() {
      return Promise.resolve({
        ok: false,
        status: 401,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(
            `<!doctype html><html><body><div class="ae-view"><div class="me-cover">Return to your workspace</div></div></body></html>`,
          ),
      });
    },
  });

  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Letter N");
  expect(browser.viewHtml()).not.toContain("Return to your workspace");
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
  expect(browser.statusState()).toBe("failed");
  expect(browser.busy()).toBeFalse();
});

test("skip 404 HTML swaps without reload or a second POST", async () => {
  const browser = browserHarness({
    inPlace: true,
    action: "/app/skip",
    formClass: "me-hatch",
    controlClasses: ["ae-button"],
    controlLabel: "Skip",
    viewHtml: '<p class="me-prompt">Letter N</p>',
    fetchImpl() {
      return Promise.resolve({
        ok: false,
        status: 404,
        headers: { get: () => "text/html; charset=utf-8" },
        text: () =>
          Promise.resolve(htmlReviewLanding("", "1 due", "Review unit not found.")),
      });
    },
  });

  browser.dispatchSubmit();
  await flushMicrotasks();
  expect(browser.viewHtml()).toContain("Review unit not found.");
  expect(browser.nativeSubmits()).toBe(0);
  expect(browser.navigations).toEqual([]);
  expect(browser.busy()).toBeFalse();
});




test("every native form keeps instant acknowledgment and duplicate suppression", () => {
  const browser = browserHarness({ action: "/app/next" });
  browser.dispatchSubmit();
  expect(browser.busy()).toBeTrue();
  expect(browser.handoff()).toBeNull();

  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);

  browser.runTimer(30_000);
  expect(browser.busy()).toBeFalse();
  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
});

test("a second document consumes the handoff and emits one visible receipt only after two real animation frames", () => {
  const { source, handoff, requestId } = submittedHandoff({ tick: 1 });
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId, { tick: 1 }));

  landing.dispatchWindow("pageshow");
  // The emission is genuinely deferred: nothing has fired yet.
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(1);

  landing.runAnimationFrame();
  // First frame only queues the second — still nothing emitted.
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(1);

  landing.runAnimationFrame();
  expect(landing.handoff()).toBeNull();
  expect(landing.fetches).toHaveLength(1);
  expect(landing.fetches[0].url).toBe("/app/performance/submit");
  const payload = JSON.parse(landing.fetches[0].request.body);
  expect(payload).toMatchObject({
    schema: "memory_engine.browser_submit.v2",
    navigation: "full_page",
    requestId,
    traceId: handoff.token,
    viewport: "mobile",
  });
  // With a ticking clock the ack and visible durations are genuinely
  // simulated elapsed time, not the frozen zero a static clock would give.
  expect(payload.tapToAckMs).toBeGreaterThan(0);
  expect(payload.gradedVisibleMs).toBeGreaterThan(payload.tapToAckMs);
  expect(payload.requestToResponseMs + payload.transferMs + payload.navigationMs).toBe(
    payload.gradedVisibleMs
  );
});

test("unavailable native navigation phases remain absent in an otherwise correlated completion", () => {
  const { source, handoff, requestId } = submittedHandoff({ tick: 1 });
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId, {
    navigation: {
      ...matchingNavigation(requestId, handoff.token),
      responseStart: 0,
      responseEnd: 0,
    },
  }));
  landing.dispatchWindow("pageshow");
  landing.runAnimationFrame();
  landing.runAnimationFrame();
  expect(landing.fetches).toHaveLength(1);
  const payload = JSON.parse(landing.fetches[0].request.body);
  expect(payload.navigation).toBe("full_page");
  expect(payload).not.toHaveProperty("requestToResponseMs");
  expect(payload).not.toHaveProperty("transferMs");
  expect(payload).not.toHaveProperty("navigationMs");
  expect(payload).not.toHaveProperty("domSwapMs");
  expect(payload.gradedVisibleMs).toBeGreaterThan(payload.tapToAckMs);
});


test("a rejected landing consumes the handoff without emitting telemetry", () => {
  const source = browserHarness();
  source.dispatchSubmit();
  source.dispatchWindow("pagehide");

  const landing = browserHarness({ storage: source.storage, now: 130 });
  landing.dispatchWindow("pageshow");

  expect(landing.handoff()).toBeNull();
  expect(landing.fetches).toHaveLength(0);
});


test("entry acknowledgment preserves the first native POST and suppresses a repeated submit", () => {
  const browser = browserHarness({
    action: "/app/account",
    formClass: "me-entry-form",
    controlClasses: ["ae-button"],
  });
  browser.dispatchSubmit();
  expect(browser.controlDisabled()).toBeTrue();
  expect(browser.prevented()).toBe(0);
  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.fetches).toEqual([]);
});

test("capture acknowledgment stays native and cannot enqueue a second capture while pending", () => {
  const browser = browserHarness({
    action: "/app/capture",
    formClass: "me-capture-form",
    controlClasses: ["ae-button"],
  });
  browser.dispatchSubmit();
  expect(browser.controlDisabled()).toBeTrue();
  expect(browser.prevented()).toBe(0);
  browser.dispatchSubmit();
  expect(browser.prevented()).toBe(1);
  expect(browser.fetches).toEqual([]);
});

test("a BFCache-restored landing clears the handoff and never schedules emission", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId));

  landing.dispatchWindow("pageshow", { persisted: true });

  expect(landing.handoff()).toBeNull();
  expect(landing.busy()).toBeFalse();
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(0);
});

test("a two-RAF emission already queued before pagehide is invalidated, not fired stale after BFCache restore", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId));

  landing.dispatchWindow("pageshow");
  expect(landing.pendingFrames()).toBe(1);

  // The tab is hidden and this document is frozen into BFCache before the
  // first animation frame ever paints.
  landing.dispatchWindow("pagehide");
  landing.dispatchWindow("pageshow", { persisted: true });

  // The already-queued frame callback (queued before pagehide) finally
  // fires on resume. It must revalidate and refuse to schedule the second
  // frame or emit anything — the landing it was scheduled for is stale.
  landing.runAnimationFrame();
  expect(landing.pendingFrames()).toBe(0);
  expect(landing.fetches).toHaveLength(0);
});

test("a landing whose verdict disappears before the second frame never emits", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId));

  landing.dispatchWindow("pageshow");
  expect(landing.pendingFrames()).toBe(1);

  // Document state changed between scheduling and the first frame (e.g. a
  // failed re-render). The queued callback must revalidate, not trust the
  // snapshot it captured at schedule time.
  landing.setVerdictPresent(false);
  landing.runAnimationFrame();

  expect(landing.pendingFrames()).toBe(0);
  expect(landing.fetches).toHaveLength(0);
});

test("an expired handoff at landing time is never scheduled for emission", () => {
  const { source, handoff, requestId } = submittedHandoff();
  // now (130000ms past timeOrigin via `now`) is well beyond startedAtMs +
  // the 65s handoff TTL.
  const landing = browserHarness(
    matchingLandingOptions(source, handoff, requestId, { now: 70_000 })
  );

  landing.dispatchWindow("pageshow");

  expect(landing.handoff()).toBeNull();
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(0);
});

test("a landing whose meta request id does not match the rendered Server-Timing never emits", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const mismatchedRequestId = "req_ffffffffffffffffffffffffffffffff";
  const landing = browserHarness(
    matchingLandingOptions(source, handoff, requestId, {
      metas: {
        "memory-engine-submit-request": mismatchedRequestId,
        "memory-engine-submit-handoff": handoff.token,
        "memory-engine-csrf-token": "csrf-test",
      },
    })
  );

  landing.dispatchWindow("pageshow");

  expect(landing.handoff()).toBeNull();
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(0);
});

test("a landing document without Server-Timing on its own navigation entry never emits", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const landing = browserHarness(
    matchingLandingOptions(source, handoff, requestId, {
      navigation: { type: "navigate", startTime: 0, responseStart: 110, responseEnd: 120 },
    })
  );

  landing.dispatchWindow("pageshow");

  expect(landing.handoff()).toBeNull();
  expect(landing.fetches).toHaveLength(0);
  expect(landing.pendingFrames()).toBe(0);
});

test("a second pageshow on the same landing document never emits a duplicate receipt", () => {
  const { source, handoff, requestId } = submittedHandoff();
  const landing = browserHarness(matchingLandingOptions(source, handoff, requestId));

  landing.dispatchWindow("pageshow");
  landing.runAnimationFrame();
  landing.runAnimationFrame();
  expect(landing.fetches).toHaveLength(1);

  landing.dispatchWindow("pageshow");
  expect(landing.pendingFrames()).toBe(0);
  expect(landing.fetches).toHaveLength(1);
});


function jobsListHarness() {
  const meta = { textContent: "old" };
  const row = {
    dataset: { jobId: "job-1", status: "queued" },
    querySelector: (selector) => (selector === ".me-job-meta" ? meta : null),
  };
  return {
    meta,
    row,
    querySelector: () => row,
    insertBefore() {},
  };
}

test("SSE updates Library activity without navigating over editable drafts", () => {
  const eventSource = {};
  const list = jobsListHarness();
  const browser = browserHarness({ eventSource, jobsList: list });

  browser.emitJob({ id: "job-1", status: "running" });
  expect(list.row.dataset.status).toBe("running");
  expect(browser.navigations).toEqual([]);

  browser.emitJob({ id: "job-1", status: "succeeded" });
  expect(list.row.dataset.status).toBe("succeeded");
  expect(browser.navigations).toEqual([]);
});

test("SSE presents authoritative failure text without replacing unsaved Library work", () => {
  const eventSource = {};
  const list = jobsListHarness();
  const browser = browserHarness({ eventSource, jobsList: list });

  browser.emitJob({ id: "job-1", status: "failed", error: "provider unavailable" });
  expect(list.meta.textContent).toBe("provider unavailable");
  expect(browser.navigations).toEqual([]);
});

test("SSE terminal events never navigate away from pages without the jobs surface", () => {
  const eventSource = {};
  const browser = browserHarness({ eventSource, jobsList: null });

  browser.emitJob({ id: "job-1", status: "succeeded" });
  browser.emitJob({ id: "job-1", status: "failed", error: "provider unavailable" });
  expect(browser.navigations).toEqual([]);
});

test("terminal events cannot act on a jobs list removed by an in-place review navigation", () => {
  const list = jobsListHarness();
  const browser = browserHarness({ eventSource: {}, jobsList: list });
  browser.emitJob({ id: "job-1", status: "running" });
  browser.setJobsList(null);
  browser.emitJob({ id: "job-1", status: "succeeded" });
  expect(list.row.dataset.status).toBe("running");
  expect(browser.navigations).toEqual([]);
});

test("only the explicit waiting job can complete capture and terminal replay cannot navigate twice", () => {
  const browser = browserHarness({
    eventSource: {},
    waitingJob: { id: "job-current", destination: "/app/library" },
  });
  browser.emitJob({ id: "job-other", status: "succeeded" });
  browser.emitJob({ id: "job-current", status: "running" });
  expect(browser.navigations).toEqual([]);
  browser.emitJob({ id: "job-current", status: "failed", error: "provider unavailable" });
  expect(browser.generationStatus.textContent).toBe("provider unavailable");
  expect(browser.navigations).toEqual(["/app/library"]);
  browser.dispatchWindow("pagehide");
  browser.dispatchWindow("pageshow", { persisted: true });
  browser.emitJob({ id: "job-current", status: "failed", error: "provider unavailable" });
  expect(browser.navigations).toEqual(["/app/library"]);
});

test("removing the capture waiting surface cancels its future terminal navigation", () => {
  const browser = browserHarness({
    eventSource: {},
    waitingJob: { id: "job-current", destination: "/app/library" },
  });
  browser.setWaitingJob(null);
  browser.emitJob({ id: "job-current", status: "succeeded" });
  expect(browser.navigations).toEqual([]);
});
