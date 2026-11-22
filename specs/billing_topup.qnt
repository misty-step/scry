// -*- mode: Bluespec; -*-
//
// billing_topup.qnt: top-up attempts, the monthly $20 UTC cap, the
// processor's PaymentIntent outcomes, webhooks (duplicated, reordered),
// reconciliation lookups, and the subscription lifecycle.
//
// Faithful to (read-only sources):
//   internal/store/topups.go   ReserveTopup, MonthTopups, NoteTopupCharge,
//                              SucceedTopup, FailTopup, TopupByCharge
//   internal/app/plan.go       TopUpResult/topUpClaimed/finishCharge/succeed,
//                              settleExisting/reconcile/ReconcileTopUps,
//                              applyTopupEvent, ApplyEvent/applyEvent,
//                              HasAccess, StartTrial
//   internal/billing/stripe.go stripeIntent.result, Charge, FindCharge,
//                              ParseWebhook
//   internal/store/billing.go  OpenPlan, SaveSubscription
//   internal/store/users.go    MarkEvent (processed_events dedupe)
//
// Two step relations:
//   step_legacy  the current code.
//   step         the proposed fix:
//     (1) SucceedTopup on a FAILED attempt re-checks the month cap under the
//         write lock: credited if a slot is free, otherwise the attempt
//         becomes "refund_due" (charge_ref kept, no credit, refund owed), so
//         money taken is neither silently kept nor credited past the cap;
//     (2) subscription events are ignored once the plan is "ended" (until an
//         explicit Reopen opens a NEW subscription_ref), invoice events for a
//         period older than the current one are ignored, InvoicePaid only
//         resets period_used when the period advances, and InvoiceFailed for
//         a period that is not newer than the current one is ignored (this
//         needs the failed invoice's period, which ParseWebhook does not
//         parse today: see the report).
//
// Abstractions:
//   * one learner; form tokens are 1..5 (5 are needed to exceed a cap of 4);
//   * the PaymentIntent for token t has id t (charge_ref 0 = none); the
//     idempotency key "topup-<uid>-<t>" guarantees at most one PI per token;
//   * UTC months are 0..1, subscription periods 0..2, subscription refs 1..2;
//   * money is in micros: each $5 top-up credits TOPUP_CREDIT = 4_000_000;
//   * webhook deliveries are drawn from the set of every event the processor
//     ever emitted, never consumed: that is duplicate AND out-of-order
//     delivery, before or after the synchronous reply or a reconciliation;
//   * "canceled with a Stripe-named reason" is folded into "rpm" (decline);
//   * slice 36: the no-card trial every account starts with is a trialing
//     row with subscription ref 0: no processor subscription (customer_ref
//     ''), so no invoice or end event can name it (event refs are 1..2),
//     and ReserveTopup refuses it ("needs_card", app.ErrTopupNeedsCard)
//     before any attempt or charge. AddCard (store.AddCardToTrial) puts
//     processor subscription 1 behind it; TrialExpire ends it at its
//     period_end; Reopen after that opens a paid plan, charged today.

module billing_topup {

  // ---------------------------------------------------------------- constants

  pure val TOPUP_CAP = 4                 // store.TopupCap
  pure val TOPUP_CREDIT = 4000000        // Cfg.TopupMicros per $5 top-up
  pure val TOKENS = 1.to(5)
  pure val MONTHS = 0.to(1)
  pure val MAX_MONTH = 1
  pure val MAX_PERIOD = 2
  pure val PERIODS = 1.to(MAX_PERIOD)    // invoice periods (0 = the opening period)
  pure val PI_EVENT_KINDS = Set("pi_succeeded", "pi_failed", "pi_canceled")
  pure val REFS = 1.to(2)
  pure val MAX_REF = 2
  // invCode(r, p) = r * 10 + p for every invoice event (ref r, period p)
  pure val INV_CODES = Set(11, 12, 21, 22)

  // PaymentIntent statuses at the processor. "unsent": Charge was never
  // sent (crash before the call). "none": sent, the processor shows nothing
  // (yet). "rpm" = requires_payment_method (a decline).
  pure val PI_STATUSES = Set("unsent", "none", "processing", "requires_action", "succeeded", "rpm", "canceled")

  // What the synchronous Charge call can do:
  //   succeeded / processing / requires_action / rpm / canceled: a PI with
  //     that status, reply delivered (stripeIntent.result mapping);
  //   error_ref:    ProcessorError naming the PI (pi is processing);
  //   lost_created: reply lost / unknown error, PI created (processing);
  //   lost_none:    reply lost, nothing visible at the processor yet.
  pure val OUTCOMES = Set("succeeded", "processing", "requires_action", "rpm", "canceled",
                          "error_ref", "lost_created", "lost_none")

  pure val PI_TARGETS = Set("processing", "requires_action", "succeeded", "rpm", "canceled")

  // ---------------------------------------------------------------- types

  // One topups row. status: none (no row) | pending | succeeded | failed |
  // refund_due (fixed model only). awaiting: this process holds the
  // in-flight claim (claimTopup) and has not processed the Charge reply.
  type Att = { status: str, month: int, ref: int, awaiting: bool, reply: str }

  // The paid 'topup' ledger row stored under a charge ref (the ledger's
  // ref is unique: INSERT OR IGNORE). paid = false: no row for that ref.
  // month is the UTC month of the attempt's created_at (the row is dated
  // then). Charge refs are 1..5 (PI id of token t is t).
  type Row = { paid: bool, tok: int, month: int }

  // The subscriptions row (only the fields that matter here).
  // pastDueFor is model bookkeeping: the period of the invoice whose
  // failure put the plan past_due.
  type Sub = { status: str, ref: int, period: int, used: bool,
               pastDueSet: bool, graceExpired: bool, pastDueFor: int }

  type Db = {
    att: int -> Att,
    ledger: int -> Row,     // charge ref -> paid topup ledger row
    credited: int,          // total top-up micros ever granted
    failedLines: int -> bool, // token -> has a "<key>-failed" ledger line
    refunds: int -> bool,     // charge ref -> owed back (fixed model only)
    processed: Evs,         // processed_events (an event is its own id)
    cardError: bool,
    month: int,             // current UTC month
    sub: Sub
  }

  // A verified webhook as ParseWebhook hands it to applyEvent.
  // kind: pi_succeeded | pi_failed | pi_canceled  (tok = attempt, ref = PI id)
  //       invoice_paid | invoice_failed | sub_ended (ref = subscription ref)
  // The processor emits each distinct event once, so its content is its id
  // (Stripe event id). This keeps the event universe finite for Apalache;
  // a second failed retry of the same invoice is folded into the first.
  type Ev = { kind: str, tok: int, ref: int, period: int }

  // A set of events, split by kind, as membership maps over the finite
  // universe (fixed-size state: much cheaper for Apalache than growing sets
  // of records): PaymentIntent events by token, invoice events by
  // invCode(ref, period), ended events by subscription ref.
  type Evs = { piSucc: int -> bool, piFail: int -> bool, piCancel: int -> bool,
               invPaid: int -> bool, invFailed: int -> bool, subEnded: int -> bool }

  // The processor's view of a subscription: open, latest paid period, ended.
  type PSub = { open: bool, paid: int, ended: bool }

  type Proc = { pi: int -> str, subs: int -> PSub, sent: Evs }

  // History (ghost) variables for the monotonicity invariants.
  type Ghost = {
    everSucceeded: int -> bool,
    everFailed: int -> bool,
    everRefund: int -> bool,
    succRef: int -> int,      // charge_ref when the attempt first became succeeded
    endedRefs: int -> bool,   // subscription refs ever seen ended locally
    periodRef: int,
    periodHigh: int         // highest period held under periodRef
  }

  // ---------------------------------------------------------------- state

  var db: Db
  var proc: Proc
  var ghost: Ghost
  var lastResult: str       // what the last action reported (for tests)

  pure val noToks: int -> bool = TOKENS.mapBy(t => false)
  pure val noRefs: int -> bool = REFS.mapBy(r => false)
  pure val noEvs: Evs = { piSucc: noToks, piFail: noToks, piCancel: noToks,
                          invPaid: INV_CODES.mapBy(c => false), invFailed: INV_CODES.mapBy(c => false),
                          subEnded: noRefs }

  pure def invCode(r: int, p: int): int = r * 10 + p

  pure def hasEv(E: Evs, ev: Ev): bool =
    if (ev.kind == "pi_succeeded") E.piSucc.get(ev.tok)
    else if (ev.kind == "pi_failed") E.piFail.get(ev.tok)
    else if (ev.kind == "pi_canceled") E.piCancel.get(ev.tok)
    else if (ev.kind == "invoice_paid") E.invPaid.get(invCode(ev.ref, ev.period))
    else if (ev.kind == "invoice_failed") E.invFailed.get(invCode(ev.ref, ev.period))
    else E.subEnded.get(ev.ref)

  pure def addEv(E: Evs, ev: Ev): Evs =
    if (ev.kind == "pi_succeeded") { ...E, piSucc: E.piSucc.set(ev.tok, true) }
    else if (ev.kind == "pi_failed") { ...E, piFail: E.piFail.set(ev.tok, true) }
    else if (ev.kind == "pi_canceled") { ...E, piCancel: E.piCancel.set(ev.tok, true) }
    else if (ev.kind == "invoice_paid") { ...E, invPaid: E.invPaid.set(invCode(ev.ref, ev.period), true) }
    else if (ev.kind == "invoice_failed") { ...E, invFailed: E.invFailed.set(invCode(ev.ref, ev.period), true) }
    else { ...E, subEnded: E.subEnded.set(ev.ref, true) }

  pure def piEv(kind: str, t: int): Ev = { kind: kind, tok: t, ref: t, period: 0 }
  pure def invEv(paid: bool, r: int, p: int): Ev =
    { kind: if (paid) "invoice_paid" else "invoice_failed", tok: 0, ref: r, period: p }
  pure def endEv(r: int): Ev = { kind: "sub_ended", tok: 0, ref: r, period: 0 }

  pure val noAtt: Att = { status: "none", month: 0, ref: 0, awaiting: false, reply: "none" }
  pure val noSub: Sub = { status: "none", ref: 0, period: 0, used: false,
                          pastDueSet: false, graceExpired: false, pastDueFor: 0 }

  pure val initDb: Db = {
    att: TOKENS.mapBy(t => noAtt),
    ledger: TOKENS.mapBy(r => { paid: false, tok: 0, month: 0 }),
    credited: 0,
    failedLines: noToks,
    refunds: noToks,
    processed: noEvs,
    cardError: false,
    month: 0,
    sub: noSub
  }
  pure val initProc: Proc = {
    pi: TOKENS.mapBy(t => "unsent"),
    subs: REFS.mapBy(r => { open: false, paid: 0, ended: false }),
    sent: noEvs
  }
  pure val initGhost: Ghost = {
    everSucceeded: noToks, everFailed: noToks, everRefund: noToks,
    succRef: TOKENS.mapBy(t => 0),
    endedRefs: noRefs, periodRef: 0, periodHigh: 0
  }

  // ---------------------------------------------------------------- store logic

  // app.HasAccess: trialing/active, or past_due within the grace period.
  pure def hasAccess(s: Sub): bool =
    s.status == "trialing" or s.status == "active" or (s.status == "past_due" and not(s.graceExpired))

  // Counting by fold over the fixed token/ref set (cheaper for Apalache
  // than the cardinality of a filtered set).
  pure def count(S: Set[int], p: (int) => bool): int =
    S.fold(0, (acc, x) => if (p(x)) acc + 1 else acc)

  // store.MonthTopups: paid topup ledger rows dated in month m plus pending
  // attempts created in month m.
  pure def capCount(d: Db, m: int): int =
    count(TOKENS, r => d.ledger.get(r).paid and d.ledger.get(r).month == m)
      + count(TOKENS, t => d.att.get(t).status == "pending" and d.att.get(t).month == m)

  // store.ReserveTopup for a token with no row yet (eligibility + cap under
  // the write lock). Returns the new db and the reported result.
  pure def reserve(d: Db, t: int): (Db, str) =
    if (not(hasAccess(d.sub))) (d, "plan_ended")
    else if (d.sub.ref == 0) (d, "needs_card")   // a no-card trial: nothing to charge
    else if (capCount(d, d.month) >= TOPUP_CAP) (d, "cap")
    else ({ ...d, att: d.att.set(t, { status: "pending", month: d.month, ref: 0, awaiting: true, reply: "none" }) },
          "reserved")

  // store.NoteTopupCharge: record the PI id on a pending attempt with none.
  pure def noteCharge(d: Db, t: int, r: int): Db =
    val a = d.att.get(t)
    if (r != 0 and a.status == "pending" and a.ref == 0) { ...d, att: d.att.set(t, { ...a, ref: r }) }
    else d

  // store.FailTopup: only from pending; charge_ref recorded only if empty;
  // a reason sets card_error and books the "-failed" ledger line.
  pure def failTopup(d: Db, t: int, r: int, reason: bool): Db =
    val a = d.att.get(t)
    if (a.status != "pending") d
    else {
      ...d,
      att: d.att.set(t, { ...a, status: "failed", ref: if (a.ref == 0) r else a.ref }),
      failedLines: if (reason) d.failedLines.set(t, true) else d.failedLines,
      cardError: if (reason) true else d.cardError
    }

  // store.SucceedTopup. Legacy (fixed = false): from pending or failed, no
  // cap re-check. Fixed: a failed attempt is re-checked against its month's
  // cap; with no free slot it becomes refund_due (no credit, ref kept).
  pure def succeedTopup(d: Db, t: int, r: int, fixed: bool): Db =
    val a = d.att.get(t)
    val cr = if (r != 0) r else a.ref
    if (not(a.status == "pending" or a.status == "failed")) d   // succeeded/refund_due: absorbing
    else if (cr == 0) d                                         // ErrNoChargeRef, nothing changed
    else if (fixed and a.status == "failed" and capCount(d, a.month) >= TOPUP_CAP)
      {
        ...d,
        att: d.att.set(t, { ...a, status: "refund_due", ref: cr }),
        failedLines: d.failedLines.set(t, false),
        refunds: d.refunds.set(cr, true)
      }
    else {
      val d1 = { ...d,
                 att: d.att.set(t, { ...a, status: "succeeded", ref: cr }),
                 failedLines: d.failedLines.set(t, false) }
      // INSERT OR IGNORE keyed by the charge ref: credit only if inserted.
      if (d1.ledger.get(cr).paid) d1
      else { ...d1,
             ledger: d1.ledger.set(cr, { paid: true, tok: t, month: a.month }),
             credited: d1.credited + TOPUP_CREDIT,
             cardError: false }
    }

  // app.finishCharge: the second short transaction after Charge answered.
  pure def finishCharge(d0: Db, t: int, fixed: bool): Db =
    val d = { ...d0, att: d0.att.set(t, { ...d0.att.get(t), awaiting: false }) }
    val rep = d0.att.get(t).reply
    if (rep == "succeeded") succeedTopup(d, t, t, fixed)
    else if (rep == "rpm") failTopup(d, t, t, true)              // DeclineError
    else if (rep == "canceled") failTopup(d, t, t, false)        // CanceledError
    else if (rep == "processing" or rep == "requires_action" or rep == "error_ref") noteCharge(d, t, t)
    else d                                                       // indeterminate, no ref

  // app.reconcile on a pending attempt nobody is charging: FindCharge maps
  // the PI's current status (stripeIntent.result).
  pure def reconcileT(d: Db, pi: int -> str, t: int, fixed: bool): Db =
    val s = pi.get(t)
    if (s == "succeeded") succeedTopup(d, t, t, fixed)                       // ChargeSucceeded
    else if (s == "rpm") failTopup(d, t, t, true)                            // ChargeFailed + reason
    else if (s == "canceled") failTopup(d, t, t, false)                      // ChargeFailed, released
    else if (s == "processing" or s == "requires_action") noteCharge(d, t, t) // ChargePending
    else d                                                                   // ChargeNotFound: stays pending

  // The reconciliation answer FindCharge gives for a PI status.
  pure def lookupResult(s: str): str =
    if (s == "succeeded") "ChargeSucceeded"
    else if (s == "rpm" or s == "canceled") "ChargeFailed"
    else if (s == "processing" or s == "requires_action") "ChargePending"
    else "ChargeNotFound"

  pure def isTopupEvent(ev: Ev): bool =
    ev.kind == "pi_succeeded" or ev.kind == "pi_failed" or ev.kind == "pi_canceled"

  // app.applyTopupEvent: TopupByCharge (by charge_ref or idem key), then
  // MarkEvent dedupe, then SucceedTopup / FailTopup.
  pure def applyTopupEvent(d: Db, ev: Ev, fixed: bool): (Db, str) =
    if (d.att.get(ev.tok).status == "none") (d, "unknown_attempt")   // not marked
    else if (hasEv(d.processed, ev)) (d, "duplicate")
    else {
      val dm = { ...d, processed: addEv(d.processed, ev) }
      if (ev.kind == "pi_succeeded") (succeedTopup(dm, ev.tok, ev.ref, fixed), "applied")
      else if (ev.kind == "pi_failed") (failTopup(dm, ev.tok, ev.ref, true), "applied")
      else (failTopup(dm, ev.tok, ev.ref, false), "applied")
    }

  // app.applyEvent for subscription events, current code: no ordering or
  // terminal checks at all.
  pure def subEventLegacy(s: Sub, ev: Ev): Sub =
    if (ev.kind == "invoice_paid")
      { ...s, status: "active", period: ev.period, used: false,
        pastDueSet: false, graceExpired: false }
    else if (ev.kind == "invoice_failed")
      { ...s, status: "past_due", pastDueFor: ev.period,
        pastDueSet: true,
        graceExpired: if (s.pastDueSet) s.graceExpired else false }
    else { ...s, status: "ended" }

  // The proposed fix.
  pure def subEventFixed(s: Sub, ev: Ev): Sub =
    if (s.status == "ended") s                                   // terminal until Reopen
    else if (ev.kind == "invoice_paid")
      if (ev.period < s.period) s                                // stale, older period
      else if (ev.period == s.period)
        { ...s, status: "active", pastDueSet: false, graceExpired: false }   // no free reset
      else
        { ...s, status: "active", period: ev.period, used: false,
          pastDueSet: false, graceExpired: false }
    else if (ev.kind == "invoice_failed")
      if (ev.period <= s.period) s                               // that invoice is not the open one
      else { ...s, status: "past_due", pastDueFor: ev.period,
             pastDueSet: true,
             graceExpired: if (s.pastDueSet) s.graceExpired else false }
    else { ...s, status: "ended" }

  pure def applySubEvent(d: Db, ev: Ev, fixed: bool): (Db, str) =
    if (d.sub.status == "none" or ev.ref != d.sub.ref) (d, "unknown_subscription")  // SubscriptionByRef: not found, not marked
    else if (hasEv(d.processed, ev)) (d, "duplicate")
    else {
      val ns = if (fixed) subEventFixed(d.sub, ev) else subEventLegacy(d.sub, ev)
      ({ ...d, processed: addEv(d.processed, ev), sub: ns,
         cardError: if (ev.kind == "invoice_paid" and ns.status == "active") false
                    else if (ev.kind == "invoice_failed" and ns.status == "past_due") true
                    else d.cardError },
       "applied")
    }

  pure def applyEvent(d: Db, ev: Ev, fixed: bool): (Db, str) =
    if (isTopupEvent(ev)) applyTopupEvent(d, ev, fixed) else applySubEvent(d, ev, fixed)

  // ---------------------------------------------------------------- processor logic

  pure def emit(p: Proc, kind: str, tok: int, ref: int, period: int): Proc =
    { ...p, sent: addEv(p.sent, { kind: kind, tok: tok, ref: ref, period: period }) }

  // The webhook a PI status change produces (ParseWebhook only passes these on).
  pure def emitPi(p: Proc, t: int, s: str): Proc =
    if (s == "succeeded") emit(p, "pi_succeeded", t, t, 0)
    else if (s == "rpm") emit(p, "pi_failed", t, t, 0)
    else if (s == "canceled") emit(p, "pi_canceled", t, t, 0)
    else p

  // PaymentIntent lifecycle at the processor. "none" -> a recovered request
  // creating the charge late. rpm -> succeeded is a later confirm of the
  // same intent (the reason the code "corrects" failed attempts).
  pure def canAdvance(src: str, dst: str): bool =
    if (src == "none") Set("processing", "succeeded", "rpm").contains(dst)
    else if (src == "processing") Set("requires_action", "succeeded", "rpm", "canceled").contains(dst)
    else if (src == "requires_action") Set("succeeded", "rpm", "canceled").contains(dst)
    else if (src == "rpm") Set("succeeded", "canceled").contains(dst)
    else false

  pure def piAfterCall(o: str): str =
    if (o == "error_ref" or o == "lost_created") "processing"
    else if (o == "lost_none") "none"
    else o

  pure def replyOf(o: str): str =
    if (o == "lost_created" or o == "lost_none") "error_noref" else o

  // ---------------------------------------------------------------- ghosts

  pure def updGhost(g: Ghost, d: Db): Ghost =
    {
      everSucceeded: TOKENS.mapBy(t => g.everSucceeded.get(t) or d.att.get(t).status == "succeeded"),
      everFailed: TOKENS.mapBy(t => g.everFailed.get(t) or d.att.get(t).status == "failed"),
      everRefund: TOKENS.mapBy(t => g.everRefund.get(t) or d.att.get(t).status == "refund_due"),
      succRef: TOKENS.mapBy(t => if (d.att.get(t).status == "succeeded" and not(g.everSucceeded.get(t))) d.att.get(t).ref
                                 else g.succRef.get(t)),
      endedRefs: REFS.mapBy(r => g.endedRefs.get(r) or (d.sub.status == "ended" and d.sub.ref == r)),
      periodRef: d.sub.ref,
      periodHigh: if (d.sub.ref != g.periodRef) d.sub.period
                  else if (d.sub.period > g.periodHigh) d.sub.period else g.periodHigh
    }

  // ---------------------------------------------------------------- actions

  action commit(nd: Db, np: Proc, r: str): bool = all {
    db' = nd,
    proc' = np,
    ghost' = updGhost(ghost, nd),
    lastResult' = r,
  }

  action init = all {
    db' = initDb,
    proc' = initProc,
    ghost' = initGhost,
    lastResult' = "init",
  }

  // app.StartTrial for a learner with no plan: trialing, subscription ref 1.
  action StartTrial = all {
    db.sub.status == "none",
    commit({ ...db, sub: { ...noSub, status: "trialing", ref: 1 } },
           { ...proc, subs: proc.subs.set(1, { open: true, paid: 0, ended: false }) },
           "trial_started"),
  }

  // Slice 36: sign-up starts the no-card trial: trialing with no processor
  // subscription (ref 0). Nothing is sent to the processor.
  action StartNoCardTrial = all {
    db.sub.status == "none",
    commit({ ...db, sub: { ...noSub, status: "trialing", ref: 0 } }, proc, "nocard_trial_started"),
  }

  // Slice 36: a card added inside the no-card trial (app.addCardToTrial):
  // processor subscription 1, still trialing until the same end.
  action AddCard = all {
    db.sub.status == "trialing", db.sub.ref == 0,
    commit({ ...db, sub: { ...db.sub, ref: 1 } },
           { ...proc, subs: proc.subs.set(1, { open: true, paid: 0, ended: false }) },
           "card_added"),
  }

  // Slice 36 pass 2: a card confirmed on "charged today" terms in the
  // no-card trial's last hour (app.StartTrial -> store.StartPaidFromNoCard):
  // the processor takes the card first, then the trial row becomes the
  // paid plan in one write: active on processor subscription 1. A declined
  // card is a stutter here: the row is only written after the processor
  // succeeded, so the trial is exactly as it was.
  action UpgradeNow = all {
    db.sub.status == "trialing", db.sub.ref == 0,
    commit({ ...db, sub: { ...noSub, status: "active", ref: 1 } },
           { ...proc, subs: proc.subs.set(1, { open: true, paid: 0, ended: false }) },
           "upgraded_now"),
  }

  // Slice 36: the no-card trial reached its period_end (Plan /
  // EndExpiredNoCardTrials): ended, with no processor involved.
  action TrialExpire = all {
    db.sub.status == "trialing", db.sub.ref == 0,
    commit({ ...db, sub: { ...db.sub, status: "ended" } }, proc, "trial_expired"),
  }

  // app.StartTrial for an ended plan (OpenPlan replaces only an ended row):
  // a NEW subscription ref, charged at once, active, period 0 of that ref.
  action Reopen = all {
    db.sub.status == "ended",
    db.sub.ref < MAX_REF,
    val nr = db.sub.ref + 1
    commit({ ...db, sub: { ...noSub, status: "active", ref: nr } },
           { ...proc, subs: proc.subs.set(nr, { open: true, paid: 0, ended: false }) },
           "reopened"),
  }

  // A top-up form submission: TopUpResult -> topUpClaimed -> ReserveTopup.
  // A resubmitted token returns the existing attempt (settleExisting): a
  // pending one nobody is charging is reconciled, never charged again.
  action Reserve(t: int): bool = all {
    db.att.get(t).status == "none",
    val res = reserve(db, t)
    commit(res._1, proc, res._2),
  }

  action Submit(t: int, fixed: bool): bool = {
    val a = db.att.get(t)
    if (a.status == "none") {
      Reserve(t)
    } else if (a.status == "pending" and not(a.awaiting)) {
      commit(reconcileT(db, proc.pi, t, fixed), proc, "existing_reconciled")
    } else {
      commit(db, proc, "existing")
    }
  }

  // Pay.Charge with the attempt's idempotency key, outside any transaction.
  action ChargeCall(t: int, o: str): bool = all {
    db.att.get(t).awaiting,
    proc.pi.get(t) == "unsent",
    val s = piAfterCall(o)
    commit({ ...db, att: db.att.set(t, { ...db.att.get(t), reply: replyOf(o) }) },
           emitPi({ ...proc, pi: proc.pi.set(t, s) }, t, s),
           "charged"),
  }

  // finishCharge processes the reply (it may run after webhooks or after
  // other attempts settled).
  action SyncReply(t: int, fixed: bool): bool = all {
    db.att.get(t).awaiting,
    proc.pi.get(t) != "unsent",
    commit(finishCharge(db, t, fixed), proc, "reply"),
  }

  // The process dies mid-charge: the in-memory claim is gone, the reply is
  // never processed; the attempt stays pending for reconciliation.
  action Crash(t: int): bool = all {
    db.att.get(t).awaiting,
    commit({ ...db, att: db.att.set(t, { ...db.att.get(t), awaiting: false }) }, proc, "crashed"),
  }

  // ReconcileTopUps / ReconcileAllTopUps for one pending, unclaimed attempt.
  action Reconcile(t: int, fixed: bool): bool = all {
    db.att.get(t).status == "pending",
    not(db.att.get(t).awaiting),
    commit(reconcileT(db, proc.pi, t, fixed), proc, lookupResult(proc.pi.get(t))),
  }

  // The processor moves a PaymentIntent on (and emits its webhook).
  action ProcAdvance(t: int, dst: str): bool = all {
    canAdvance(proc.pi.get(t), dst),
    commit(db, emitPi({ ...proc, pi: proc.pi.set(t, dst) }, t, dst), "pi_advanced"),
  }

  // The processor bills the next period of subscription r.
  action ProcInvoice(r: int, paid: bool): bool = all {
    proc.subs.get(r).open,
    not(proc.subs.get(r).ended),
    proc.subs.get(r).paid < MAX_PERIOD,
    val ps = proc.subs.get(r)
    if (paid)
      commit(db, emit({ ...proc, subs: proc.subs.set(r, { ...ps, paid: ps.paid + 1 }) },
                      "invoice_paid", 0, r, ps.paid + 1), "invoice_paid")
    else
      commit(db, emit(proc, "invoice_failed", 0, r, ps.paid + 1), "invoice_failed"),
  }

  // The processor ends subscription r (customer.subscription.deleted).
  action ProcEnd(r: int): bool = all {
    proc.subs.get(r).open,
    not(proc.subs.get(r).ended),
    commit(db, emit({ ...proc, subs: proc.subs.set(r, { ...proc.subs.get(r), ended: true }) },
                    "sub_ended", 0, r, 0), "sub_ended"),
  }

  // A verified webhook delivery (ApplyEvent). Any event ever emitted may be
  // delivered at any time, any number of times.
  action Deliver(ev: Ev, fixed: bool): bool = {
    val res = applyEvent(db, ev, fixed)
    commit(res._1, proc, res._2)
  }

  // Deliver a sent event (any sent event, any number of times, any order).
  action DeliverSent(ev: Ev, fixed: bool): bool = all {
    hasEv(proc.sent, ev),
    Deliver(ev, fixed),
  }

  action MonthRollover = all {
    db.month < MAX_MONTH,
    commit({ ...db, month: db.month + 1 }, proc, "month_rollover"),
  }

  action GraceExpires = all {
    db.sub.status == "past_due",
    not(db.sub.graceExpired),
    commit({ ...db, sub: { ...db.sub, graceExpired: true } }, proc, "grace_expired"),
  }

  // Spend from the period allowance (store.Spend), abstracted to a flag.
  action UseAllowance = all {
    hasAccess(db.sub),
    not(db.sub.used),
    commit({ ...db, sub: { ...db.sub, used: true } }, proc, "used_allowance"),
  }

  action stepWith(fixed: bool): bool = any {
    StartTrial,
    StartNoCardTrial,
    AddCard,
    UpgradeNow,
    TrialExpire,
    Reopen,
    // A resubmitted token is settleExisting -> reconcile (= Reconcile) or a no-op.
    nondet t = oneOf(TOKENS) Reserve(t),
    nondet t = oneOf(TOKENS) nondet o = oneOf(OUTCOMES) ChargeCall(t, o),
    nondet t = oneOf(TOKENS) SyncReply(t, fixed),
    nondet t = oneOf(TOKENS) Crash(t),
    nondet t = oneOf(TOKENS) Reconcile(t, fixed),
    nondet t = oneOf(TOKENS) nondet s = oneOf(PI_TARGETS) ProcAdvance(t, s),
    nondet r = oneOf(REFS) nondet paid = oneOf(Set(true, false)) ProcInvoice(r, paid),
    nondet r = oneOf(REFS) ProcEnd(r),
    nondet t = oneOf(TOKENS) nondet k = oneOf(PI_EVENT_KINDS) DeliverSent(piEv(k, t), fixed),
    nondet r = oneOf(REFS) nondet p = oneOf(PERIODS) nondet paid = oneOf(Set(true, false))
      DeliverSent(invEv(paid, r, p), fixed),
    nondet r = oneOf(REFS) DeliverSent(endEv(r), fixed),
    MonthRollover,
    GraceExpires,
    UseAllowance,
  }

  // The proposed fix.
  action step = stepWith(true)

  // The current code.
  action step_legacy = stepWith(false)

  // ---------------------------------------------------------------- invariants
  //
  // Every invariant is a pure predicate over (db, proc, ghost) so the same
  // definition serves the state invariants below and the inductive-step
  // check (init_ind) at the end of the module.

  pure def st(d: Db, t: int): str = d.att.get(t).status
  pure def nSucc(d: Db): int = count(TOKENS, t => st(d, t) == "succeeded")

  // Credit == succeeded * TOPUP_CREDIT, and paid top-up ledger rows are
  // 1-to-1 with succeeded attempts (each succeeded attempt owns the row
  // under its charge_ref; every row belongs to a succeeded attempt with
  // that charge_ref and month; equal counts).
  pure def noDoubleCredit(d: Db): bool = and {
    d.credited == nSucc(d) * TOPUP_CREDIT,
    count(TOKENS, r => d.ledger.get(r).paid) == nSucc(d),
    TOKENS.forall(t => st(d, t) == "succeeded" implies
      (TOKENS.contains(d.att.get(t).ref) and d.ledger.get(d.att.get(t).ref).paid
       and d.ledger.get(d.att.get(t).ref).tok == t)),
    TOKENS.forall(r => d.ledger.get(r).paid implies {
      val row = d.ledger.get(r)
      TOKENS.contains(row.tok) and st(d, row.tok) == "succeeded"
        and d.att.get(row.tok).ref == r and d.att.get(row.tok).month == row.month
    }),
  }

  // For every UTC month: pending + succeeded attempts created in it <= 4.
  pure def monthlyCap(d: Db): bool =
    MONTHS.forall(m =>
      count(TOKENS, t => (st(d, t) == "pending" or st(d, t) == "succeeded") and d.att.get(t).month == m)
        <= TOPUP_CAP)

  // Terminal states never regress. Nuance: a Failed attempt MAY later become
  // Succeeded (legacy and fixed: a late authoritative success corrects it)
  // or refund_due (fixed only); it never returns to Pending. Succeeded and
  // refund_due never change, nor does a succeeded attempt's charge_ref. A
  // subscription ref that reached Ended stays Ended; only Reopen (a NEW
  // ref) leaves it.
  pure def terminalMonotonic(d: Db, g: Ghost): bool = and {
    TOKENS.forall(t => g.everSucceeded.get(t) implies
      (st(d, t) == "succeeded" and d.att.get(t).ref == g.succRef.get(t))),
    TOKENS.forall(t => g.everRefund.get(t) implies st(d, t) == "refund_due"),
    TOKENS.forall(t => g.everFailed.get(t) implies
      (st(d, t) == "failed" or st(d, t) == "succeeded" or st(d, t) == "refund_due")),
    REFS.forall(r => (g.endedRefs.get(r) and d.sub.ref == r) implies d.sub.status == "ended"),
  }

  pure def succeededHasChargeRef(d: Db): bool =
    TOKENS.forall(t => (st(d, t) == "succeeded" or st(d, t) == "refund_due") implies d.att.get(t).ref != 0)

  // Refund accounting (fixed): every refund_due attempt is owed back exactly
  // once (refunds is keyed by its charge ref) and is not credited; nothing
  // else is owed.
  pure def refundAccounting(d: Db): bool = and {
    TOKENS.forall(t => st(d, t) == "refund_due" implies
      (TOKENS.contains(d.att.get(t).ref) and d.refunds.get(d.att.get(t).ref)
       and not(d.ledger.get(d.att.get(t).ref).paid))),
    TOKENS.forall(r => d.refunds.get(r) implies
      TOKENS.exists(t => st(d, t) == "refund_due" and d.att.get(t).ref == r)),
  }

  // Credit (or a refund) only for money the processor actually took.
  pure def creditBackedByCharge(d: Db, p: Proc): bool =
    TOKENS.forall(t => (st(d, t) == "succeeded" or st(d, t) == "refund_due") implies p.pi.get(t) == "succeeded")

  // A "card failed" history line only for an attempt that is failed now.
  pure def failedLineConsistent(d: Db): bool =
    TOKENS.forall(t => d.failedLines.get(t) implies st(d, t) == "failed")

  // The subscription period never moves backwards under one ref (a stale
  // InvoicePaid would also reset period_used: free allowance).
  pure def periodMonotonic(d: Db, g: Ghost): bool =
    d.sub.ref == g.periodRef and d.sub.period == g.periodHigh

  // past_due only for an invoice newer than the current paid period (a stale
  // InvoiceFailed for an invoice that was paid must not flip active -> past_due).
  pure def pastDueIsCurrent(d: Db): bool =
    d.sub.status == "past_due" implies d.sub.pastDueFor > d.sub.period

  // Slice 36: while the plan has no processor subscription (ref 0: none or
  // the no-card trial, live or ended), no top-up attempt exists, no charge
  // was ever sent, and no credit was ever granted.
  pure def noCardNeverCharged(d: Db, p: Proc): bool =
    d.sub.ref == 0 implies and {
      TOKENS.forall(t => st(d, t) == "none" and p.pi.get(t) == "unsent"),
      d.credited == 0,
      REFS.forall(r => not(p.subs.get(r).open)),
    }

  pure def allOf(d: Db, p: Proc, g: Ghost): bool = and {
    noCardNeverCharged(d, p),
    noDoubleCredit(d),
    monthlyCap(d),
    terminalMonotonic(d, g),
    succeededHasChargeRef(d),
    refundAccounting(d),
    creditBackedByCharge(d, p),
    failedLineConsistent(d),
    periodMonotonic(d, g),
    pastDueIsCurrent(d),
  }

  def status(t: int): str = st(db, t)
  val nSucceeded: int = nSucc(db)
  val nPaidRows: int = count(TOKENS, r => db.ledger.get(r).paid)

  val inv_no_double_credit: bool = noDoubleCredit(db)
  val inv_monthly_cap: bool = monthlyCap(db)
  val inv_terminal_monotonic: bool = terminalMonotonic(db, ghost)
  val inv_succeeded_has_charge_ref: bool = succeededHasChargeRef(db)
  val inv_refund_accounting: bool = refundAccounting(db)
  val inv_credit_backed_by_charge: bool = creditBackedByCharge(db, proc)
  val inv_failed_line_consistent: bool = failedLineConsistent(db)
  val inv_period_monotonic: bool = periodMonotonic(db, ghost)
  val inv_past_due_is_current: bool = pastDueIsCurrent(db)
  val inv_nocard_never_charged: bool = noCardNeverCharged(db, proc)
  val inv_all: bool = allOf(db, proc, ghost)

  // ---------------------------------------------------------------- test helpers

  // Deliver the event of this kind/ref/period, which must have been sent.
  // Topup events use tok = ref = token and period 0.
  action DeliverOne(kind: str, ref: int, period: int, fixed: bool): bool =
    DeliverSent({ kind: kind, tok: if (PI_EVENT_KINDS.contains(kind)) ref else 0, ref: ref, period: period }, fixed)

  // Reserve, charge successfully and process the synchronous reply.
  run payOne(t: int, fixed: bool): bool =
    Submit(t, fixed).then(ChargeCall(t, "succeeded")).then(SyncReply(t, fixed))

  // ---------------------------------------------------------------- tests

  run duplicateWebhooksTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "processing"))
      .then(SyncReply(1, true))
      .expect(status(1) == "pending" and db.att.get(1).ref == 1 and lastResult == "reply")
      .then(ProcAdvance(1, "succeeded"))
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT)
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(lastResult == "duplicate")
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .then(Submit(1, true))
      .expect(db.credited == TOPUP_CREDIT and nPaidRows == 1 and inv_all)

  run outOfOrderFailedThenSucceededTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "requires_action"))
      .then(SyncReply(1, true))
      .then(ProcAdvance(1, "rpm"))
      .then(ProcAdvance(1, "succeeded"))
      .then(DeliverOne("pi_failed", 1, 0, true))
      .expect(status(1) == "failed" and db.failedLines.get(1) and db.cardError)
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT
              and not(db.failedLines.get(1)) and not(db.cardError) and inv_all)
      .then(DeliverOne("pi_failed", 1, 0, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and inv_all)

  run outOfOrderSucceededThenFailedTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "processing"))
      .then(ProcAdvance(1, "rpm"))
      .then(ProcAdvance(1, "succeeded"))
      // both webhooks before the synchronous reply
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .then(DeliverOne("pi_failed", 1, 0, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and db.failedLines == noToks)
      .then(SyncReply(1, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and nPaidRows == 1 and inv_all)

  run webhookBeforeSyncReplyTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "succeeded"))
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(status(1) == "succeeded" and db.att.get(1).awaiting)
      .then(SyncReply(1, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and inv_all)

  run capBoundaryAndRolloverTest =
    init
      .then(StartTrial)
      .then(payOne(1, true))
      .then(payOne(2, true))
      .then(payOne(3, true))
      // the 4th is still pending: it holds its slot
      .then(Submit(4, true))
      .expect(lastResult == "reserved")
      .then(Submit(5, true))
      .expect(lastResult == "cap" and status(5) == "none")
      .then(ChargeCall(4, "succeeded"))
      .then(SyncReply(4, true))
      .expect(nSucceeded == 4 and db.credited == 4 * TOPUP_CREDIT)
      .then(Submit(5, true))
      .expect(lastResult == "cap" and status(5) == "none")
      .then(MonthRollover)
      .then(payOne(5, true))
      .expect(status(5) == "succeeded" and db.att.get(5).month == 1
              and db.credited == 5 * TOPUP_CREDIT and inv_all)

  run failedAttemptFreesSlotTest =
    init
      .then(StartTrial)
      .then(payOne(1, true))
      .then(payOne(2, true))
      .then(payOne(3, true))
      .then(Submit(4, true))
      .then(ChargeCall(4, "rpm"))
      .then(SyncReply(4, true))
      .expect(status(4) == "failed" and db.failedLines.get(4))
      .then(payOne(5, true))
      .expect(status(5) == "succeeded" and nSucceeded == 4 and inv_all)

  run lateReconciliationTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "lost_none"))
      .then(SyncReply(1, true))
      .expect(status(1) == "pending" and db.att.get(1).ref == 0)
      .then(Reconcile(1, true))
      .expect(lastResult == "ChargeNotFound" and status(1) == "pending")   // ChargeNotFound
      .then(ProcAdvance(1, "processing"))                                   // the request is recovered
      .then(Reconcile(1, true))
      .expect(status(1) == "pending" and db.att.get(1).ref == 1)            // ChargePending keeps the PI id
      .then(ProcAdvance(1, "succeeded"))
      .then(Reconcile(1, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT)     // ChargeSucceeded
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(db.credited == TOPUP_CREDIT and nPaidRows == 1 and inv_all)

  run crashThenReconcileFailedTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "lost_created"))
      .then(Crash(1))
      .then(ProcAdvance(1, "canceled"))
      .then(Reconcile(1, true))
      .expect(status(1) == "failed" and db.att.get(1).ref == 1 and db.failedLines == noToks and inv_all)

  run resubmittedTokenTest =
    init
      .then(StartTrial)
      .then(Submit(1, true))
      .then(ChargeCall(1, "lost_created"))
      .then(Submit(1, true))
      .expect(lastResult == "existing")                         // in flight: waits, never a 2nd charge
      .then(SyncReply(1, true))
      .then(ProcAdvance(1, "succeeded"))
      .then(Submit(1, true))
      .expect(lastResult == "existing_reconciled" and status(1) == "succeeded")
      .then(Submit(1, true))
      .expect(lastResult == "existing" and db.credited == TOPUP_CREDIT and inv_all)

  run topupRefusedOnEndedPlanTest =
    init
      .then(StartTrial)
      .then(ProcEnd(1))
      .then(DeliverOne("sub_ended", 1, 0, true))
      .then(Submit(1, true))
      .expect(lastResult == "plan_ended" and status(1) == "none")

  run subscriptionOutOfOrderAndReopenTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, true))                       // invoice_paid period 1
      .then(ProcInvoice(1, false))                      // invoice_failed period 2
      .then(ProcInvoice(1, true))                       // retry paid, period 2
      .then(DeliverOne("invoice_paid", 1, 2, true))
      .expect(db.sub.status == "active" and db.sub.period == 2)
      .then(UseAllowance)
      .then(DeliverOne("invoice_paid", 1, 1, true))     // stale, older period: ignored
      .expect(db.sub.period == 2 and db.sub.used)
      .then(DeliverOne("invoice_failed", 1, 2, true))   // stale failure of a paid invoice: ignored
      .expect(db.sub.status == "active")
      .then(DeliverOne("invoice_paid", 1, 2, true))     // duplicate
      .expect(lastResult == "duplicate")
      .then(ProcEnd(1))
      .then(DeliverOne("sub_ended", 1, 0, true))
      .expect(db.sub.status == "ended" and inv_all)

  run subscriptionEndedThenReopenTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, true))
      .then(ProcInvoice(1, false))
      .then(ProcEnd(1))
      .then(DeliverOne("sub_ended", 1, 0, true))
      .then(DeliverOne("invoice_paid", 1, 1, true))     // late: plan stays ended
      .then(DeliverOne("invoice_failed", 1, 2, true))
      .expect(db.sub.status == "ended" and not(hasAccess(db.sub)) and inv_all)
      .then(Reopen)
      .expect(db.sub.status == "active" and db.sub.ref == 2 and inv_all)
      .then(DeliverOne("invoice_paid", 1, 1, true))     // old ref matches nothing
      .expect(lastResult == "unknown_subscription" and db.sub.status == "active" and db.sub.ref == 2)
      .then(ProcInvoice(2, true))
      .then(DeliverOne("invoice_paid", 2, 1, true))
      .expect(db.sub.period == 1 and inv_all)

  // ---- the current code's hazards, and the fix on the same schedule

  run capBypassSchedule(fixed: bool): bool =
    init
      .then(StartTrial)
      .then(Submit(1, fixed))
      .then(ChargeCall(1, "rpm"))
      .then(SyncReply(1, fixed))                       // A declined: slot freed
      .then(payOne(2, fixed))
      .then(payOne(3, fixed))
      .then(payOne(4, fixed))
      .then(payOne(5, fixed))                          // 4 paid this month
      .then(ProcAdvance(1, "succeeded"))               // A's intent later succeeds
      .then(DeliverOne("pi_succeeded", 1, 0, fixed))

  run legacyCapBypassTest =
    capBypassSchedule(false)
      .expect(nSucceeded == 5 and db.credited == 5 * TOPUP_CREDIT
              and not(inv_monthly_cap) and inv_no_double_credit)

  run fixedCapBypassRefundTest =
    capBypassSchedule(true)
      .expect(status(1) == "refund_due" and db.refunds == noToks.set(1, true) and nSucceeded == 4
              and db.credited == 4 * TOPUP_CREDIT and inv_all)
      .then(DeliverOne("pi_succeeded", 1, 0, true))
      .expect(status(1) == "refund_due" and db.refunds == noToks.set(1, true) and inv_all)

  run legacyEndedRegressionTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, true))
      .then(ProcEnd(1))
      .then(DeliverOne("sub_ended", 1, 0, false))
      .then(DeliverOne("invoice_paid", 1, 1, false))   // late invoice.paid
      .expect(db.sub.status == "active" and hasAccess(db.sub) and not(inv_terminal_monotonic))

  run legacyEndedToPastDueTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, false))
      .then(ProcEnd(1))
      .then(DeliverOne("sub_ended", 1, 0, false))
      .then(DeliverOne("invoice_failed", 1, 1, false)) // late payment_failed
      .expect(db.sub.status == "past_due" and hasAccess(db.sub) and not(inv_terminal_monotonic))
      .then(Submit(1, false))
      .expect(lastResult == "reserved")                // a top-up charged on an ended plan

  run legacyStalePeriodTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, true))
      .then(ProcInvoice(1, true))
      .then(DeliverOne("invoice_paid", 1, 2, false))
      .then(UseAllowance)
      .then(DeliverOne("invoice_paid", 1, 1, false))   // stale older period
      .expect(db.sub.period == 1 and not(db.sub.used) and not(inv_period_monotonic))

  run legacyStaleFailedTest =
    init
      .then(StartTrial)
      .then(ProcInvoice(1, false))
      .then(ProcInvoice(1, true))
      .then(DeliverOne("invoice_paid", 1, 1, false))
      .then(DeliverOne("invoice_failed", 1, 1, false)) // stale failure of the paid invoice
      .expect(db.sub.status == "past_due" and not(inv_past_due_is_current))

  // ---------------------------------------------------------------- inductive proof
  //
  // ind_inv = inv_all /\ strengthening lemmas. It is inductive for `step`:
  //   quint verify --invariant=ind_inv --max-steps=0              (init => ind_inv)
  //   quint verify --init=init_ind --invariant=ind_inv --max-steps=1
  // where init_ind is ANY state (over the finite typing ranges) satisfying
  // ind_inv. Together: inv_all holds in every reachable state of `step`, at
  // any depth (not just up to a bound).

  pure val ATT_STATUSES = Set("none", "pending", "succeeded", "failed", "refund_due")
  pure val REPLIES = Set("none", "succeeded", "processing", "requires_action", "rpm", "canceled",
                         "error_ref", "error_noref")
  pure val SUB_STATUSES = Set("none", "trialing", "active", "past_due", "ended")
  pure val BOOLS = Set(true, false)
  pure val TOK0 = 0.to(5)          // 0 or a token / charge ref
  pure val REF0 = 0.to(MAX_REF)    // 0 or a subscription ref
  pure val PER0 = 0.to(MAX_PERIOD)

  pure def lemmas(d: Db, p: Proc, g: Ghost): bool = and {
    // the PI (charge ref) of token t is t
    TOKENS.forall(t => d.att.get(t).ref == 0 or d.att.get(t).ref == t),
    // no row: nothing in flight, nothing charged
    TOKENS.forall(t => st(d, t) == "none" implies
      (d.att.get(t).ref == 0 and not(d.att.get(t).awaiting) and d.att.get(t).reply == "none"
       and p.pi.get(t) == "unsent")),
    // a charge exists only for a reserved attempt; the reply describes the PI
    TOKENS.forall(t => p.pi.get(t) != "unsent" implies st(d, t) != "none"),
    TOKENS.forall(t => d.att.get(t).awaiting implies st(d, t) != "none"),
    TOKENS.forall(t => d.att.get(t).reply == "succeeded" implies p.pi.get(t) == "succeeded"),
    // a pi_succeeded webhook is only sent for a succeeded PI
    TOKENS.forall(t => p.sent.piSucc.get(t) implies p.pi.get(t) == "succeeded"),
    // subscription refs
    d.sub.status == "none" implies (d.sub.ref == 0 and REFS.forall(r => not(g.endedRefs.get(r)))),
    d.sub.status != "none" implies (REFS.contains(d.sub.ref) or d.sub.ref == 0),
    (d.sub.ref == 0 and d.sub.status != "none") implies d.sub.status.in(Set("trialing", "ended")),
    REFS.forall(r => g.endedRefs.get(r) implies r <= d.sub.ref),
    d.month <= MAX_MONTH,
  }

  pure def indOf(d: Db, p: Proc, g: Ghost): bool = allOf(d, p, g) and lemmas(d, p, g)

  val ind_inv: bool = indOf(db, proc, ghost)

  // ---- slice 36: the no-card trial
  run nocardTopupRefusedTest =
    init
      .then(StartNoCardTrial)
      .then(Reserve(1))
      .expect(lastResult == "needs_card" and status(1) == "none" and proc.pi.get(1) == "unsent" and inv_all)
      .then(ChargeCall(1, "succeeded"))
      .fail()

  run nocardAddCardThenTopupTest =
    init
      .then(StartNoCardTrial)
      .then(AddCard)
      .expect(db.sub.status == "trialing" and db.sub.ref == 1)
      .then(payOne(1, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and inv_all)

  // pass 2: a confirmed upgrade in the trial's last hour is the paid plan
  // at once, and top-ups work on it.
  run nocardUpgradeNowThenTopupTest =
    init
      .then(StartNoCardTrial)
      .then(UpgradeNow)
      .expect(db.sub.status == "active" and db.sub.ref == 1 and inv_all)
      .then(payOne(1, true))
      .expect(status(1) == "succeeded" and db.credited == TOPUP_CREDIT and inv_all)

  run nocardExpiryThenReopenTest =
    init
      .then(StartNoCardTrial)
      .then(TrialExpire)
      .expect(db.sub.status == "ended" and db.sub.ref == 0)
      .then(Reserve(1))
      .expect(lastResult == "plan_ended" and inv_all)
      .then(Reopen)
      .expect(db.sub.status == "active" and db.sub.ref == 1 and inv_all)
      .then(payOne(2, true))
      .expect(status(2) == "succeeded" and inv_all)

  run nocardEventsNeverApplyTest =
    init
      .then(StartNoCardTrial)
      .then(ProcInvoice(1, true))
      .fail()

  // An arbitrary state over the typing ranges that satisfies ind_inv.
  action init_ind = {
    nondet aSt = oneOf(setOfMaps(TOKENS, ATT_STATUSES))
    nondet aMo = oneOf(setOfMaps(TOKENS, MONTHS))
    nondet aRef = oneOf(setOfMaps(TOKENS, TOK0))
    nondet aAw = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet aRep = oneOf(setOfMaps(TOKENS, REPLIES))
    nondet lPaid = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet lTok = oneOf(setOfMaps(TOKENS, TOK0))
    nondet lMo = oneOf(setOfMaps(TOKENS, MONTHS))
    nondet nCred = oneOf(0.to(5))
    nondet fLines = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet refs = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet dS = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet dF = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet dC = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet dIP = oneOf(setOfMaps(INV_CODES, BOOLS))
    nondet dIF = oneOf(setOfMaps(INV_CODES, BOOLS))
    nondet dE = oneOf(setOfMaps(REFS, BOOLS))
    nondet cErr = oneOf(BOOLS)
    nondet mon = oneOf(MONTHS)
    nondet sSt = oneOf(SUB_STATUSES)
    nondet sRef = oneOf(REF0)
    nondet sPer = oneOf(PER0)
    nondet sUsed = oneOf(BOOLS)
    nondet sPds = oneOf(BOOLS)
    nondet sGe = oneOf(BOOLS)
    nondet sPdf = oneOf(PER0)
    nondet piS = oneOf(setOfMaps(TOKENS, PI_STATUSES))
    nondet psOpen = oneOf(setOfMaps(REFS, BOOLS))
    nondet psPaid = oneOf(setOfMaps(REFS, PER0))
    nondet psEnded = oneOf(setOfMaps(REFS, BOOLS))
    nondet eS = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet eF = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet eC = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet eIP = oneOf(setOfMaps(INV_CODES, BOOLS))
    nondet eIF = oneOf(setOfMaps(INV_CODES, BOOLS))
    nondet eE = oneOf(setOfMaps(REFS, BOOLS))
    nondet gS = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet gF = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet gR = oneOf(setOfMaps(TOKENS, BOOLS))
    nondet gRef = oneOf(setOfMaps(TOKENS, TOK0))
    nondet gEnd = oneOf(setOfMaps(REFS, BOOLS))
    nondet gPRef = oneOf(REF0)
    nondet gPHigh = oneOf(PER0)
    val d: Db = {
      att: TOKENS.mapBy(t => { status: aSt.get(t), month: aMo.get(t), ref: aRef.get(t),
                               awaiting: aAw.get(t), reply: aRep.get(t) }),
      ledger: TOKENS.mapBy(r => { paid: lPaid.get(r), tok: lTok.get(r), month: lMo.get(r) }),
      credited: nCred * TOPUP_CREDIT,
      failedLines: fLines,
      refunds: refs,
      processed: { piSucc: dS, piFail: dF, piCancel: dC, invPaid: dIP, invFailed: dIF, subEnded: dE },
      cardError: cErr,
      month: mon,
      sub: { status: sSt, ref: sRef, period: sPer, used: sUsed,
             pastDueSet: sPds, graceExpired: sGe, pastDueFor: sPdf }
    }
    val p: Proc = {
      pi: piS,
      subs: REFS.mapBy(r => { open: psOpen.get(r), paid: psPaid.get(r), ended: psEnded.get(r) }),
      sent: { piSucc: eS, piFail: eF, piCancel: eC, invPaid: eIP, invFailed: eIF, subEnded: eE }
    }
    val g: Ghost = {
      everSucceeded: gS, everFailed: gF, everRefund: gR, succRef: gRef,
      endedRefs: gEnd, periodRef: gPRef, periodHigh: gPHigh
    }
    all {
      indOf(d, p, g),
      db' = d,
      proc' = p,
      ghost' = g,
      lastResult' = "induction",
    }
  }
}
