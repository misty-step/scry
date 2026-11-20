// -*- mode: Bluespec; -*-
//
// Allowance reservations: reserve -> (provider call) -> settle.
//
// Models one learner's subscription row and its reservations table, as
// implemented in
//   internal/store/reservation.go  Reserve / Settle / ReservedMicros
//   internal/store/billing.go      Spend, Subscription, OpenPlan
//   internal/app/plan.go           HasAccess (GraceDays), Plan, reserve/settle,
//                                  applyEvent (InvoicePaid / InvoiceFailed /
//                                  SubscriptionEnded), TopUp (topupEligible)
//   internal/app/study.go          Answer: settle only `if use.Model != ""`
//   internal/app/explain.go        Explain: reserve -> always settle
//   internal/app/learn.go          Learn/runDraft, Transcribe
//   internal/store/store.go        reservations(id, user_id, amount>0): no
//                                  created_at, no boot epoch
//   cmd/scry/main.go, app/run.go   startup / housekeeping: nothing sweeps
//                                  reservations
//
// Two transition relations share one state space:
//
//   step_legacy  faithful to the current code:
//     * Reserve is one atomic INSERT..SELECT that checks only
//       allowance - used + topup - SUM(reserved) >= amount. It does NOT look
//       at the subscription status: the app's Plan()/HasAccess check is a
//       separate earlier read (TOCTOU), and Explain relies on the web
//       `member` gate only. We model the store-level guard (no status check).
//     * Settle deletes the row; a missing row is a silent no-op (the real
//       cost is dropped). Cost 0 is a plain release. Otherwise Spend:
//         period_used = MIN(allowance, used + cost)
//         topup      -= MAX(0, cost - MAX(0, allowance - used))
//       Uncapped: cost is not bounded by the reserved amount (Cost() prices
//       unknown models at the top rate) and topup may go negative.
//     * Leaky paths (Abandon): Answer skips settle when the judge fails
//       without a model; runDraft skips settle on error-without-model and on
//       panic; Transcribe skips settle when speech fails.
//     * Crash leaves every in-flight row held forever: no created_at, no TTL,
//       no startup sweep (main.go only runs FailStaleDrafts for drafts).
//
//   step  the proposed fix:
//     * Reserve is guarded *in the same atomic statement* by access:
//       trialing / active / past_due with now - past_due_since <= GRACE.
//       Refused once grace has expired or the plan has ended.
//     * Settle debits d = min(cost, reserved amount, available) and never
//       drives a balance negative; the overage cost - d is recorded in
//       `written_off` (an operator-visible ledger), not charged. CHOSEN
//       POLICY: "cap at reservation, write off the rest".
//     * Every reservation carries a boot epoch and a created time. Restart
//       runs a startup sweep releasing every reservation from earlier boots.
//       Housekeeping expires reservations older than TTL (ExpireTTL); the
//       housekeeping ticker is modelled by forbidding time to advance past a
//       reservation's TTL without the sweep having run (AdvanceTime guard).
//     * Settle of an already settled / released / swept / expired id is a
//       no-op (cost recorded in `dropped`). Residual risk kept visible: a
//       slow owner settling after its row hit TTL loses that usage, so TTL
//       must exceed the longest provider call (runDraft has no deadline).
//
// Time is abstract ticks (one tick = one housekeeping window); GRACE and TTL
// are counted in ticks. Amounts are micros, scaled down.
//
// Verification (see /home/user/chief-evidence/quint-allowance_reservations.log):
//   quint test; quint run --invariant=inv_all (step) and
//   --step=step_legacy (violations); quint verify --invariant=inv_all
//   --max-steps=5/6; quint verify --inductive-invariant=inv_inductive, an
//   unbounded proof that inv_all holds in every reachable state of `step`.
module allowance_reservations {

  // ---------------------------------------------------------------- constants
  type RID = str
  pure val RIDS: Set[RID] = Set("r1", "r2", "r3")
  pure val ALLOWANCE = 10        // Cfg.AllowanceMicros (3_000_000 scaled)
  pure val TRIAL_ALLOWANCE = 6   // Cfg.TrialMicros (1_000_000 scaled; < ALLOWANCE)
  pure val TOPUP_CREDIT = 4      // Cfg.TopupMicros
  pure val GRACE = 2             // Cfg.GraceDays, in ticks
  pure val TTL = 2               // proposed reservation TTL, in ticks
  pure val AMOUNTS = 1.to(4)     // reserve budgets (Judge/Explain/Draft/Speech)
  pure val COSTS = 0.to(6)       // actual cost; may exceed the reservation

  pure def imin(a: int, b: int): int = if (a < b) a else b
  pure def imax(a: int, b: int): int = if (a > b) a else b

  // -------------------------------------------------------------------- state
  // subscriptions row
  var status: str                // "trialing" | "active" | "past_due" | "ended"
  var now: int
  var past_due_since: int
  var allowance: int             // allowance_micros
  var used: int                  // period_used_micros
  var topup: int                 // topup_micros (never expires)
  // process
  var up: bool                   // process running
  var epoch: int                 // boot counter
  // reservations table (ids are random tokens: each id is used at most once)
  // "free"   : id not issued yet
  // "held"   : row present, a live goroutine owns the id and will settle it
  // "orphan" : row present, nobody can settle it any more (leaky path or crash)
  // "settled" | "released" | "swept" | "expired" : row deleted (terminal)
  var res_state: RID -> str
  var res_amount: RID -> int
  var res_epoch: RID -> int      // fix: boot epoch column
  var res_created: RID -> int    // fix: created_at column
  // ghost / history
  var debited: RID -> int        // what settling this reservation drew from the balances
  var settle_deletes: int        // Settle calls whose DELETE removed a row
  var wiped: int                 // unused-allowance accounting: used discarded by period resets
  var credited: int              // all top-up credit ever granted
  var written_off: int           // fix: overage not charged to the learner
  var dropped: int               // cost of settles that found no row (lost usage)
  var opened_without_access: bool // a reservation was opened while HasAccess was false
  var last_action: str

  // ---------------------------------------------------------------- derived
  def has_access: bool =
    status == "trialing" or status == "active" or
    (status == "past_due" and now - past_due_since <= GRACE)

  def reserved_total: int =
    RIDS.fold(0, (acc, r) => if (held(r)) acc + res_amount.get(r) else acc)

  // what the Reserve statement compares against `amount`
  def available: int = allowance - used + topup - reserved_total

  def total_debited: int = RIDS.fold(0, (acc, r) => acc + debited.get(r))

  // the row is still in the reservations table
  def held(r: RID): bool = res_state.get(r) == "held" or res_state.get(r) == "orphan"

  // enabling conditions (shared by the actions and by the tests)
  def reserve_legacy_ok(r: RID, amt: int): bool =
    up and res_state.get(r) == "free" and amt > 0 and available >= amt
  def reserve_ok(r: RID, amt: int): bool = reserve_legacy_ok(r, amt) and has_access
  def owner_can_settle(r: RID): bool = up and res_state.get(r) == "held"
  def expire_ok(r: RID): bool = up and held(r) and now - res_created.get(r) >= TTL
  def advance_ok: bool =
    not(up) or RIDS.forall(r => held(r) implies now - res_created.get(r) < TTL)

  // ------------------------------------------------------------------- frames
  action keep_sub = all {
    status' = status, now' = now, past_due_since' = past_due_since, allowance' = allowance,
  }
  action keep_bal = all { used' = used, topup' = topup }
  action keep_proc = all { up' = up, epoch' = epoch }
  action keep_res = all {
    res_state' = res_state, res_amount' = res_amount, res_epoch' = res_epoch,
    res_created' = res_created,
  }
  action keep_acct = all {
    debited' = debited, settle_deletes' = settle_deletes, wiped' = wiped, credited' = credited,
    written_off' = written_off, dropped' = dropped,
  }
  action keep_hist = all { opened_without_access' = opened_without_access }

  // --------------------------------------------------------------------- init
  action init = all {
    status' = "trialing",
    now' = 0,
    past_due_since' = 0,
    allowance' = TRIAL_ALLOWANCE,
    used' = 0,
    topup' = 0,
    up' = true,
    epoch' = 0,
    res_state' = RIDS.mapBy(r => "free"),
    res_amount' = RIDS.mapBy(r => 0),
    res_epoch' = RIDS.mapBy(r => 0),
    res_created' = RIDS.mapBy(r => 0),
    debited' = RIDS.mapBy(r => 0),
    settle_deletes' = 0,
    wiped' = 0,
    credited' = 0,
    written_off' = 0,
    dropped' = 0,
    opened_without_access' = false,
    last_action' = "Init",
  }

  // ------------------------------------------------------------------ Reserve
  // shared effect of a successful INSERT
  action open_reservation(r: RID, amt: int): bool = all {
    res_state' = res_state.set(r, "held"),
    res_amount' = res_amount.set(r, amt),
    res_epoch' = res_epoch.set(r, epoch),
    res_created' = res_created.set(r, now),
    opened_without_access' = (opened_without_access or not(has_access)),
    keep_sub, keep_bal, keep_proc, keep_acct,
    last_action' = "Reserve",
  }

  // legacy: store.Reserve checks only the budget (no status / grace).
  action ReserveLegacy(r: RID, amt: int): bool = all {
    reserve_legacy_ok(r, amt),
    open_reservation(r, amt),
  }

  // fix: access is part of the same atomic statement.
  action Reserve(r: RID, amt: int): bool = all {
    reserve_ok(r, amt),
    open_reservation(r, amt),
  }

  // -------------------------------------------------------------------- Spend
  // legacy store.Spend: SQLite evaluates every SET expression on the old row.
  action SpendLegacy(r: RID, cost: int): bool = all {
    used' = imin(allowance, used + cost),
    topup' = topup - imax(0, cost - imax(0, allowance - used)),
    debited' = debited.set(r, cost),
    written_off' = written_off,
  }

  // fix: debit at most the reservation and at most what is available;
  // allowance first, then top-up; the rest is written off.
  action Spend(r: RID, cost: int): bool = {
    val d = imin(cost, imin(res_amount.get(r), imax(0, allowance - used + topup)))
    val from_allowance = imin(d, imax(0, allowance - used))
    all {
      used' = used + from_allowance,
      topup' = topup - (d - from_allowance),
      debited' = debited.set(r, d),
      written_off' = written_off + (cost - d),
    }
  }

  // ------------------------------------------------------------------- Settle
  // Settle of a row that is no longer there (already settled / released /
  // swept / expired): DELETE affects 0 rows -> return nil. Identical in both.
  action SettleNoop(r: RID, cost: int): bool = all {
    up,
    res_state.get(r) != "free",
    res_state.get(r) != "held",
    dropped' = dropped + cost,
    debited' = debited, settle_deletes' = settle_deletes, wiped' = wiped, credited' = credited,
    written_off' = written_off,
    keep_sub, keep_bal, keep_proc, keep_res, keep_hist,
    last_action' = "SettleNoop",
  }

  // the row is deleted by its live owner
  action delete_row(r: RID): bool = all {
    res_state' = res_state.set(r, "settled"),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    settle_deletes' = settle_deletes + 1,
  }

  action SettleLegacy(r: RID, cost: int): bool = any {
    all {
      owner_can_settle(r),
      delete_row(r),
      if (cost == 0)
        all { keep_bal, debited' = debited, written_off' = written_off }
      else
        SpendLegacy(r, cost),
      wiped' = wiped, credited' = credited, dropped' = dropped,
      keep_sub, keep_proc, keep_hist,
      last_action' = "Settle",
    },
    SettleNoop(r, cost),
  }

  action Settle(r: RID, cost: int): bool = any {
    all {
      owner_can_settle(r),
      delete_row(r),
      Spend(r, cost),       // cost 0 => d = 0, a pure release
      wiped' = wiped, credited' = credited, dropped' = dropped,
      keep_sub, keep_proc, keep_hist,
      last_action' = "Settle",
    },
    SettleNoop(r, cost),
  }

  // ------------------------------------------------------------------ Release
  // explicit release of an unused reservation (settle with empty usage on the
  // Learn/runDraft error paths). Same in both.
  action Release(r: RID): bool = all {
    owner_can_settle(r),
    res_state' = res_state.set(r, "released"),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    keep_sub, keep_bal, keep_proc, keep_acct, keep_hist,
    last_action' = "Release",
  }

  // ------------------------------------------------------------------ Abandon
  // a leaky code path returns without settling: Answer judge failure with no
  // model (study.go), runDraft error-without-model / panic (learn.go),
  // Transcribe speech failure (learn.go). Nobody holds the id any more.
  action Abandon(r: RID): bool = all {
    owner_can_settle(r),
    res_state' = res_state.set(r, "orphan"),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    keep_sub, keep_bal, keep_proc, keep_acct, keep_hist,
    last_action' = "Abandon",
  }

  // ---------------------------------------------------------------- ExpireTTL
  // fix only: housekeeping releases reservations older than TTL.
  action ExpireTTL(r: RID): bool = all {
    expire_ok(r),
    res_state' = res_state.set(r, "expired"),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    keep_sub, keep_bal, keep_proc, keep_acct, keep_hist,
    last_action' = "ExpireTTL",
  }

  // -------------------------------------------------------------------- TopUp
  // TopUp: topupEligible requires HasAccess (both legacy and fix).
  action TopUp: bool = all {
    up, has_access,
    topup' = topup + TOPUP_CREDIT,
    credited' = credited + TOPUP_CREDIT,
    used' = used,
    debited' = debited, settle_deletes' = settle_deletes, wiped' = wiped,
    written_off' = written_off, dropped' = dropped,
    keep_sub, keep_proc, keep_res, keep_hist,
    last_action' = "TopUp",
  }

  // ------------------------------------------------------ subscription status
  // InvoicePaid = monthly period reset: active, used = 0, allowance reset.
  action InvoicePaid: bool = all {
    status != "ended",
    status' = "active",
    past_due_since' = 0,
    allowance' = ALLOWANCE,
    used' = 0,
    wiped' = wiped + used,
    now' = now, topup' = topup,
    debited' = debited, settle_deletes' = settle_deletes, credited' = credited,
    written_off' = written_off, dropped' = dropped,
    keep_proc, keep_res, keep_hist,
    last_action' = "InvoicePaid",
  }

  // InvoiceFailed -> past_due; PastDueSince only set the first time.
  action InvoiceFailed: bool = all {
    status != "ended",
    status' = "past_due",
    past_due_since' = if (status == "past_due") past_due_since else now,
    now' = now, allowance' = allowance,
    keep_bal, keep_proc, keep_res, keep_acct, keep_hist,
    last_action' = "InvoiceFailed",
  }

  action SubscriptionEnded: bool = all {
    status != "ended",
    status' = "ended",
    now' = now, past_due_since' = past_due_since, allowance' = allowance,
    keep_bal, keep_proc, keep_res, keep_acct, keep_hist,
    last_action' = "SubscriptionEnded",
  }

  // OpenPlan replaces an ended plan; top-up credit is carried over.
  action Resubscribe: bool = all {
    status == "ended",
    status' = "active",
    past_due_since' = 0,
    allowance' = ALLOWANCE,
    used' = 0,
    wiped' = wiped + used,
    now' = now, topup' = topup,
    debited' = debited, settle_deletes' = settle_deletes, credited' = credited,
    written_off' = written_off, dropped' = dropped,
    keep_proc, keep_res, keep_hist,
    last_action' = "Resubscribe",
  }

  // -------------------------------------------------------------------- time
  action tick = all {
    now' = now + 1,
    status' = status, past_due_since' = past_due_since, allowance' = allowance,
    keep_bal, keep_proc, keep_res, keep_acct, keep_hist,
    last_action' = "AdvanceTime",
  }

  action AdvanceTimeLegacy: bool = tick

  // fix: the hourly housekeeping sweep runs inside every TTL window, so while
  // the process is up time cannot pass a reservation's TTL unexpired.
  action AdvanceTime: bool = all {
    advance_ok,
    tick,
  }

  // ------------------------------------------------------ crash and recovery
  action Crash: bool = all {
    up,
    up' = false,
    epoch' = epoch,
    // every in-flight goroutine dies: its row is orphaned
    res_state' = RIDS.mapBy(r => if (held(r)) "orphan" else res_state.get(r)),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    opened_without_access' = opened_without_access,
    keep_sub, keep_bal, keep_acct,
    last_action' = "Crash",
  }

  // legacy startup: Integrity, FailStaleDrafts, ... nothing for reservations.
  action RestartLegacy: bool = all {
    not(up),
    up' = true,
    epoch' = epoch + 1,
    keep_sub, keep_bal, keep_res, keep_acct, keep_hist,
    last_action' = "Restart",
  }

  // fix startup sweep: release every reservation from an earlier boot.
  action Restart: bool = all {
    not(up),
    up' = true,
    epoch' = epoch + 1,
    res_state' = RIDS.mapBy(r =>
      if (held(r) and res_epoch.get(r) < epoch + 1) "swept" else res_state.get(r)),
    res_amount' = res_amount, res_epoch' = res_epoch, res_created' = res_created,
    keep_sub, keep_bal, keep_acct, keep_hist,
    last_action' = "Restart",
  }

  // -------------------------------------------------------------------- steps
  action step_legacy = {
    nondet r = RIDS.oneOf()
    nondet amt = AMOUNTS.oneOf()
    nondet cost = COSTS.oneOf()
    any {
      ReserveLegacy(r, amt),
      SettleLegacy(r, cost),
      Release(r),
      Abandon(r),
      TopUp,
      InvoicePaid,
      InvoiceFailed,
      SubscriptionEnded,
      Resubscribe,
      AdvanceTimeLegacy,
      Crash,
      RestartLegacy,
    }
  }

  action step = {
    nondet r = RIDS.oneOf()
    nondet amt = AMOUNTS.oneOf()
    nondet cost = COSTS.oneOf()
    any {
      Reserve(r, amt),
      Settle(r, cost),
      Release(r),
      Abandon(r),
      ExpireTTL(r),
      TopUp,
      InvoicePaid,
      InvoiceFailed,
      SubscriptionEnded,
      Resubscribe,
      AdvanceTime,
      Crash,
      Restart,
    }
  }

  // --------------------------------------------------------------- invariants
  val inv_non_negative_balances: bool = and {
    used >= 0,
    used <= allowance,
    topup >= 0,
    reserved_total >= 0,
  }

  // while healthy, every held micro is backed by allowance or credit
  val inv_reserved_within_available: bool =
    up implies reserved_total <= (allowance - used) + topup

  // immediately after Crash + Restart (startup sweep): no reservation opened
  // before the crash (i.e. in an earlier boot epoch; Reserve needs the
  // process up, so every row held at the crash has res_epoch < epoch) is
  // still held, and the whole balance is available again.
  val inv_no_leaked_reservations_after_recovery: bool =
    last_action == "Restart" implies and {
      RIDS.forall(r => res_epoch.get(r) < epoch implies not(held(r))),
      available == allowance - used + topup,
    }

  // a row is deleted (and debited) by Settle at most once: the number of
  // deleting settles equals the number of settled rows; a debit only exists
  // on a settled row; and balances moved by exactly the sum of single debits
  // (debited[r] is *assigned*, so a second debit of r would break this):
  //   discarded-at-reset + used + credit consumed == sum of per-reservation debits
  val inv_idempotent_settle: bool = and {
    settle_deletes == RIDS.fold(0, (n, r) => if (res_state.get(r) == "settled") n + 1 else n),
    RIDS.forall(r => debited.get(r) != 0 implies res_state.get(r) == "settled"),
    wiped + used + (credited - topup) == total_debited,
  }

  val inv_grace_expiry_blocks_new_reservations: bool = not(opened_without_access)

  // extra (fix): a settle never charges more than was reserved
  val inv_debit_within_reservation: bool =
    RIDS.forall(r => debited.get(r) <= res_amount.get(r))

  // extra (fix): while up, no reservation outlives its TTL or its boot
  val inv_no_stale_reservations: bool =
    up implies RIDS.forall(r => held(r) implies and {
      now - res_created.get(r) <= TTL,
      res_epoch.get(r) == epoch,
    })

  val inv_all: bool = and {
    inv_non_negative_balances,
    inv_reserved_within_available,
    inv_no_leaked_reservations_after_recovery,
    inv_idempotent_settle,
    inv_grace_expiry_blocks_new_reservations,
    inv_debit_within_reservation,
    inv_no_stale_reservations,
  }

  // ---------------------------------------------------- inductive invariant
  // inv_all strengthened with type/shape facts so that it is inductive:
  //   init => inv_inductive  and  inv_inductive /\ step => inv_inductive'
  // (checked by Apalache with --inductive-invariant: an unbounded proof of
  // inv_all for the fixed `step`).
  pure val STATUSES = Set("trialing", "active", "past_due", "ended")
  pure val STATES = Set("free", "held", "orphan", "settled", "released", "swept", "expired")
  pure val ACTIONS = Set("Init", "Reserve", "Settle", "SettleNoop", "Release", "Abandon", "ExpireTTL",
                         "TopUp", "InvoicePaid", "InvoiceFailed", "SubscriptionEnded", "Resubscribe",
                         "AdvanceTime", "Crash", "Restart")

  val inv_types: bool = and {
    status.in(STATUSES),
    now.in(Int), past_due_since.in(Int),
    allowance.in(Set(TRIAL_ALLOWANCE, ALLOWANCE)),
    used.in(Int), topup.in(Int),
    up.in(Bool), epoch.in(Int),
    res_state.in(setOfMaps(RIDS, STATES)),
    res_amount.in(setOfMaps(RIDS, 0.to(4))),
    res_epoch.in(setOfMaps(RIDS, Int)),
    res_created.in(setOfMaps(RIDS, Int)),
    debited.in(setOfMaps(RIDS, Int)),
    settle_deletes.in(Int), wiped.in(Int), credited.in(Int),
    written_off.in(Int), dropped.in(Int),
    opened_without_access.in(Bool),
    last_action.in(ACTIONS),
  }

  val inv_inductive: bool = and {
    inv_types,
    inv_all,
    // strengthening
    reserved_total <= (allowance - used) + topup,          // also while down
    RIDS.forall(r => held(r) implies res_epoch.get(r) <= epoch),
    RIDS.forall(r => res_state.get(r) == "free" implies res_amount.get(r) == 0),
    RIDS.forall(r => debited.get(r) >= 0),
  }

  // ------------------------------------------------------------------- tests
  // concurrent requests interleave; each Reserve is atomic; the last is refused
  run budgetCapTest =
    init
      .then(InvoicePaid)                       // allowance 10
      .then(Reserve("r1", 4))
      .then(Reserve("r2", 4))
      .expect(available == 2)
      .expect(not(reserve_ok("r3", 4)))        // over the cap: refused
      .then(Reserve("r3", 2))                  // exactly the rest fits
      .expect(available == 0 and reserved_total == 10 and inv_all)

  run partialAndZeroSettleTest =
    init
      .then(InvoicePaid)
      .then(Reserve("r1", 4))
      .then(Reserve("r2", 3))
      .then(Settle("r1", 2))                   // partial: 2 of 4
      .expect(used == 2 and reserved_total == 3 and debited.get("r1") == 2)
      .then(Settle("r2", 0))                   // zero: pure release
      .expect(used == 2 and reserved_total == 0 and available == 8 and inv_all)

  run doubleSettleTest =
    init
      .then(InvoicePaid)
      .then(Reserve("r1", 4))
      .then(Settle("r1", 3))
      .expect(used == 3)
      .then(Settle("r1", 3))                   // second settle: no-op
      .expect(used == 3 and settle_deletes == 1 and dropped == 3 and inv_all)
      .expect(not(owner_can_settle("r1")))     // cannot release a settled row

  run overCostSettleTest =
    init
      .then(InvoicePaid)
      .then(Reserve("r1", 2))
      .then(Reserve("r2", 4))
      .then(Settle("r1", 6))                   // unknown model priced high
      .expect(used == 2 and written_off == 4 and reserved_total == 4 and inv_all)

  run crashSweepTest =
    init
      .then(InvoicePaid)
      .then(Reserve("r1", 4))
      .then(Reserve("r2", 3))
      .expect(available == 3)
      .then(Crash)
      .then(Restart)
      .expect(reserved_total == 0 and available == 10 and res_state.get("r2") == "swept"
              and res_state.get("r1") == "swept" and inv_all)
      .then(Settle("r1", 4))                   // late settle of a swept id: no-op
      .expect(used == 0 and inv_all)

  run ttlExpiryTest =
    init
      .then(InvoicePaid)
      .then(Reserve("r1", 4))
      .then(Abandon("r1"))                     // leaky path: never settled
      .then(AdvanceTime)
      .expect(not(expire_ok("r1")))            // not yet old enough
      .then(AdvanceTime)
      .expect(not(advance_ok))                 // housekeeping must run first
      .then(ExpireTTL("r1"))
      .expect(reserved_total == 0 and available == 10 and inv_all)
      .then(AdvanceTime)

  run graceExpiryTest =
    init
      .then(InvoicePaid)
      .then(InvoiceFailed)                     // past_due since 0
      .then(AdvanceTime)
      .then(AdvanceTime)                       // now 2 == GRACE: still in grace
      .then(Reserve("r1", 1))
      .then(Settle("r1", 1))
      .then(AdvanceTime)                       // now 3: grace expired
      .expect(not(has_access))
      .expect(not(reserve_ok("r2", 1)))
      .then(InvoicePaid)                       // card retried and paid
      .then(Reserve("r2", 1))
      .then(SubscriptionEnded)
      .expect(not(reserve_ok("r3", 1)))        // ended: refused
      .expect(inv_all)

  run periodResetTest =
    init
      .then(Reserve("r1", 4))                  // trial allowance 6
      .then(Settle("r1", 4))
      .then(TopUp)
      .expect(used == 4 and topup == 4 and available == 6)
      .then(InvoicePaid)                       // new month
      .expect(used == 0 and allowance == ALLOWANCE and topup == 4 and available == 14
              and wiped == 4 and inv_all)

  // ---- legacy bugs
  run legacyNegativeTopupTest =
    init
      .then(ReserveLegacy("r1", 1))
      .then(ReserveLegacy("r2", 1))
      .then(SettleLegacy("r1", 6))             // cost 6 against a 1-micro hold
      .expect(used == 6 and not(inv_reserved_within_available))
      .then(SettleLegacy("r2", 6))
      .expect(topup == -6 and not(inv_non_negative_balances) and not(inv_debit_within_reservation))

  run legacyCrashLeakTest =
    init
      .then(ReserveLegacy("r1", 4))
      .then(Crash)
      .then(RestartLegacy)
      .expect(held("r1") and available == 2 and not(inv_no_leaked_reservations_after_recovery))
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .expect(not(owner_can_settle("r1")))     // nobody can settle it: held forever
      .expect(held("r1") and not(inv_no_stale_reservations))

  run legacyLeakyPathTest =
    init
      .then(ReserveLegacy("r1", 4))
      .then(Abandon("r1"))                     // judge failed with no model
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .expect(held("r1") and available == 2 and not(inv_no_stale_reservations))

  run legacyReserveAfterGraceTest =
    init
      .then(InvoiceFailed)
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .then(AdvanceTimeLegacy)
      .expect(not(has_access))
      .then(ReserveLegacy("r1", 1))            // store does not check status
      .expect(not(inv_grace_expiry_blocks_new_reservations))
      .then(SubscriptionEnded)
      .then(ReserveLegacy("r2", 1))            // even an ended plan

  // legacy Settle is idempotent too (DELETE..RowsAffected), but loses the cost
  run legacyDoubleSettleTest =
    init
      .then(ReserveLegacy("r1", 2))
      .then(SettleLegacy("r1", 2))
      .then(SettleLegacy("r1", 2))
      .expect(used == 2 and settle_deletes == 1 and dropped == 2 and inv_idempotent_settle)
}
