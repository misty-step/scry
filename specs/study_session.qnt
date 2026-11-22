// -*- mode: Bluespec; -*-
/**
 * Study session state machine of scry (FSRS-scheduled study sessions).
 *
 * Faithful model of the code at HEAD (fb393b2):
 *   internal/srs/srs.go            Next (FSRS-4.5)
 *   internal/store/study.go        StudySession, Current, StartSession, RecordReview,
 *                                  Advance, EndSession
 *   internal/app/study.go          Study, Answer, Next
 *   internal/app/explain.go        Explain ("Why?")
 *   internal/store/library.go      UpdateItem, RemoveItem
 *   internal/web/handlers_study.go phase rendering (pending review shown, practice
 *                                  review looked up by id when it is not an entry's review)
 *
 * Phases of a session (derived exactly as the web layer derives them):
 *   NoSession      no session has been started
 *   Ended          ended_at != 0
 *   PendingReview  pending_review_id != 0           (the post-answer review: every
 *                                                   judged answer, right ones included,
 *                                                   waits here for "Next card")
 *   RetryingExact  retry_item_id != 0               (an exact text tried again, practice)
 *   Asking         otherwise                        (Current() is the next unanswered entry)
 *
 * Concurrency: Answer is not atomic in Go. It reads the session, judges (a slow model
 * call) and then calls RecordReview, whose transaction does NOT re-check the session.
 * Next is likewise read-then-Advance. Both are modelled as Begin/Commit pairs per
 * browser tab so interleavings (double submit, two tabs, end/restart races) are explored.
 *
 * `step_legacy` is the HEAD behaviour; `step` is the fixed design:
 *   - CommitAnswer re-validates inside the transaction (session still open, nothing
 *     pending, item still the current one / still the retry target, practice only on an
 *     exact item) and computes FSRS from the row it updates, not from the stale read;
 *     "end if that was the last item" targets the same session.
 *   - CommitNext re-validates (session open, pending unchanged, retry target exact).
 *   - EditItem that clears `exact` cancels a retry of that item.
 *   - Ending a session (also via StartSession) clears pending/retry.
 *
 * Slice 41 (post-answer review): CommitAnswer always holds. A model-judged Right used to
 * move straight on (and end the session after the last item); now every scored or
 * practice answer leaves its review pending, and only Next (CommitNext) advances or ends
 * the session. "Edit card" in the review is EditItem keeping `exact`: it changes the
 * item's text only, never the pending review, the graded snapshot or FSRS
 * (inv_answer_held, inv_edit_keeps_schedule, reviewInlineEditTest). "Ask Why?" explains
 * any model-judged review, Right included.
 *
 * Abstractions: 2 items (1 is a line of an exact text, 2 is model-judged), 2 sessions,
 * 4 reviews, 2 tabs, 3 days (0..2). Prompt/expected texts are small version numbers. FSRS
 * numbers are small integers with a deterministic update that keeps the real
 * properties: reps+1; first review sets stability from the grade (w[g-1]) and the
 * initial difficulty; Missed after the first review adds a lapse and does not raise
 * stability; Close/Right grow it; difficulty clamped [1,10]; stability clamped
 * [MIN_STAB, MAX_STAB] (abstracts [0.1, 3650]); Missed due = +1 day, otherwise the
 * interval is >= 1 day.
 */
module study_session {

  // ------------------------------------------------------------------ bounds
  pure val ITEMS: Set[int] = Set(1, 2)
  pure val ITEM_KEYS: Set[int] = 0.to(2)     // 0 is a dummy, never in a session
  pure val SIDS: Set[int] = 1.to(2)
  pure val SID_KEYS: Set[int] = 0.to(2)      // 0 is a dummy, never started
  pure val MAX_SESSION: int = 2
  pure val RIDS: Set[int] = 1.to(4)
  pure val RID_KEYS: Set[int] = 0.to(4)      // 0 is "no review"
  pure val MAX_REVIEW: int = 4
  pure val TABS: Set[int] = Set(1, 2)
  pure val MAX_DAY: int = 2
  pure val ANSWERS: Set[int] = Set(1, 2)
  pure val TEXTS: Set[int] = Set(0, 1)
  pure val VERDICTS: Set[str] = Set("right", "close", "missed")
  pure val MIN_STAB: int = 1
  pure val MAX_STAB: int = 5
  pure val MIN_DIFF: int = 1
  pure val MAX_DIFF: int = 10
  pure val NOT_MEMBER: int = -1

  // ------------------------------------------------------------------ types
  type Fsrs = { reps: int, lapses: int, stab: int, diff: int, last: int, due: int }
  type Item = { prompt: int, expected: int, exact: bool, deleted: bool }
  type Review = {
    exists: bool, item: int, session: int, answer: int, verdict: str,
    judge: str,               // "exact" or "model"
    practice: bool,
    gprompt: int, gexpected: int,   // graded_prompt / graded_expected snapshot
    late: bool                // ghost: written while its session was already ended
  }
  // study_sessions row; session_items live in the separate map `entries`
  type Session = { started: bool, ended: bool, pending: int, retry: int }
  // (session, item) -> session_items.review_id (0 = unanswered, NOT_MEMBER = not in session)
  type Entries = (int, int) -> int
  type Tab = {
    mode: str,                // "idle" | "answer" (judging in flight) | "next" (Next in flight)
    session: int, item: int, practice: bool, exact: bool,
    prompt: int, expected: int, snap: Fsrs, at: int, answer: int,
    retry: int, pending: int  // for Next: decided retry target, pending id read
  }
  type Explained = { ok: bool, review: int, prompt: int, expected: int, answer: int, verdict: str }

  pure val NEW_FSRS: Fsrs = { reps: 0, lapses: 0, stab: 0, diff: 0, last: 0, due: 0 }
  pure val NO_REVIEW: Review = {
    exists: false, item: 0, session: 0, answer: 0, verdict: "", judge: "",
    practice: false, gprompt: 0, gexpected: 0, late: false
  }
  pure val NO_SESSION: Session = { started: false, ended: false, pending: 0, retry: 0 }
  pure val IDLE_TAB: Tab = {
    mode: "idle", session: 0, item: 0, practice: false, exact: false,
    prompt: 0, expected: 0, snap: NEW_FSRS, at: 0, answer: 0, retry: 0, pending: 0
  }
  pure val NO_EXPLAIN: Explained = { ok: false, review: 0, prompt: 0, expected: 0, answer: 0, verdict: "" }

  // ------------------------------------------------------------------ abstract FSRS
  pure def clamp(x: int, lo: int, hi: int): int = if (x < lo) lo else if (x > hi) hi else x
  // w[0..2] = 0.49, 1.40, 3.71 for again/hard/good
  pure def initStab(v: str): int = if (v == "right") 3 else if (v == "close") 2 else 1
  // initDifficulty(g) = w4 - (g-3) w5 : good 5.16, hard 6.39, again 7.62
  pure def initDiff(v: str): int = if (v == "right") 5 else if (v == "close") 6 else 7
  // srs.Next
  pure def nextFsrs(f: Fsrs, v: str, at: int): Fsrs = {
    val first = f.reps == 0 or f.stab <= 0
    val stab =
      if (first) initStab(v)
      else if (v == "missed") clamp(f.stab - 1, MIN_STAB, MAX_STAB)   // min(forget, S)
      else if (v == "close") clamp(f.stab + 1, MIN_STAB, MAX_STAB)
      else clamp(f.stab + 2, MIN_STAB, MAX_STAB)
    val diff =
      if (first) initDiff(v)
      else clamp(f.diff + (if (v == "missed") 2 else if (v == "close") 1 else 0), MIN_DIFF, MAX_DIFF)
    val lapses = if (not(first) and v == "missed") f.lapses + 1 else f.lapses
    val interval = if (v == "missed") 1 else clamp(stab, 1, MAX_STAB)
    { reps: f.reps + 1, lapses: lapses, stab: stab, diff: diff, last: at, due: at + interval }
  }

  // ------------------------------------------------------------------ session helpers
  pure def isMember(en: Entries, s: int, i: int): bool = en.get((s, i)) != NOT_MEMBER
  // store.session: (t.deleted_at = 0 OR si.review_id != 0)
  pure def visible(en: Entries, its: int -> Item, s: int, i: int): bool =
    isMember(en, s, i) and (not(its.get(i).deleted) or en.get((s, i)) != 0)
  // StudySession.Current (entries ordered by item id)
  pure def currentOf(s: int, se: Session, en: Entries, its: int -> Item): int =
    if (ITEMS.exists(i => se.retry == i and visible(en, its, s, i))) se.retry
    else ITEMS.fold(0, (acc, i) =>
      if (visible(en, its, s, i) and en.get((s, i)) == 0 and (acc == 0 or i < acc)) i else acc)
  pure def openOf(se: Session): bool = se.started and not(se.ended)
  pure def activeOf(ss: int -> Session): int =
    SIDS.fold(0, (acc, s) => if (openOf(ss.get(s))) s else acc)
  pure def phaseOf(se: Session): str =
    if (not(se.started)) "NoSession"
    else if (se.ended) "Ended"
    else if (se.pending != 0) "PendingReview"
    else if (se.retry != 0) "RetryingExact"
    else "Asking"
  pure def endSess(se: Session): Session = se.with("ended", true).with("pending", 0).with("retry", 0)

  // ------------------------------------------------------------------ state
  var now: int
  var items: int -> Item
  var fsrs: int -> Fsrs
  var sessions: int -> Session
  var entries: Entries
  var nextSession: int
  var reviews: int -> Review
  var nextReview: int
  var tabs: int -> Tab
  // ghost / history
  var reviewSnap: int -> Review    // each review row as it was when inserted
  var prevFsrs: int -> Fsrs        // item FSRS before the last step
  var lastAction: str
  var lastResult: str              // "ok" | "rejected"
  var explained: Explained         // the last "Why?" answer

  // derived
  def active: int = activeOf(sessions)
  def latest: int = nextSession - 1
  def phase: str = if (latest == 0) "NoSession" else phaseOf(sessions.get(latest))
  def current: int = if (active == 0) 0 else currentOf(active, sessions.get(active), entries, items)
  def entry(s: int, i: int): int = entries.get((s, i))

  action init = all {
    now' = 0,
    items' = ITEM_KEYS.mapBy(i =>
      if (i == 0) { prompt: 0, expected: 0, exact: false, deleted: true }
      else { prompt: 0, expected: 0, exact: i == 1, deleted: false }),
    fsrs' = ITEM_KEYS.mapBy(_ => NEW_FSRS),
    sessions' = SID_KEYS.mapBy(_ => NO_SESSION),
    entries' = tuples(SID_KEYS, ITEM_KEYS).mapBy(_ => NOT_MEMBER),
    nextSession' = 1,
    reviews' = RID_KEYS.mapBy(_ => NO_REVIEW),
    nextReview' = 1,
    tabs' = TABS.mapBy(_ => IDLE_TAB),
    reviewSnap' = RID_KEYS.mapBy(_ => NO_REVIEW),
    prevFsrs' = ITEM_KEYS.mapBy(_ => NEW_FSRS),
    lastAction' = "init",
    lastResult' = "ok",
    explained' = NO_EXPLAIN,
  }

  // Assign every variable; ghosts follow automatically.
  action update(nNow: int, nItems: int -> Item, nFsrs: int -> Fsrs, nSessions: int -> Session,
                nEntries: Entries, nNextSession: int, nReviews: int -> Review, nNextReview: int, nTabs: int -> Tab,
                nSnap: int -> Review, nExplained: Explained, act: str, res: str): bool = all {
    now' = nNow, items' = nItems, fsrs' = nFsrs, sessions' = nSessions, entries' = nEntries,
    nextSession' = nNextSession, reviews' = nReviews, nextReview' = nNextReview, tabs' = nTabs,
    reviewSnap' = nSnap, explained' = nExplained,
    prevFsrs' = fsrs, lastAction' = act, lastResult' = res,
  }
  action onlyTabs(nTabs: int -> Tab, act: str, res: str): bool =
    update(now, items, fsrs, sessions, entries, nextSession, reviews, nextReview, nTabs, reviewSnap, explained, act, res)
  action onlySessions(nSessions: int -> Session, nTabs: int -> Tab, act: str, res: str): bool =
    update(now, items, fsrs, nSessions, entries, nextSession, reviews, nextReview, nTabs, reviewSnap, explained, act, res)

  // ------------------------------------------------------------------ transitions

  // App.Study -> StoreStartSession. StartSession ends ANY open session (two tabs both
  // calling Study, or Study racing an answer, reach it with an open session), then opens
  // a new one over the due items. HEAD's UPDATE only sets ended_at (study.go:92), so a
  // pending/retry survives on the ended row; the fix ends it like EndSession.
  action start(legacy: bool): bool = {
    val due = ITEMS.filter(i => not(items.get(i).deleted) and fsrs.get(i).due <= now)
    val sid = nextSession
    val closed = SID_KEYS.mapBy(s => {
      val se = sessions.get(s)
      if (openOf(se)) (if (legacy) se.with("ended", true) else endSess(se)) else se
    })
    val fresh = { started: true, ended: false, pending: 0, retry: 0 }
    val nEntries = ITEMS.fold(entries, (en, i) => en.set((sid, i), if (due.contains(i)) 0 else NOT_MEMBER))
    all {
      sid <= MAX_SESSION,
      due != Set(),
      update(now, items, fsrs, closed.set(sid, fresh), nEntries, sid + 1, reviews, nextReview, tabs,
             reviewSnap, explained, "Start", "ok"),
    }
  }

  // App.Answer's checks before judging (study.go:98-104).
  def answerAccepted(i: int): bool = {
    val se = sessions.get(active)
    active != 0 and se.pending == 0 and currentOf(active, se, entries, items) == i
  }
  // What App.Answer has read when it calls the judge.
  def answeringTab(i: int, a: int): Tab = {
    mode: "answer", session: active, item: i, practice: sessions.get(active).retry == i,
    exact: items.get(i).exact, prompt: items.get(i).prompt, expected: items.get(i).expected,
    snap: fsrs.get(i), at: now, answer: a, retry: 0, pending: 0
  }

  // App.Answer up to the judge call: read the active session and validate.
  // Stale submissions (non-current item, answer while PendingReview, no open session)
  // are rejected with no change (ErrStale).
  action beginAnswer(t: int, i: int, a: int): bool = {
    val tb = tabs.get(t)
    all {
      tb.mode == "idle",
      if (answerAccepted(i))
        onlyTabs(tabs.set(t, answeringTab(i, a)), "BeginAnswer", "ok")
      else
        onlyTabs(tabs, "BeginAnswer", "rejected"),
    }
  }

  // The model could not be reached: nothing is recorded (ErrJudgeUnavailable).
  action abortAnswer(t: int): bool = all {
    tabs.get(t).mode == "answer",
    not(tabs.get(t).exact),
    onlyTabs(tabs.set(t, IDLE_TAB), "JudgeUnavailable", "rejected"),
  }

  // App.Answer after judging: srs.Next + store.RecordReview (+ end if last).
  action commitAnswer(legacy: bool, t: int, v: str): bool = all {
    tabs.get(t).mode == "answer",
    commitWith(legacy, t, tabs.get(t), v),
  }

  // Answer with nothing interleaved between the read and the write (one step).
  action answerNow(legacy: bool, t: int, i: int, a: int, v: str): bool = all {
    tabs.get(t).mode == "idle",
    if (answerAccepted(i)) commitWith(legacy, t, answeringTab(i, a), v)
    else onlyTabs(tabs, "BeginAnswer", "rejected"),
  }

  action commitWith(legacy: bool, t: int, tb: Tab, v: str): bool = {
    val s = tb.session
    val i = tb.item
    val se = sessions.get(s)
    val stillValid = openOf(se) and se.pending == 0 and currentOf(s, se, entries, items) == i
                     and (se.retry == i) == tb.practice
                     and (not(tb.practice) or items.get(i).exact)
    val id = nextReview
    val rev = { exists: true, item: i, session: s, answer: tb.answer, verdict: v,
                judge: if (tb.exact) "exact" else "model", practice: tb.practice,
                gprompt: tb.prompt, gexpected: tb.expected, late: not(openOf(se)) }
    // HEAD: next := srs.Next(item.Sched, ...) from the read before judging (study.go:158)
    val base = if (legacy) tb.snap else fsrs.get(i)
    val nFsrs = if (tb.practice) fsrs else fsrs.set(i, nextFsrs(base, v, tb.at))
    // UPDATE session_items SET review_id = id ... AND review_id = 0  (scored only)
    val nEntries =
      if (not(tb.practice) and entries.get((s, i)) == 0) entries.set((s, i), id) else entries
    // Every answer is held: the review of this answer is pending until Next. Answering
    // never advances the session and never ends it, not even on the last item.
    val ss2 = sessions.set(s, se.with("pending", id).with("retry", 0))
    all {
      not(tb.exact) or v != "close",       // exact.Diff: right or missed only
      nextReview <= MAX_REVIEW,
      if (legacy or stillValid)
        update(now, items, nFsrs, ss2, nEntries, nextSession, reviews.set(id, rev), id + 1,
               tabs.set(t, IDLE_TAB), reviewSnap.set(id, rev), explained,
               if (tb.practice) "CommitPractice" else "CommitScored", "ok")
      else
        onlyTabs(tabs.set(t, IDLE_TAB), "CommitAnswer", "rejected"),
    }
  }

  // App.Next up to Advance: read the session; with again, an exact pending item is
  // put back up. Next with nothing open is a no-op.
  action beginNext(t: int, again: bool): bool = {
    val tb = tabs.get(t)
    all {
      tb.mode == "idle",
      if (active != 0) onlyTabs(tabs.set(t, nextTab(again)), "BeginNext", "ok")
      else onlyTabs(tabs, "BeginNext", "rejected"),
    }
  }

  // What App.Next has read when it calls Advance (study.go:177-189).
  def nextTab(again: bool): Tab = {
    val se = sessions.get(active)
    val pr = reviews.get(se.pending)
    // us.Item filters deleted_at = 0
    val retry = if (again and se.pending != 0 and
                    ITEMS.exists(i => pr.item == i and items.get(i).exact and not(items.get(i).deleted)))
                  pr.item else 0
    IDLE_TAB.with("mode", "next").with("session", active).with("retry", retry).with("pending", se.pending)
  }

  // store.Advance (+ end if nothing is left). Covers Advance (PendingReview -> Asking/Ended),
  // AdvanceRetry (PendingReview -> RetryingExact) and skip retry (RetryingExact -> Asking).
  action commitNext(legacy: bool, t: int): bool = all {
    tabs.get(t).mode == "next",
    commitNextWith(legacy, t, tabs.get(t)),
  }

  // Next with nothing interleaved (one step).
  action nextNow(legacy: bool, t: int, again: bool): bool = all {
    tabs.get(t).mode == "idle",
    if (active != 0) commitNextWith(legacy, t, nextTab(again))
    else onlyTabs(tabs, "BeginNext", "rejected"),
  }

  action commitNextWith(legacy: bool, t: int, tb: Tab): bool = {
    val s = tb.session
    val se = sessions.get(s)
    val valid = openOf(se) and se.pending == tb.pending
                and (tb.retry == 0 or items.get(tb.retry).exact)
    val ss1 = sessions.set(s, se.with("pending", 0).with("retry", tb.retry))
    val target = if (legacy) activeOf(ss1) else s
    val tse = ss1.get(target)
    val ss2 = if (tb.retry == 0 and target != 0 and openOf(tse) and currentOf(target, tse, entries, items) == 0)
                ss1.set(target, endSess(tse)) else ss1
    val act = if (tb.retry != 0) "AdvanceRetry" else if (tb.pending != 0) "Advance" else "SkipRetry"
    all {
      if (legacy or valid) onlySessions(ss2, tabs.set(t, IDLE_TAB), act, "ok")
      else onlySessions(sessions, tabs.set(t, IDLE_TAB), act, "rejected"),
    }
  }

  // store.EndSession (only an open session; clears pending and retry).
  action endSession: bool = all {
    active != 0,
    onlySessions(sessions.set(active, endSess(sessions.get(active))), tabs, "End", "ok"),
  }

  // store.UpdateItem: prompt/answer/exact may change at any time, mid-session too.
  action editItem(legacy: bool, i: int, p: int, e: int, x: bool): bool = {
    val it = items.get(i)
    val nSessions = if (legacy or x) sessions else SID_KEYS.mapBy(s => {
      val se = sessions.get(s)
      if (openOf(se) and se.retry == i) se.with("retry", 0) else se
    })
    all {
      not(it.deleted),
      it.prompt != p or it.expected != e or it.exact != x,
      update(now, items.set(i, it.with("prompt", p).with("expected", e).with("exact", x)), fsrs,
             nSessions, entries, nextSession, reviews, nextReview, tabs, reviewSnap, explained, "EditItem", "ok"),
    }
  }

  // store.RemoveItem
  action removeItem(i: int): bool = all {
    not(items.get(i).deleted),
    update(now, items.set(i, items.get(i).with("deleted", true)), fsrs, sessions, entries, nextSession,
           reviews, nextReview, tabs, reviewSnap, explained, "RemoveItem", "ok"),
  }

  // store.RestoreIdea (brings a removed item back).
  action restoreItem(i: int): bool = all {
    items.get(i).deleted,
    update(now, items.set(i, items.get(i).with("deleted", false)), fsrs, sessions, entries, nextSession,
           reviews, nextReview, tabs, reviewSnap, explained, "RestoreItem", "ok"),
  }

  // App.Explain ("Ask Why?"): read-only; any model-judged review (Right, Close or
  // Missed), never an exact text; uses the review's snapshot.
  action explain(r: int): bool = {
    val rv = reviews.get(r)
    val ok = rv.judge == "model"
    all {
      rv.exists,
      update(now, items, fsrs, sessions, entries, nextSession, reviews, nextReview, tabs, reviewSnap,
             if (ok) { ok: true, review: r, prompt: rv.gprompt, expected: rv.gexpected,
                       answer: rv.answer, verdict: rv.verdict }
             else NO_EXPLAIN.with("review", r),
             "Explain", if (ok) "ok" else "rejected"),
    }
  }

  action tick: bool = all {
    now < MAX_DAY,
    update(now + 1, items, fsrs, sessions, entries, nextSession, reviews, nextReview, tabs, reviewSnap,
           explained, "Tick", "ok"),
  }

  // ------------------------------------------------------------------ steps
  // `step` / `step_legacy` are the canonical machines (every request split into its read
  // and its write, so all interleavings are possible). The *_fast variants add the
  // one-step compositions answerNow/nextNow, which are behaviours the split machine
  // already has (Begin immediately followed by Commit); they only help the random
  // simulator reach deep states. Apalache is run on `step`.
  action stepWith(legacy: bool): bool = {
    nondet t = TABS.oneOf()
    nondet i = ITEMS.oneOf()
    nondet a = ANSWERS.oneOf()
    nondet v = VERDICTS.oneOf()
    nondet again = Set(true, false).oneOf()
    nondet p = TEXTS.oneOf()
    nondet e = TEXTS.oneOf()
    nondet x = Set(true, false).oneOf()
    nondet r = RIDS.oneOf()
    any {
      start(legacy),
      beginAnswer(t, i, a),
      abortAnswer(t),
      commitAnswer(legacy, t, v),
      beginNext(t, again),
      commitNext(legacy, t),
      endSession,
      editItem(legacy, i, p, e, x),
      removeItem(i),
      restoreItem(i),
      explain(r),
      tick,
    }
  }
  action fastWith(legacy: bool): bool = {
    nondet t = TABS.oneOf()
    nondet i = ITEMS.oneOf()
    nondet a = ANSWERS.oneOf()
    nondet v = VERDICTS.oneOf()
    nondet again = Set(true, false).oneOf()
    any {
      answerNow(legacy, t, i, a, v),
      nextNow(legacy, t, again),
    }
  }
  action step = stepWith(false)
  action step_legacy = stepWith(true)
  action step_fast = any { stepWith(false), fastWith(false) }
  action step_legacy_fast = any { stepWith(true), fastWith(true) }

  // ------------------------------------------------------------------ counting helpers
  def scoredCount(s: int, i: int): int = RIDS.fold(0, (acc, r) => {
    val rv = reviews.get(r)
    if (rv.exists and not(rv.practice) and rv.session == s and rv.item == i) acc + 1 else acc
  })
  def scoredTotal(i: int): int = RIDS.fold(0, (acc, r) => {
    val rv = reviews.get(r)
    if (rv.exists and not(rv.practice) and rv.item == i) acc + 1 else acc
  })

  // ------------------------------------------------------------------ invariants

  val inv_one_open_session: bool =
    SIDS.forall(a => SIDS.forall(b => (openOf(sessions.get(a)) and openOf(sessions.get(b))) implies a == b))

  // PendingReview points at a review of this session: either the scored review that is
  // the item's session_items.review_id, or (after an exact retry) a practice review of an
  // item already answered in this session, judged word for word, which is NOT the
  // entry's review (handlers_study.go looks it up by id).
  // (Lookups are written as "forall k: x == k implies P(m.get(k))" so that Apalache
  // only indexes maps by constants.)
  val inv_pending_wellformed: bool = SIDS.forall(s => {
    val se = sessions.get(s)
    and {
      se.pending == 0 or RIDS.contains(se.pending),
      RIDS.forall(r => se.pending == r implies {
        val rv = reviews.get(r)
        rv.exists and rv.session == s and ITEMS.contains(rv.item) and
        ITEMS.forall(i => rv.item == i implies and {
          isMember(entries, s, i),
          if (rv.practice) entries.get((s, i)) != 0 and entries.get((s, i)) != r and rv.judge == "exact"
          else entries.get((s, i)) == r,
        })
      }),
    }
  })

  // RetryingExact only targets an answered exact=true entry, and never alongside a pending.
  val inv_retry_targets_exact: bool = SIDS.forall(s => {
    val se = sessions.get(s)
    and {
      se.retry == 0 or ITEMS.contains(se.retry),
      ITEMS.forall(i => (openOf(se) and se.retry == i) implies and {
        se.pending == 0, isMember(entries, s, i), entries.get((s, i)) > 0, items.get(i).exact,
      }),
    }
  })

  // Ended sessions are clean and accept no further answers.
  val inv_ended_clean: bool = SIDS.forall(s =>
    sessions.get(s).ended implies (sessions.get(s).pending == 0 and sessions.get(s).retry == 0))
  val inv_no_late_writes: bool = RIDS.forall(r => not(reviews.get(r).exists and reviews.get(r).late))

  // Practice reviews are only made of exact texts (judged word for word).
  val inv_practice_is_exact: bool = RIDS.forall(r =>
    (reviews.get(r).exists and reviews.get(r).practice) implies reviews.get(r).judge == "exact")

  val inv_session_phase_consistency: bool = all {
    inv_one_open_session, inv_pending_wellformed, inv_retry_targets_exact,
    inv_ended_clean, inv_no_late_writes, inv_practice_is_exact,
  }

  val inv_fsrs_bounds: bool = ITEMS.forall(i => {
    val f = fsrs.get(i)
    val pf = prevFsrs.get(i)
    all {
      f.reps >= 0, f.lapses >= 0, f.lapses <= f.reps,
      f.reps >= pf.reps, f.lapses >= pf.lapses,            // monotone
      f.last <= now,
      f.reps > 0 implies all {
        MIN_STAB <= f.stab, f.stab <= MAX_STAB,
        MIN_DIFF <= f.diff, f.diff <= MAX_DIFF,
        f.due >= f.last + 1,
      },
    }
  })
  // No double FSRS application: one scored review per item per session, and reps is
  // exactly the number of scored reviews of the item.
  val inv_no_double_scoring: bool = all {
    SIDS.forall(s => ITEMS.forall(i => scoredCount(s, i) <= 1)),
    ITEMS.forall(i => fsrs.get(i).reps == scoredTotal(i)),
  }
  val inv_fsrs_bounds_and_monotonicity: bool = inv_fsrs_bounds and inv_no_double_scoring

  val inv_practice_does_not_mutate_fsrs: bool = all {
    lastAction == "CommitPractice" implies fsrs == prevFsrs,
    // session_items.review_id always names this entry's scored review, never a practice one
    SIDS.forall(s => ITEMS.forall(i => {
      val en = entries.get((s, i))
      and {
        en <= 0 or RIDS.contains(en),
        RIDS.forall(r => en == r implies {
          val rv = reviews.get(r)
          rv.exists and not(rv.practice) and rv.item == i and rv.session == s
        }),
      }
    })),
  }

  val inv_explain_immutable: bool = all {
    RIDS.forall(r => reviews.get(r) == reviewSnap.get(r)),
    explained.ok implies and {
      RIDS.contains(explained.review),
      RIDS.forall(r => explained.review == r implies {
        val rv = reviewSnap.get(r)
        all {
          rv.exists, rv.judge == "model", VERDICTS.contains(rv.verdict),
          explained.prompt == rv.gprompt, explained.expected == rv.gexpected,
          explained.answer == rv.answer, explained.verdict == rv.verdict,
        }
      }),
    },
    lastAction == "Explain" implies fsrs == prevFsrs,
  }

  // The post-answer review: an answer that was recorded is on screen. The review just
  // written is the pending review of an open session, so the session cannot have moved
  // on to the next item or ended by itself.
  val inv_answer_held: bool =
    ((lastAction == "CommitScored" or lastAction == "CommitPractice") and lastResult == "ok") implies
      SIDS.exists(s => openOf(sessions.get(s)) and sessions.get(s).pending == nextReview - 1)

  // "Edit card" changes an item's text only: no schedule moves on an edit. (That no
  // review row changes is the first clause of inv_explain_immutable.)
  val inv_edit_keeps_schedule: bool = lastAction == "EditItem" implies fsrs == prevFsrs

  val inv_all: bool = all {
    inv_session_phase_consistency,
    inv_fsrs_bounds_and_monotonicity,
    inv_practice_does_not_mutate_fsrs,
    inv_explain_immutable,
    inv_answer_held,
    inv_edit_keeps_schedule,
  }

  // ------------------------------------------------------------------ scenarios
  // Item 1 is exact, item 2 is model-judged; both are due on day 0.
  run answer(legacy: bool, t: int, i: int, a: int, v: str): bool =
    beginAnswer(t, i, a).then(commitAnswer(legacy, t, v))
  run pressEnter(legacy: bool, again: bool): bool = beginNext(1, again).then(commitNext(legacy, 1))

  run fullSessionLifecycleTest =
    init
      .then(start(false))
      .expect(phase == "Asking" and current == 1)
      .then(beginAnswer(1, 1, 1))
      .expect(tabs.get(1).mode == "answer" and phase == "Asking")
      .then(commitAnswer(false, 1, "missed"))   // every answer holds
      .expect(phase == "PendingReview" and sessions.get(1).pending == 1
              and fsrs.get(1).reps == 1 and fsrs.get(1).due == 1 and fsrs.get(1).lapses == 0)
      .then(pressEnter(false, false))
      .expect(phase == "Asking" and current == 2)
      .then(beginAnswer(2, 2, 1))
      .then(commitAnswer(false, 2, "close"))    // model Close holds
      .expect(phase == "PendingReview" and entry(1, 2) == 2)
      .then(pressEnter(false, false))                // nothing left
      .expect(phase == "Ended" and active == 0 and inv_all)

  run staleItemRejectionTest =
    init
      .then(start(false))
      .then(beginAnswer(1, 2, 1))              // not the current item
      .expect(lastResult == "rejected" and tabs.get(1).mode == "idle" and fsrs.get(2) == NEW_FSRS)
      .then(answer(false, 1, 1, 1, "right"))
      .expect(phase == "PendingReview")
      .then(beginAnswer(1, 2, 1))              // answer while PendingReview
      .expect(lastResult == "rejected" and nextReview == 2)
      .then(endSession)
      .then(beginAnswer(1, 2, 1))              // answer to an Ended session
      .expect(lastResult == "rejected" and nextReview == 2 and phase == "Ended" and inv_all)

  run exactPracticeRetryTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "missed"))
      .then(pressEnter(false, true))                 // "Again"
      .expect(phase == "RetryingExact" and current == 1 and sessions.get(1).retry == 1)
      .then(beginAnswer(1, 1, 2))
      .expect(tabs.get(1).practice)
      .then(commitAnswer(false, 1, "right"))
      .expect(all {
        lastAction == "CommitPractice", phase == "PendingReview",
        sessions.get(1).pending == 2,                  // pending = the practice review
        entry(1, 1) == 1,           // entry keeps its scored review
        fsrs.get(1) == prevFsrs.get(1), fsrs.get(1).reps == 1,
        reviews.get(2).practice, inv_all,
      })
      .then(pressEnter(false, true))                 // again from a practice review
      .expect(phase == "RetryingExact")
      .then(pressEnter(false, false))                // skip the retry
      .expect(phase == "Asking" and current == 2 and inv_all)

  run editAndExplainTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "right"))
      .then(pressEnter(false, false))
      .then(answer(false, 1, 2, 2, "missed"))
      .expect(phase == "PendingReview" and reviews.get(2).judge == "model")
      .then(editItem(false, 2, 1, 1, false))   // the item is reworded
      .then(explain(2))
      .expect(all {
        explained.ok, explained.review == 2,
        explained.prompt == 0, explained.expected == 0, explained.answer == 2,
        items.get(2).prompt == 1,                    // not the current text
        reviews.get(2) == reviewSnap.get(2), fsrs == prevFsrs, inv_all,
      })
      .then(explain(1))                        // exact-judged: not explainable
      .expect(not(explained.ok) and lastResult == "rejected" and inv_all)

  run sessionCompletionTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "missed"))
      .then(pressEnter(false, false))
      .then(answer(false, 1, 2, 1, "right"))   // model Right holds too, even on the last item
      .expect(phase == "PendingReview" and sessions.get(1).pending == 2 and active == 1
              and fsrs.get(1).reps == 1 and fsrs.get(1).due == 1 and fsrs.get(2).reps == 1 and fsrs.get(2).due == 3
              and inv_all)
      .then(pressEnter(false, false))          // only Next ends the session
      .expect(phase == "Ended" and sessions.get(1).pending == 0 and inv_all)
      .then(tick)
      .then(start(false))                      // item 1 is due again on day 1
      .expect(phase == "Asking" and active == 2 and current == 1 and entry(2, 2) == NOT_MEMBER)
      .then(answer(false, 1, 1, 1, "missed"))  // lapse after the first review
      .expect(fsrs.get(1).lapses == 1 and fsrs.get(1).reps == 2 and fsrs.get(1).due == 2 and inv_all)

  // Slice 41: a Right answer stops on its review; "Edit card" there rewords the item
  // and changes nothing else; "Ask Why?" explains the Right answer as it was graded;
  // Next then moves on.
  run reviewInlineEditTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "right"))
      .then(pressEnter(false, false))
      .then(answer(false, 1, 2, 2, "right"))   // model-judged Right
      .expect(phase == "PendingReview" and sessions.get(1).pending == 2 and active == 1 and inv_all)
      .then(editItem(false, 2, 1, 1, false))   // Edit card: new question and answer, exact kept
      .expect(all {
        phase == "PendingReview", sessions.get(1).pending == 2, active == 1,
        items.get(2).prompt == 1, items.get(2).expected == 1, not(items.get(2).exact),
        reviews.get(2) == reviewSnap.get(2), reviews.get(2).gprompt == 0, reviews.get(2).gexpected == 0,
        fsrs == prevFsrs, entry(1, 2) == 2, inv_all,
      })
      .then(explain(2))                        // Ask Why? on a Right answer
      .expect(all {
        explained.ok, explained.verdict == "right",
        explained.prompt == 0, explained.expected == 0,   // the graded text, not the edit
        fsrs == prevFsrs, phase == "PendingReview", inv_all,
      })
      .then(pressEnter(false, false))
      .expect(phase == "Ended" and items.get(2).prompt == 1 and inv_all)

  // Legacy bug 1a: double submit / two tabs on the same current item.
  run legacyDoubleSubmitTest =
    init
      .then(start(true))
      .then(answer(true, 1, 1, 1, "right"))
      .then(pressEnter(true, false))
      .then(beginAnswer(1, 2, 1))
      .then(beginAnswer(2, 2, 2))              // second tab, same item, both pass the check
      .then(commitAnswer(true, 1, "missed"))
      .then(commitAnswer(true, 2, "close"))
      .expect(all {
        scoredCount(1, 2) == 2,                       // two scored reviews in one session
        fsrs.get(2).reps == 1,                        // written twice from the same read
        entry(1, 2) == 2,          // entry keeps only the first
        sessions.get(1).pending == 3,                 // pending names an unattached review
        not(inv_no_double_scoring), not(inv_pending_wellformed), not(inv_all),
      })

  run fixedDoubleSubmitTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "right"))
      .then(pressEnter(false, false))
      .then(beginAnswer(1, 2, 1))
      .then(beginAnswer(2, 2, 2))
      .then(commitAnswer(false, 1, "missed"))
      .then(commitAnswer(false, 2, "close"))
      .expect(lastResult == "rejected" and scoredCount(1, 2) == 1 and fsrs.get(2).reps == 1 and inv_all)

  // Legacy bug 1b: a submit racing a restart writes into the ended session.
  run legacyAnswerIntoEndedSessionTest =
    init
      .then(start(true))
      .then(beginAnswer(1, 1, 1))
      .then(start(true))                       // another tab: Study -> StartSession
      .then(commitAnswer(true, 1, "missed"))
      .expect(all {
        sessions.get(1).ended, sessions.get(1).pending == 1, reviews.get(1).late,
        fsrs.get(1).reps == 1,                        // FSRS moved by an ended session
        not(inv_ended_clean), not(inv_no_late_writes),
      })

  run fixedAnswerIntoEndedSessionTest =
    init
      .then(start(false))
      .then(beginAnswer(1, 1, 1))
      .then(start(false))
      .then(commitAnswer(false, 1, "missed"))
      .expect(lastResult == "rejected" and nextReview == 1 and fsrs.get(1) == NEW_FSRS and inv_all)

  // Legacy bug 2: the retry target is edited to exact=false mid-retry.
  run legacyRetryEditTest =
    init
      .then(start(true))
      .then(answer(true, 1, 1, 1, "missed"))
      .then(pressEnter(true, true))
      .expect(phase == "RetryingExact")
      .then(editItem(true, 1, 0, 0, false))
      .expect(phase == "RetryingExact" and not(items.get(1).exact) and not(inv_retry_targets_exact))
      .then(beginAnswer(1, 1, 2))
      .expect(tabs.get(1).practice and not(tabs.get(1).exact))   // goes to the model, as practice
      .then(commitAnswer(true, 1, "close"))
      .expect(reviews.get(2).practice and reviews.get(2).judge == "model"
              and not(inv_practice_is_exact) and not(inv_session_phase_consistency))

  run fixedRetryEditTest =
    init
      .then(start(false))
      .then(answer(false, 1, 1, 1, "missed"))
      .then(pressEnter(false, true))
      .then(editItem(false, 1, 0, 0, false))
      .expect(phase == "Asking" and current == 2 and inv_all)

  // Legacy hazard 3: Next racing a restart sets retry on the ended session.
  run legacyStaleAdvanceTest =
    init
      .then(start(true))
      .then(answer(true, 1, 1, 1, "missed"))
      .then(beginNext(1, true))
      .then(start(true))
      .then(commitNext(true, 1))
      .expect(sessions.get(1).ended and sessions.get(1).retry == 1 and not(inv_ended_clean))
}
