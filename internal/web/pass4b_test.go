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

// webhookObject delivers a signed event with exactly the given
// PaymentIntent object, so a test can shape malformed objects.
func (w *world) webhookObject(id, typ string, obj map[string]any) int {
	w.t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "type": typ, "created": w.now().Unix(), "livemode": false,
		"data": map[string]any{"object": obj}})
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

// canceledState is a released attempt as SQLite holds it.
func canceledState(t *testing.T, w *world, uid int64) string {
	t.Helper()
	att := onlyAttempt(t, w, uid)
	us := w.store.For(uid)
	sub, _ := us.Subscription(ctx)
	n, _ := us.MonthTopups(ctx, w.now())
	var paid, failed int
	w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='paid'", uid).Scan(&paid)
	w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='failed'", uid).Scan(&failed)
	return fmt.Sprintf("%s|charge_ref=%s|credit=%d|paid_ledger=%d|failed_ledger=%d|reason=%q|card_error=%q|pending_slots=%d",
		att.Status, att.ChargeRef, sub.TopupMicros, paid, failed, att.Reason, sub.CardError, n)
}

// Pass 4b: a signed payment_intent.canceled webhook (ID-only or keyed)
// that names a processor error records it as the card fault with one
// failed history line, keeping the intent's id and releasing the slot;
// one with no message releases fault-free.
func TestP4b_CanceledWebhookKeepsSuppliedReason(t *testing.T) {
	const msg = "The processor rejected this card."
	for _, keyed := range []bool{false, true} {
		for _, reason := range []string{msg, ""} {
			t.Run(fmt.Sprintf("keyed=%t/reason=%q", keyed, reason), func(t *testing.T) {
				w, b, st, uid := stripeWorld(t, fmt.Sprintf("p4b-cancel-%t-%d@example.com", keyed, len(reason)))
				st.set(func() { st.answer = "requires_action" })
				b.postTopup()
				pi := st.only()
				pendingHoldsSlot(t, w, uid, pi.ID)
				var code int
				if keyed {
					st.set(func() { pi.Status, pi.Reason = "canceled", reason })
					code = w.webhook("evt_p4b_cancel", "payment_intent.canceled", pi)
				} else {
					code = w.webhookIDOnly("evt_p4b_cancel", "payment_intent.canceled", pi.ID, pi.Customer, reason)
				}
				if code != http.StatusNoContent {
					t.Fatalf("webhook: %d", code)
				}
				failedLines := 0
				if reason != "" {
					failedLines = 1
				}
				want := fmt.Sprintf("failed|charge_ref=%s|credit=0|paid_ledger=0|failed_ledger=%d|reason=%q|card_error=%q|pending_slots=0", pi.ID, failedLines, reason, reason)
				if got := canceledState(t, w, uid); got != want {
					t.Fatalf("%s\nwant %s", got, want)
				}
			})
		}
	}
}

// Pass 4b: a signed key-only payment_intent.succeeded whose object has an
// empty id is ignored (204) for each of two pending attempts: both keep
// their saved charge_ref, stay pending with no credit, and no ledger line
// has a blank ref. The genuine webhooks then settle each exactly once.
func TestP4b_EmptyIDSucceededWebhookIsIgnored(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "p4b-emptyid@example.com")
	st.set(func() { st.answer = "requires_action" })
	b.postTopup()
	b.mustGet("/billing")
	b.postTopup()
	st.mu.Lock()
	var pis []*standInIntent
	for _, id := range []string{"pi_standin_1", "pi_standin_2"} {
		pis = append(pis, st.intents[id])
	}
	st.mu.Unlock()
	if pis[0] == nil || pis[1] == nil || pis[0].Key == pis[1].Key {
		t.Fatalf("want two distinct intents: %+v", pis)
	}
	state := func() string {
		var b strings.Builder
		rows, err := w.store.DB().Query("SELECT idem_key||'|'||status||'|'||charge_ref FROM topups WHERE user_id=? ORDER BY charge_ref", uid)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			rows.Scan(&s)
			b.WriteString(s + ";")
		}
		rows.Close()
		var credit int64
		var paid, blank int
		w.store.DB().QueryRow("SELECT topup_micros FROM subscriptions WHERE user_id=?", uid).Scan(&credit)
		w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE user_id=? AND kind='topup' AND status='paid'", uid).Scan(&paid)
		w.store.DB().QueryRow("SELECT count(*) FROM ledger WHERE ref=''").Scan(&blank)
		return fmt.Sprintf("%scredit=%d|paid_ledger=%d|blank_refs=%d", b.String(), credit, paid, blank)
	}
	before := billingRows(t, w)
	for i, pi := range pis {
		obj := map[string]any{"id": "", "object": "payment_intent", "status": "succeeded", "customer": pi.Customer,
			"metadata": map[string]string{"scry_key": pi.Key}}
		if code := w.webhookObject(fmt.Sprintf("evt_p4b_noid_%d", i), "payment_intent.succeeded", obj); code != http.StatusNoContent {
			t.Fatalf("empty-id webhook %d: %d", i, code)
		}
	}
	want := fmt.Sprintf("%s|pending|%s;%s|pending|%s;credit=0|paid_ledger=0|blank_refs=0", pis[0].Key, pis[0].ID, pis[1].Key, pis[1].ID)
	if got := state(); got != want {
		t.Fatalf("after empty-id webhooks: %s\nwant %s", got, want)
	}
	if after := billingRows(t, w); after != before {
		t.Fatalf("empty-id webhooks changed billing rows:\n%s\n%s", before, after)
	}
	for i, pi := range pis {
		st.set(func() { pi.Status = "succeeded" })
		if code := w.webhookIDOnly(fmt.Sprintf("evt_p4b_ok_%d", i), "payment_intent.succeeded", pi.ID, pi.Customer, ""); code != http.StatusNoContent {
			t.Fatalf("genuine webhook %d: %d", i, code)
		}
	}
	want = fmt.Sprintf("%s|succeeded|%s;%s|succeeded|%s;credit=8000000|paid_ledger=2|blank_refs=0", pis[0].Key, pis[0].ID, pis[1].Key, pis[1].ID)
	if got := state(); got != want {
		t.Fatalf("after genuine webhooks: %s\nwant %s", got, want)
	}
}
