// -*- mode: Bluespec; -*-
//
// subscription_claim.qnt: a learner's confirmed card step (slice 36 pass 3).
// Opening a paid plan charges the card, so it must happen at most once
// however many requests, processes, lost replies, restarts and minutes
// there are.
//
// Faithful to:
//   internal/app/plan.go                   StartTrial, subscribe, recordSubscribed
//   internal/store/subscription_attempts.go ClaimSubscription,
//                                          FailSubscriptionAttempt, CommitSubscription
//   internal/billing/stripe.go             Subscribe, FindSubscription
//
// Two step relations:
//   step_legacy  the code before: no claim, and the idempotency key is the
//                minute the request started in. Two requests in different
//                minutes, or a retry after a lost reply, charge twice.
//   step         one pending attempt per learner, stored before the
//                processor is called; an attempt whose outcome is not known
//                is repeated under its own key while the processor
//                remembers it, and only looked up after that; the plan and
//                the attempt's end are one write.
//
// Abstractions: one learner, two processes (each runs one request for the
// learner at a time: App.subInFlight), idempotency keys are 1..4, the
// processor's answer to a key is fixed by its first request while the key
// is remembered. Forget (24 hours) needs no request still in flight, as a
// request is bounded by subscribeTimeout. Lookups are complete or fail.

module subscription_claim {

  pure val PROCS = Set(1, 2)
  pure val KEYS = 1.to(4)

  // What the processor did with a key. "none": never asked. "paid": a
  // subscription was opened and charged. "declined": refused, nothing done.
  type Outcome = str

  // A process: idle, or a request holding a key. "claimed": the processor
  // has not been asked. "replied": it answered (reply). "lookup": the key
  // is too old to repeat; only a lookup may follow.
  type Proc = { at: str, key: int, reply: Outcome }

  var plan: str                 // nocard | active (the subscriptions row)
  var planKey: int              // the key whose subscription the plan records
  var attempt: int -> str       // durable: none | pending | succeeded | failed
  var remembered: int -> bool   // the processor still knows the key
  var did: int -> Outcome       // what its first request did
  var charges: int              // paid subscriptions opened at the processor
  var ledger: int               // paid lines booked
  var proc: int -> Proc
  var minute: int               // legacy: the key a new request would use

  pure val idle: Proc = { at: "idle", key: 0, reply: "none" }

  action init = all {
    plan' = "nocard",
    planKey' = 0,
    attempt' = KEYS.mapBy(k => "none"),
    remembered' = KEYS.mapBy(k => false),
    did' = KEYS.mapBy(k => "none"),
    charges' = 0,
    ledger' = 0,
    proc' = PROCS.mapBy(p => idle),
    minute' = 1
  }

  action sameDb = all { plan' = plan, planKey' = planKey, attempt' = attempt, ledger' = ledger }
  action samePay = all { remembered' = remembered, did' = did, charges' = charges }

  def pendingKeys = KEYS.filter(k => attempt.get(k) == "pending")
  def freeKeys = KEYS.filter(k => attempt.get(k) == "none")

  // ---------------------------------------------------------------- step

  // ClaimSubscription: the pending attempt, else a new one under a new key.
  // A pending attempt whose key the processor forgot is only looked up.
  action Claim(p: int): bool = all {
    proc.get(p).at == "idle",
    plan == "nocard",
    if (pendingKeys.size() > 0) {
      nondet k = pendingKeys.oneOf()
      all {
        proc' = proc.set(p, { at: if (did.get(k) != "none" and not(remembered.get(k))) "lookup" else "claimed", key: k, reply: "none" }),
        sameDb
      }
    } else all {
      freeKeys.size() > 0,
      nondet k = freeKeys.oneOf()
      all {
        proc' = proc.set(p, { at: "claimed", key: k, reply: "none" }),
        attempt' = attempt.set(k, "pending"),
        plan' = plan, planKey' = planKey, ledger' = ledger
      }
    },
    samePay,
    minute' = minute
  }

  // The request reaches the processor: a key it remembers answers as it
  // first did; one it does not is run.
  action send(p: int, outcome: Outcome): bool = {
    val k = proc.get(p).key
    all {
      proc.get(p).at == "claimed",
      if (remembered.get(k)) all {
        proc' = proc.set(p, { ...proc.get(p), at: "replied", reply: did.get(k) }),
        samePay
      } else all {
        proc' = proc.set(p, { ...proc.get(p), at: "replied", reply: outcome }),
        remembered' = remembered.set(k, true),
        did' = did.set(k, outcome),
        charges' = if (outcome == "paid") charges + 1 else charges
      },
      sameDb,
      minute' = minute
    }
  }
  action Send(p: int): bool = {
    nondet outcome = Set("paid", "declined").oneOf()
    send(p, outcome)
  }

  // FindSubscription after the key is forgotten: what it did, or nothing.
  action Lookup(p: int): bool = {
    val k = proc.get(p).key
    all {
      proc.get(p).at == "lookup",
      proc' = proc.set(p, { ...proc.get(p), at: "replied", reply: if (did.get(k) == "paid") "paid" else "declined" }),
      samePay, sameDb,
      minute' = minute
    }
  }

  // The reply is lost, or the process restarts, at any point.
  action Lose(p: int): bool = all {
    proc.get(p).at != "idle",
    proc' = proc.set(p, idle),
    samePay, sameDb,
    minute' = minute
  }

  // FailSubscriptionAttempt, or CommitSubscription (plan, ledger line and
  // attempt in one write; nothing if another process settled it first).
  action Settle(p: int): bool = {
    val k = proc.get(p).key
    all {
      proc.get(p).at == "replied",
      if (attempt.get(k) != "pending") sameDb
      else if (proc.get(p).reply == "declined") all {
        attempt' = attempt.set(k, "failed"),
        plan' = plan, planKey' = planKey, ledger' = ledger
      } else all {
        attempt' = attempt.set(k, "succeeded"),
        plan' = "active", planKey' = k, ledger' = ledger + 1
      },
      proc' = proc.set(p, idle),
      samePay,
      minute' = minute
    }
  }

  // The processor forgets every key (24 hours): no request is in flight.
  action Forget = all {
    PROCS.forall(p => proc.get(p).at == "idle"),
    remembered' = KEYS.mapBy(k => false),
    did' = did, charges' = charges,
    sameDb,
    proc' = proc,
    minute' = minute
  }

  action step = {
    nondet p = PROCS.oneOf()
    any { Claim(p), Send(p), Lookup(p), Lose(p), Settle(p), Forget }
  }

  // ---------------------------------------------------------------- legacy

  action LegacyStart(p: int): bool = all {
    proc.get(p).at == "idle",
    plan == "nocard",
    proc' = proc.set(p, { at: "claimed", key: minute, reply: "none" }),
    samePay, sameDb,
    minute' = minute
  }
  action LegacySettle(p: int): bool = all {
    proc.get(p).at == "replied",
    if (proc.get(p).reply == "paid" and plan == "nocard") all {
      plan' = "active", planKey' = proc.get(p).key, ledger' = ledger + 1, attempt' = attempt
    } else sameDb,
    proc' = proc.set(p, idle),
    samePay,
    minute' = minute
  }
  action NextMinute = all {
    minute < 4,
    minute' = minute + 1,
    samePay, sameDb,
    proc' = proc
  }
  action step_legacy = {
    nondet p = PROCS.oneOf()
    any { LegacyStart(p), Send(p), Lose(p), LegacySettle(p), NextMinute }
  }

  // ---------------------------------------------------------------- invariants

  // The learner is charged at most once, ever.
  val inv_charged_once = charges <= 1
  // At most one attempt is pending (the partial unique index).
  val inv_one_pending = pendingKeys.size() <= 1
  // The plan, its one ledger line and its attempt move together, and the
  // plan records a subscription that was paid for.
  val inv_commit_atomic = and {
    (plan == "active") == (ledger == 1),
    ledger <= 1,
    (plan == "active") == KEYS.exists(k => attempt.get(k) == "succeeded"),
    plan == "active" implies (did.get(planKey) == "paid" and attempt.get(planKey) == "succeeded")
  }
  // Money taken is never forgotten: its attempt is pending or recorded.
  val inv_never_lost = KEYS.forall(k =>
    did.get(k) == "paid" implies (attempt.get(k) == "pending" or attempt.get(k) == "succeeded"))
  // A refusal never touches the no-card plan.
  val inv_decline_keeps_plan = charges == 0 implies plan == "nocard"

  val inv_all = and { inv_charged_once, inv_one_pending, inv_commit_atomic, inv_never_lost, inv_decline_keeps_plan }

  // ---------------------------------------------------------------- tests

  run overlappingRequestsTest =
    init.then(Claim(1)).then(send(1, "paid")).then(Claim(2)).then(send(2, "paid"))
      .then(Settle(1)).then(Settle(2))
      .expect(inv_all and charges == 1 and ledger == 1 and plan == "active")

  run lostPaidReplyTest =
    init.then(Claim(1)).then(send(1, "paid")).then(Lose(1))
      .expect(inv_all and plan == "nocard" and charges == 1)
      .then(Claim(1)).then(send(1, "paid")).then(Settle(1))
      .expect(inv_all and charges == 1 and ledger == 1 and plan == "active")

  run lostPaidReplyForgottenKeyTest =
    init.then(Claim(1)).then(send(1, "paid")).then(Lose(1)).then(Forget)
      .then(Claim(1)).expect(proc.get(1).at == "lookup")
      .then(Lookup(1)).then(Settle(1))
      .expect(inv_all and charges == 1 and ledger == 1 and plan == "active")

  run declineThenGoodCardTest =
    init.then(Claim(1)).then(send(1, "declined")).then(Settle(1))
      .expect(inv_all and plan == "nocard" and pendingKeys.size() == 0)
      .then(Claim(1)).expect(attempt.get(proc.get(1).key) == "pending" and did.get(proc.get(1).key) == "none")
      .then(send(1, "paid")).then(Settle(1))
      .expect(inv_all and charges == 1 and plan == "active")

  // The finding: two minutes, two keys, two charges.
  run legacyMinuteBoundaryTest =
    init.then(LegacyStart(1)).then(NextMinute).then(LegacyStart(2))
      .then(send(1, "paid")).then(send(2, "paid"))
      .expect(charges == 2 and not(inv_charged_once))

  run legacyLostReplyTest =
    init.then(LegacyStart(1)).then(send(1, "paid")).then(Lose(1)).then(NextMinute)
      .then(LegacyStart(1)).then(send(1, "paid"))
      .expect(charges == 2 and not(inv_charged_once))
}
