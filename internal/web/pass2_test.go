package web_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/srs"
	"scry/internal/store"
)

// stripeStandIn is a loopback stand-in for Stripe's PaymentIntents API,
// with state of its own, separate from Scry's database. It is idempotent
// in the Idempotency-Key like Stripe, can lose its reply after creating the
// intent, can hide an intent from lookups (a request Stripe has not
// finished recovering), and can return truncated lists.
type stripeStandIn struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	intents map[string]*standInIntent // by id
	byKey   map[string]string         // idempotency key -> id
	posts   int                       // POST /v1/payment_intents received
	lists   []string                  // list queries received
	gets    []string                  // ids retrieved directly
	answer  string                    // status a new intent is created in
	lose    bool                      // create the intent, then drop the connection
	hidden  bool                      // lookups do not show any intent yet
	endless bool                      // every list page says has_more
	decline string                    // refuse new intents: 402 card_error naming the intent
	apiErr  bool                      // create the intent, then answer 500 naming it
	reason  string                    // last_payment_error a new intent is created with
}

type standInIntent struct {
	ID, Status, Customer, Key, Reason string
}

func newStripeStandIn(t *testing.T) *stripeStandIn {
	s := &stripeStandIn{t: t, intents: map[string]*standInIntent{}, byKey: map[string]string{}, answer: "succeeded"}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stripeStandIn) json(pi *standInIntent) string {
	raw, _ := json.Marshal(map[string]any{"id": pi.ID, "object": "payment_intent", "status": pi.Status, "customer": pi.Customer,
		"metadata": map[string]string{"scry_key": pi.Key}, "last_payment_error": map[string]string{"message": pi.Reason}})
	return string(raw)
}

func (s *stripeStandIn) serve(rw http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rw.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == "POST" && r.URL.Path == "/v1/payment_intents":
		s.posts++
		key := r.Header.Get("Idempotency-Key")
		if key == "" || r.FormValue("metadata[scry_key]") != key || r.FormValue("confirm") != "true" {
			s.t.Errorf("charge without its key: %q %q", key, r.FormValue("metadata[scry_key]"))
		}
		id, ok := s.byKey[key]
		if !ok {
			id = "pi_standin_" + strconv.Itoa(len(s.intents)+1)
			s.byKey[key] = id
			s.intents[id] = &standInIntent{ID: id, Status: s.answer, Customer: r.FormValue("customer"), Key: key, Reason: s.reason}
			if s.decline != "" {
				s.intents[id].Status, s.intents[id].Reason = "requires_payment_method", s.decline
			}
		}
		if s.lose {
			conn, _, err := rw.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		if s.apiErr {
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(rw, `{"error":{"type":"api_error","message":"An unknown error occurred","payment_intent":{"id":%q}}}`, id)
			return
		}
		if pi := s.intents[id]; s.decline != "" {
			// Stripe's answer to a declined off-session confirm: 402, with
			// the intent it refused nested in the error.
			rw.WriteHeader(http.StatusPaymentRequired)
			fmt.Fprintf(rw, `{"error":{"type":"card_error","code":"card_declined","message":%q,"payment_intent":%s}}`, pi.Reason, s.json(pi))
			return
		}
		fmt.Fprint(rw, s.json(s.intents[id]))
	case r.Method == "GET" && r.URL.Path == "/v1/payment_intents":
		s.lists = append(s.lists, r.URL.RawQuery)
		q := r.URL.Query()
		if s.endless {
			var page []string
			for i := 0; i < 100; i++ {
				page = append(page, fmt.Sprintf(`{"id":"pi_other_%d_%s","status":"succeeded","metadata":{}}`, i, q.Get("starting_after")))
			}
			fmt.Fprintf(rw, `{"data":[%s],"has_more":true}`, strings.Join(page, ","))
			return
		}
		var data []string
		if !s.hidden {
			for _, pi := range s.intents {
				if pi.Customer == q.Get("customer") {
					data = append(data, s.json(pi))
				}
			}
		}
		fmt.Fprintf(rw, `{"data":[%s],"has_more":false}`, strings.Join(data, ","))
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/payment_intents/"):
		id := strings.TrimPrefix(r.URL.Path, "/v1/payment_intents/")
		s.gets = append(s.gets, id)
		pi, ok := s.intents[id]
		if !ok || s.hidden {
			rw.WriteHeader(404)
			fmt.Fprint(rw, `{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such payment_intent"}}`)
			return
		}
		fmt.Fprint(rw, s.json(pi))
	default:
		s.t.Errorf("unexpected provider call %s %s", r.Method, r.URL.Path)
		rw.WriteHeader(500)
	}
}

// set changes what the processor holds, as Stripe finishing its work would.
func (s *stripeStandIn) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

func (s *stripeStandIn) only() *standInIntent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.intents) != 1 {
		s.t.Fatalf("%d intents at the processor, want 1", len(s.intents))
	}
	for _, pi := range s.intents {
		return pi
	}
	return nil
}

func (s *stripeStandIn) postCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts
}

const standInWebhookSecret = "whsec_pass2_standin"

// useStripe points the world's app at the stand-in through the real Stripe
// adapter.
func (w *world) useStripe(st *stripeStandIn) {
	w.t.Helper()
	s, err := billing.NewStripe("sk_test_pass2", "pk_test_pass2", "price_pass2", standInWebhookSecret)
	if err != nil {
		w.t.Fatal(err)
	}
	s.BaseURL, s.HTTP = st.srv.URL, st.srv.Client()
	w.app.Pay = s
}

// webhook delivers a signed PaymentIntent event, as Stripe would.
func (w *world) webhook(id, typ string, pi *standInIntent) int {
	w.t.Helper()
	obj := map[string]any{"id": pi.ID, "object": "payment_intent", "status": pi.Status, "customer": pi.Customer,
		"metadata": map[string]string{"scry_key": pi.Key}, "last_payment_error": map[string]string{"message": pi.Reason}}
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

// postTopup submits the last rendered top-up form and returns where it
// redirected to.
func (b *browser) postTopup() string {
	b.t.Helper()
	form := url.Values{"csrf": {b.csrf}, "topup_token": {b.topup}}
	req, _ := http.NewRequest("POST", b.w.srv.URL+"/billing/topup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := &http.Client{Jar: b.c.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		b.t.Fatalf("POST /billing/topup: %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// onlyAttempt is the learner's single top-up attempt, read straight from SQLite.
func onlyAttempt(t *testing.T, w *world, uid int64) store.TopupAttempt {
	t.Helper()
	var token string
	if err := w.store.DB().QueryRow(`SELECT token FROM topups WHERE user_id=?`, uid).Scan(&token); err != nil {
		t.Fatal(err)
	}
	att, err := w.store.For(uid).Topup(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return att
}

// stripeWorld is a learner on a plan, charged through the Stripe adapter
// against the stand-in, looking at a fresh Billing form.
func stripeWorld(t *testing.T, email string) (*world, *browser, *stripeStandIn, int64) {
	t.Helper()
	w := newWorld(t)
	b := w.browser()
	b.signUp(email)
	st := newStripeStandIn(t)
	w.useStripe(st)
	b.mustGet("/billing")
	return w, b, st, w.user(email).ID
}

// exactlyOneCredit checks the processor took one payment and Scry credited
// it once: one provider POST, one paid ledger line, one top-up of credit.
func exactlyOneCredit(t *testing.T, w *world, st *stripeStandIn, uid int64) {
	t.Helper()
	us := w.store.For(uid)
	sub, _ := us.Subscription(ctx)
	att := onlyAttempt(t, w, uid)
	if st.postCount() != 1 || paidTopups(t, us) != 1 || sub.TopupMicros != w.app.Cfg.TopupMicros || att.Status != store.TopupSucceeded {
		t.Fatalf("posts=%d paid=%d credit=%d status=%s; want one charge credited once", st.postCount(), paidTopups(t, us), sub.TopupMicros, att.Status)
	}
}

// F2 (Pass 2): Stripe created and later succeeded the PaymentIntent, but
// the reply was lost and the intent was not visible for more than the
// settle window. Absence is never taken as proof that nothing was charged:
// the attempt stays pending, and the late success (seen by a later lookup,
// or delivered by webhook) credits exactly once.
func TestF2_LateSuccessAfterLostReplyIsNeverDropped(t *testing.T) {
	for _, via := range []string{"lookup", "webhook"} {
		t.Run(via, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, "late-"+via+"@example.com")
			st.set(func() { st.lose, st.hidden, st.answer = true, true, "processing" })
			if loc := b.postTopup(); loc != "/billing?ok=pending" {
				t.Fatalf("lost reply redirected to %s", loc)
			}
			w.advance(app.TopupSettleWindow + time.Second) // 121 s, still nothing visible
			b.mustGet("/billing")
			w.advance(time.Minute)
			if err := w.app.ReconcileAllTopUps(ctx); err != nil {
				t.Fatal(err)
			}
			att := onlyAttempt(t, w, uid)
			if att.Status != store.TopupPending {
				t.Fatalf("absent after %v: %s, want still pending", w.now().Sub(att.Created), att.Status)
			}
			if n, _ := w.store.For(uid).MonthTopups(ctx, w.now()); n != 1 {
				t.Fatalf("an unconfirmed attempt holds %d cap slots, want 1", n)
			}
			// Resubmitting the form is told the charge is not confirmed yet,
			// and is never charged again.
			if added, err := w.app.TopUpResult(ctx, uid, att.Token); added || !errors.Is(err, app.ErrTopupUnconfirmed) {
				t.Fatalf("resubmitted: %v %v", added, err)
			}
			// Stripe finishes recovering the request: the intent succeeded.
			pi := st.only()
			st.set(func() { pi.Status = "succeeded" })
			if via == "webhook" {
				if code := w.webhook("evt_late_1", "payment_intent.succeeded", pi); code != http.StatusNoContent {
					t.Fatalf("webhook: %d", code)
				}
				exactlyOneCredit(t, w, st, uid)
				// A replay of the event, and a second delivery under a new id,
				// never credit again.
				w.webhook("evt_late_1", "payment_intent.succeeded", pi)
				w.webhook("evt_late_2", "payment_intent.succeeded", pi)
				st.set(func() { st.hidden = false })
			} else {
				st.set(func() { st.hidden = false })
			}
			has(t, b.mustGet("/billing"), "You also have $4 of top-up credit")
			b.mustGet("/billing")
			exactlyOneCredit(t, w, st, uid)
			if added, err := w.app.TopUpResult(ctx, uid, att.Token); added || err != nil {
				t.Fatalf("resubmitted settled form: %v %v", added, err)
			}
			exactlyOneCredit(t, w, st, uid)
		})
	}
}

// F2 (Pass 2): a lookup whose list is still truncated (has_more) after its
// page limit proves nothing. The attempt stays pending, then is credited
// once a complete lookup finds the succeeded intent.
func TestF2_IncompletePaginationStaysRecoverable(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "pages@example.com")
	st.set(func() { st.lose, st.endless = true, true })
	b.postTopup()
	w.advance(app.TopupSettleWindow + time.Second)
	b.mustGet("/billing")
	b.mustGet("/billing")
	if att := onlyAttempt(t, w, uid); att.Status != store.TopupPending {
		t.Fatalf("incomplete scan settled the attempt: %s", att.Status)
	}
	st.mu.Lock()
	lists := len(st.lists)
	st.mu.Unlock()
	if lists < 2*20 {
		t.Fatalf("%d list pages read, want the full page limit on each lookup", lists)
	}
	st.set(func() { st.endless = false })
	has(t, b.mustGet("/billing"), "Topped up.", "You also have $4 of top-up credit")
	exactlyOneCredit(t, w, st, uid)
}

// F2 (Pass 2): the learner's plan moved to a new processor customer (a
// resubscription) while a top-up was pending. Reconciliation looks the
// charge up under the customer recorded on the attempt, never the plan's
// current one, so the original customer's succeeded intent is credited.
func TestF2_CustomerChangedWhilePendingStillCreditsTheOriginalCharge(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "recustomer@example.com")
	us := w.store.For(uid)
	original, _ := us.Subscription(ctx)
	st.set(func() { st.lose = true })
	b.postTopup()
	if att := onlyAttempt(t, w, uid); att.CustomerRef != original.CustomerRef || att.PaymentRef != original.PaymentRef {
		t.Fatalf("attempt recorded %q/%q, want the plan's customer and card at reservation", att.CustomerRef, att.PaymentRef)
	}
	if _, err := us.ChangeSubscription(ctx, func(s *store.Subscription) {
		s.CustomerRef, s.PaymentRef = "cus_replacement", "pm_replacement"
	}); err != nil {
		t.Fatal(err)
	}
	w.advance(app.TopupSettleWindow + time.Second)
	has(t, b.mustGet("/billing"), "You also have $4 of top-up credit")
	exactlyOneCredit(t, w, st, uid)
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, q := range st.lists {
		if strings.Contains(q, "cus_replacement") {
			t.Fatalf("looked the charge up under the new customer: %s", q)
		}
	}
	if pi := st.intents["pi_standin_1"]; pi.Customer != original.CustomerRef {
		t.Fatalf("charged %s", pi.Customer)
	}
}

// F2 (Pass 2): Stripe answered 200 with a PaymentIntent still processing.
// That is not a decline: the intent id is kept, the learner is told the
// top-up is being confirmed, and the intent settles later - to exactly one
// credit when it succeeds (by lookup or by webhook), or to a release with
// no credit when it fails or is canceled.
func TestF2_ProcessingChargeStaysPendingThenSettles(t *testing.T) {
	for _, end := range []string{"succeeded-lookup", "succeeded-webhook", "canceled-webhook", "failed-lookup", "failed-then-succeeded-webhook"} {
		t.Run(end, func(t *testing.T) {
			w, b, st, uid := stripeWorld(t, end+"@example.com")
			st.set(func() { st.answer = "processing" })
			loc := b.postTopup()
			if loc != "/billing?ok=pending" {
				t.Fatalf("processing redirected to %s, want pending", loc)
			}
			page := b.mustGet(loc)
			has(t, page, "being confirmed with the card processor", "charged at most once")
			hasNot(t, page, "declined", "confirm this payment")
			att := onlyAttempt(t, w, uid)
			pi := st.only()
			if att.Status != store.TopupPending || att.ChargeRef != pi.ID {
				t.Fatalf("processing attempt: status=%s charge=%q, want pending with %s", att.Status, att.ChargeRef, pi.ID)
			}
			b.mustGet("/billing") // still processing: still pending
			if att := onlyAttempt(t, w, uid); att.Status != store.TopupPending {
				t.Fatalf("still processing, but %s", att.Status)
			}
			us := w.store.For(uid)
			switch end {
			case "succeeded-lookup":
				st.set(func() { pi.Status = "succeeded" })
				has(t, b.mustGet("/billing"), "Topped up.", "You also have $4 of top-up credit")
				b.mustGet("/billing")
				exactlyOneCredit(t, w, st, uid)
				st.mu.Lock()
				gets, lists := len(st.gets), len(st.lists)
				st.mu.Unlock()
				if gets == 0 || lists != 0 {
					t.Fatalf("lookups by id=%d, lists=%d; a known intent is retrieved directly", gets, lists)
				}
			case "succeeded-webhook":
				st.set(func() { pi.Status = "succeeded" })
				if code := w.webhook("evt_proc_ok", "payment_intent.succeeded", pi); code != http.StatusNoContent {
					t.Fatalf("webhook: %d", code)
				}
				w.webhook("evt_proc_ok", "payment_intent.succeeded", pi)
				b.mustGet("/billing")
				exactlyOneCredit(t, w, st, uid)
			case "canceled-webhook":
				st.set(func() { pi.Status = "canceled" })
				w.webhook("evt_proc_cancel", "payment_intent.canceled", pi)
				att := onlyAttempt(t, w, uid)
				sub, _ := us.Subscription(ctx)
				if n, _ := us.MonthTopups(ctx, w.now()); att.Status != store.TopupFailed || n != 0 || sub.TopupMicros != 0 || paidTopups(t, us) != 0 {
					t.Fatalf("canceled: status=%s slots=%d credit=%d", att.Status, n, sub.TopupMicros)
				}
				if added, err := w.app.TopUpResult(ctx, uid, att.Token); added || !errors.Is(err, app.ErrTopupReleased) || st.postCount() != 1 {
					t.Fatalf("resubmitted canceled form: %v %v posts=%d", added, err, st.postCount())
				}
			case "failed-lookup":
				st.set(func() { pi.Status, pi.Reason = "requires_payment_method", "Your card has insufficient funds." })
				b.mustGet("/billing")
				att := onlyAttempt(t, w, uid)
				sub, _ := us.Subscription(ctx)
				if att.Status != store.TopupFailed || att.Reason != "Your card has insufficient funds." || sub.TopupMicros != 0 || paidTopups(t, us) != 0 {
					t.Fatalf("failed: %+v credit=%d", att, sub.TopupMicros)
				}
			case "failed-then-succeeded-webhook":
				// The processor is authoritative: a success it reports after a
				// failure (it was retried at the processor) is still credited once,
				// and the "card failed" history line gives way to the paid one.
				pi.Reason = "Your card was declined."
				w.webhook("evt_proc_fail", "payment_intent.payment_failed", pi)
				if att := onlyAttempt(t, w, uid); att.Status != store.TopupFailed {
					t.Fatalf("payment_failed: %s", att.Status)
				}
				st.set(func() { pi.Status = "succeeded" })
				w.webhook("evt_proc_ok_late", "payment_intent.succeeded", pi)
				w.webhook("evt_proc_ok_late", "payment_intent.succeeded", pi)
				exactlyOneCredit(t, w, st, uid)
				ledger, _ := us.Ledger(ctx)
				for _, e := range ledger {
					if e.Kind == "topup" && e.Status == "failed" {
						t.Fatalf("a credited top-up still shows as failed: %+v", e)
					}
				}
			}
		})
	}
}

// F2 (Pass 2): a webhook only settles the attempt its PaymentIntent belongs
// to. An intent Scry did not make, or one whose customer does not match
// the attempt's, is acknowledged and ignored.
func TestF2_TopupWebhooksOnlySettleTheirOwnAttempt(t *testing.T) {
	w, b, st, uid := stripeWorld(t, "hookguard@example.com")
	st.set(func() { st.answer = "processing" })
	b.postTopup()
	pi := st.only()
	stranger := &standInIntent{ID: "pi_stranger", Status: "succeeded", Customer: pi.Customer, Key: "topup-999-notours0000000000"}
	if code := w.webhook("evt_stranger", "payment_intent.succeeded", stranger); code != http.StatusNoContent {
		t.Fatalf("unknown intent: %d", code)
	}
	wrongCustomer := *pi
	wrongCustomer.Status, wrongCustomer.Customer = "succeeded", "cus_someone_else"
	w.webhook("evt_wrong_customer", "payment_intent.succeeded", &wrongCustomer)
	invoicePI := &standInIntent{ID: "pi_invoice", Status: "succeeded", Customer: pi.Customer}
	w.webhook("evt_invoice_pi", "payment_intent.succeeded", invoicePI)
	us := w.store.For(uid)
	sub, _ := us.Subscription(ctx)
	if att := onlyAttempt(t, w, uid); att.Status != store.TopupPending || sub.TopupMicros != 0 || paidTopups(t, us) != 0 {
		t.Fatalf("a foreign event settled the attempt: %s credit=%d", att.Status, sub.TopupMicros)
	}
}

// D3 (Pass 2): whether the plan may be charged is decided inside the
// reservation transaction. A second connection holds the SQLite write lock
// while it ends the plan; TopUp waits for TX1, sees the ended plan once the
// writer commits, and refuses: no charge, no credit, no attempt.
func TestD3_PlanEndedWhileTopUpWaitsForTheWriteLockIsRefused(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("race@example.com")
	u := w.user("race@example.com")
	pay := &countingPay{}
	w.app.Pay = pay

	other, err := sql.Open("sqlite", "file:"+w.store.Path()+"?_txlock=immediate&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.Begin() // BEGIN IMMEDIATE: holds the write lock
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE subscriptions SET status='ended' WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := w.app.TopUpResult(ctx, u.ID, app.NewTopupToken()); done <- err }()
	time.Sleep(300 * time.Millisecond) // the pre-check read the old "active"; TX1 now waits for the lock
	select {
	case err := <-done:
		t.Fatalf("top-up finished while the plan's end was uncommitted: %v", err)
	default:
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, app.ErrPlanEnded) {
		t.Fatalf("top-up after the plan ended: %v, want ErrPlanEnded", err)
	}
	var status string
	var credit int64
	var attempts int
	if err := w.store.DB().QueryRow(`SELECT status, topup_micros, (SELECT count(*) FROM topups WHERE user_id=?) FROM subscriptions WHERE user_id=?`,
		u.ID, u.ID).Scan(&status, &credit, &attempts); err != nil {
		t.Fatal(err)
	}
	t.Logf("after the race: %s|%d|%d, charges=%d", status, credit, attempts, pay.total())
	if status != "ended" || credit != 0 || attempts != 0 || pay.total() != 0 {
		t.Fatalf("ended plan was topped up: %s|%d|%d charges=%d", status, credit, attempts, pay.total())
	}
}

// Ruling (Pass 2): "Why?" explains the question and expected answer that
// were actually graded. Editing the card afterwards (say in another tab)
// changes neither the explanation's inputs nor the grade or schedule.
func TestExplain_ExplainsTheGradedAnswerAfterTheCardIsEdited(t *testing.T) {
	w, b, rv, ex := explainWorld(t, "snapshot@example.com", srs.Missed)
	us := w.lib("snapshot@example.com")
	item, _ := us.Item(ctx, rv.ItemID)
	if rv.GradedPrompt != item.Prompt || rv.GradedExpected != item.Answer {
		t.Fatalf("review kept %q/%q, want the graded %q/%q", rv.GradedPrompt, rv.GradedExpected, item.Prompt, item.Answer)
	}
	other := w.browser()
	other.c.Jar = b.c.Jar // the same learner in a second tab
	other.mustGet("/library/items/" + strconv.FormatInt(item.ID, 10))
	other.post("/library/items/"+strconv.FormatInt(item.ID, 10), "prompt", "Replacement prompt", "answer", "Replacement expected answer")
	if edited, _ := us.Item(ctx, item.ID); edited.Answer != "Replacement expected answer" {
		t.Fatalf("edit did not land: %+v", edited)
	}
	status, body := b.explain(rv.ID, true)
	var out struct {
		Text     string
		Fallback bool
	}
	_ = json.Unmarshal([]byte(body), &out)
	if status != 200 || out.Fallback || !strings.Contains(out.Text, "the card expects “"+item.Answer+"”") || strings.Contains(out.Text, "Replacement") {
		t.Fatalf("explained %d %s, want the graded expected answer %q", status, body, item.Answer)
	}
	if ex.Calls() != 1 {
		t.Fatalf("explainer calls = %d", ex.Calls())
	}
	after, _ := us.Review(ctx, rv.ID)
	edited, _ := us.Item(ctx, item.ID)
	if !reflect.DeepEqual(after, rv) || !reflect.DeepEqual(edited.Sched, item.Sched) {
		t.Fatalf("explaining changed the record:\n%+v\n%+v", rv, after)
	}
}

// A review recorded before graded inputs were kept cannot know whether its
// card was edited since, so "Why?" gives the verdict's own sentence.
func TestExplain_LegacyReviewWithoutSnapshotFallsBack(t *testing.T) {
	w, b, rv, ex := explainWorld(t, "legacy@example.com", srs.Close)
	if _, err := w.store.DB().Exec(`UPDATE reviews SET graded_prompt='', graded_expected='' WHERE id=?`, rv.ID); err != nil {
		t.Fatal(err)
	}
	_, body := b.explain(rv.ID, true)
	var out struct {
		Text     string
		Fallback bool
	}
	_ = json.Unmarshal([]byte(body), &out)
	if !out.Fallback || out.Text != ai.CriterionSentence(srs.Close) || ex.Calls() != 0 {
		t.Fatalf("legacy review: %s, calls=%d", body, ex.Calls())
	}
}
