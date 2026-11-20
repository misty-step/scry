package web_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// webhookIDOnly delivers a signed PaymentIntent event whose object carries
// no metadata.scry_key at all, as Stripe sends for an intent created
// without Scry's metadata (or whose metadata was dropped): only the
// intent's id, its customer and any failure reason.
func (w *world) webhookIDOnly(id, typ, piID, customer, reason string) int {
	w.t.Helper()
	obj := map[string]any{"id": piID, "object": "payment_intent", "customer": customer, "metadata": map[string]string{},
		"last_payment_error": map[string]string{"message": reason}}
	body, _ := json.Marshal(map[string]any{"id": id, "type": typ, "created": w.now().Unix(), "livemode": false,
		"data": map[string]any{"object": obj}})
	if strings.Contains(string(body), "scry_key") {
		w.t.Fatal("ID-only webhook carries scry_key")
	}
	stamp := strconv.FormatInt(w.now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(standInWebhookSecret))
	mac.Write([]byte(stamp + "."))
	mac.Write(body)
	req, _ := http.NewRequest("POST", w.srv.URL+"/webhooks/stripe", strings.NewReader(string(body)))
	req.Header.Set("Stripe-Signature", "t="+stamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// billingRows is every billing row in SQLite, for checking that nothing
// changed.
func billingRows(t *testing.T, w *world) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		"SELECT user_id||'|'||token||'|'||status||'|'||charge_ref||'|'||reason||'|'||settled_at FROM topups ORDER BY user_id, token",
		"SELECT user_id||'|'||kind||'|'||status||'|'||amount_cents||'|'||ref FROM ledger ORDER BY user_id, ref",
		"SELECT user_id||'|'||topup_micros||'|'||card_error||'|'||status FROM subscriptions ORDER BY user_id",
		"SELECT id FROM processed_events ORDER BY id",
	} {
		rows, err := w.store.DB().Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			b.WriteString(s + "\n")
		}
		rows.Close()
		b.WriteString("--\n")
	}
	return b.String()
}

// settledOnce reads SQLite directly (no Billing page load, so no
// reconciliation) and checks the attempt succeeded with the intent's id,
// exactly one $4 of credit and exactly one paid top-up line, and no failed
// line left behind.
func settledOnce(t *testing.T, w *world, uid int64, piID string) {
	t.Helper()
	var status, ref string
	var credit int64
	var paid, failed int
	db := w.store.DB()
	if err := db.QueryRow("SELECT status, charge_ref FROM topups WHERE user_id=?", uid).Scan(&status, &ref); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT topup_micros FROM subscriptions WHERE user_id=?", uid).Scan(&credit); err != nil {
		t.Fatal(err)
	}
	db.QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='paid'", uid).Scan(&paid)
	db.QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='failed'", uid).Scan(&failed)
	got := fmt.Sprintf("%s|charge_ref=%s|credit=%d|paid_ledger=%d|failed_ledger=%d", status, ref, credit, paid, failed)
	want := fmt.Sprintf("succeeded|charge_ref=%s|credit=4000000|paid_ledger=1|failed_ledger=0", piID)
	if got != want || w.app.Cfg.TopupMicros != 4000000 {
		t.Fatalf("after webhook, before any reload: %s; want %s", got, want)
	}
}

// F2 (Pass 4): a signed payment_intent.succeeded webhook WITHOUT
// metadata.scry_key settles the attempt that saved that intent's id as
// charge_ref: a failed attempt (HTTP 402 naming the intent) and a pending
// one (HTTP 200 requires_action). The credit is granted on delivery, before
// any Billing reload, exactly once; replays (same and distinct event ids)
// answer 204 and never credit again.
func TestF2P4_IDOnlySucceededWebhookSettlesBySavedChargeRef(t *testing.T) {
	for _, start := range []string{"failed-402", "pending-requires_action"} {
		t.Run(start, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, "idonly-"+start+"@example.com")
			if start == "failed-402" {
				st.set(func() { st.decline = "Your card was declined." })
				if loc := b.postTopup(); loc != "/billing?declined=Your+card+was+declined." {
					t.Fatalf("402 redirected to %s", loc)
				}
				failedWithRef(t, w, uid, st.only().ID)
			} else {
				st.set(func() { st.answer = "requires_action" })
				if loc := b.postTopup(); loc != "/billing?ok=pending" {
					t.Fatalf("requires_action redirected to %s", loc)
				}
				pendingHoldsSlot(t, w, uid, st.only().ID)
			}
			pi := st.only()
			st.set(func() { pi.Status = "succeeded" })
			for _, evID := range []string{"evt_p4_ok", "evt_p4_ok", "evt_p4_ok_2"} {
				if code := w.webhookIDOnly(evID, "payment_intent.succeeded", pi.ID, pi.Customer, ""); code != http.StatusNoContent {
					t.Fatalf("%s: %d", evID, code)
				}
				settledOnce(t, w, uid, pi.ID) // straight from SQLite, before any reload
			}
			exactlyOneCredit(t, w, st, uid)
			has(t, b.mustGet("/billing"), "You also have $4 of top-up credit")
			settledOnce(t, w, uid, pi.ID)
		})
	}
}

// F2 (Pass 4): signed payment_failed and canceled webhooks WITHOUT
// metadata.scry_key release a pending attempt found by its saved
// charge_ref, keeping that id. A failure names its card fault; a
// cancellation records none.
func TestF2P4_IDOnlyFailureWebhooksSettlePendingBySavedChargeRef(t *testing.T) {
	for _, typ := range []string{"payment_intent.payment_failed", "payment_intent.canceled"} {
		t.Run(typ, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, "idonly-"+strings.TrimPrefix(typ, "payment_intent.")+"@example.com")
			st.set(func() { st.answer = "requires_action" })
			b.postTopup()
			pi := st.only()
			pendingHoldsSlot(t, w, uid, pi.ID)
			reason := ""
			if typ == "payment_intent.payment_failed" {
				reason = "Your card has insufficient funds."
			}
			if code := w.webhookIDOnly("evt_p4_fail", typ, pi.ID, pi.Customer, reason); code != http.StatusNoContent {
				t.Fatalf("webhook: %d", code)
			}
			att := failedWithRef(t, w, uid, pi.ID)
			sub, _ := w.store.For(uid).Subscription(ctx)
			var failed int
			w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND status='failed'", uid).Scan(&failed)
			wantFailed := 0
			if reason != "" {
				wantFailed = 1
			}
			if att.Reason != reason || sub.CardError != reason || failed != wantFailed {
				t.Fatalf("reason=%q card_error=%q failed_lines=%d; want %q and %d", att.Reason, sub.CardError, failed, reason, wantFailed)
			}
		})
	}
}

// F2 (Pass 4): a signed payment_intent.succeeded without metadata.scry_key
// for a PaymentIntent no attempt recorded (a plan's invoice payment) is
// acknowledged with 204 and changes nothing: no attempt, credit, ledger
// line or processed-event marker.
func TestF2P4_IDOnlyUnrelatedIntentWebhookChangesNothing(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "idonly-unrelated@example.com")
	st.set(func() { st.answer = "requires_action" })
	b.postTopup()
	pi := st.only()
	before := billingRows(t, w)
	for _, cus := range []string{pi.Customer, "cus_someone_else", ""} {
		if code := w.webhookIDOnly("evt_p4_invoice_"+cus, "payment_intent.succeeded", "pi_invoice_unrelated", cus, ""); code != http.StatusNoContent {
			t.Fatalf("unrelated intent: %d", code)
		}
	}
	if after := billingRows(t, w); after != before {
		t.Fatalf("an unrelated intent changed billing rows:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	pendingHoldsSlot(t, w, uid, pi.ID)
}

// F2 (Pass 4): the customer guard still holds for ID-only webhooks: a
// succeeded event naming the saved charge_ref but another customer is
// refused (no credit, no event marker), and the genuine one still credits.
func TestF2P4_IDOnlyWebhookWrongCustomerIsRefused(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "idonly-wrongcus@example.com")
	st.set(func() { st.answer = "requires_action" })
	b.postTopup()
	pi := st.only()
	before := billingRows(t, w)
	if code := w.webhookIDOnly("evt_p4_wrong", "payment_intent.succeeded", pi.ID, "cus_attacker", ""); code != http.StatusNoContent {
		t.Fatalf("wrong customer: %d", code)
	}
	if after := billingRows(t, w); after != before {
		t.Fatalf("a wrong-customer webhook changed billing rows:\n%s\n%s", before, after)
	}
	pendingHoldsSlot(t, w, uid, pi.ID)
	st.set(func() { pi.Status = "succeeded" })
	w.webhookIDOnly("evt_p4_right", "payment_intent.succeeded", pi.ID, pi.Customer, "")
	settledOnce(t, w, uid, pi.ID)
}

// releasedState is what a released top-up leaves behind.
func releasedState(t *testing.T, w *world, uid int64) string {
	t.Helper()
	us := w.store.For(uid)
	att := onlyAttempt(t, w, uid)
	sub, _ := us.Subscription(ctx)
	n, _ := us.MonthTopups(ctx, w.now())
	var failed int
	w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND status='failed'", uid).Scan(&failed)
	return fmt.Sprintf("status=%s ref_kept=%t reason=%q card_error=%q slots=%d credit=%d paid=%d failed_lines=%d",
		att.Status, att.ChargeRef != "", att.Reason, sub.CardError, n, sub.TopupMicros, paidTopups(t, us), failed)
}

// Minor (Pass 4): a direct HTTP 200 canceled PaymentIntent with no
// last_payment_error keeps its id, releases the slot with no card fault and
// no failed history line, and redirects ok=released, never inventing
// "Your card was declined.". It ends in exactly the state a lookup that
// discovers the intent canceled leaves.
func TestF2P4_DirectCanceledWithoutReasonReleasesCleanly(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "direct-canceled@example.com")
	st.set(func() { st.answer = "canceled" })
	loc := b.postTopup()
	if loc != "/billing?ok=released" {
		t.Fatalf("direct canceled redirected to %s, want ok=released", loc)
	}
	pi := st.only()
	att := failedWithRef(t, w, uid, pi.ID)
	if att.Reason != "" {
		t.Fatalf("direct canceled reason %q", att.Reason)
	}
	page := b.mustGet(loc)
	has(t, page, "did not take that top-up")
	hasNot(t, page, "Your card was declined.", "(card failed)")
	direct := releasedState(t, w, uid)

	// The same intent discovered canceled by a lookup (the reply was lost).
	w2, b2, st2, uid2 := stripeWorld(t, "lookup-canceled@example.com")
	st2.set(func() { st2.lose, st2.answer = true, "canceled" })
	b2.postTopup()
	b2.mustGet("/billing")
	if lookup := releasedState(t, w2, uid2); lookup != direct {
		t.Fatalf("direct canceled %s\nlookup canceled %s", direct, lookup)
	}
	want := `status=failed ref_kept=true reason="" card_error="" slots=0 credit=0 paid=0 failed_lines=0`
	if direct != want {
		t.Fatalf("direct canceled: %s; want %s", direct, want)
	}
	if st.postCount() != 1 {
		t.Fatalf("%d charges", st.postCount())
	}
}

// Minor (Pass 4): a 200 requires_payment_method with no sentence, and a
// 200 canceled that does carry the processor's reason, are still declines:
// the reason is shown and recorded as a card fault, and the id is kept.
func TestF2P4_DeclinesWithReasonStillReportTheReason(t *testing.T) {
	for _, tc := range []struct{ status, reason, want string }{
		{"requires_payment_method", "", "Your card was declined."},
		{"canceled", "Your card was declined by the bank.", "Your card was declined by the bank."},
	} {
		t.Run(tc.status, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, "decl-"+tc.status+"@example.com")
			st.set(func() { st.answer, st.reason = tc.status, tc.reason })
			loc := b.postTopup()
			if want := "/billing?declined=" + strings.ReplaceAll(tc.want, " ", "+"); loc != want {
				t.Fatalf("redirected to %s, want %s", loc, want)
			}
			att := failedWithRef(t, w, uid, st.only().ID)
			sub, _ := w.store.For(uid).Subscription(ctx)
			if att.Reason != tc.want || sub.CardError != tc.want {
				t.Fatalf("reason=%q card_error=%q", att.Reason, sub.CardError)
			}
		})
	}
}
