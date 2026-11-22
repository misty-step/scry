package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Slice 36 pass 3: FindSubscription reports absence only after a complete
// lookup, follows pages, matches only its own key, prefers an open
// subscription, and never guesses at a status it does not know.
func TestSlice36_StripeFindSubscription(t *testing.T) {
	cus := func(id, key string) string { return fmt.Sprintf(`{"id":%q,"metadata":{"scry_key":%q}}`, id, key) }
	sub := func(id, key, status string) string {
		return fmt.Sprintf(`{"id":%q,"status":%q,"metadata":{"scry_key":%q},"current_period_start":100,"current_period_end":200,"latest_invoice":{"id":"in_%s"}}`, id, status, key, id)
	}
	page := func(more bool, data string) string {
		return fmt.Sprintf(`{"object":"list","has_more":%v,"data":[%s]}`, more, data)
	}
	for _, tc := range []struct {
		name      string
		customers map[string]string // by starting_after
		subs      string
		state     SubscriptionState
		ref       string
		fails     bool
	}{
		{"none", map[string]string{"": page(false, "")}, "", SubscriptionNotFound, "", false},
		{"another attempt's customer", map[string]string{"": page(false, cus("cus_1", "other"))}, page(false, sub("sub_1", "k", "active")), SubscriptionNotFound, "", false},
		{"customer, no subscription", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, ""), SubscriptionNotFound, "", false},
		{"second page", map[string]string{"": page(true, cus("cus_0", "other")), "cus_0": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "active")), SubscriptionPaid, "sub_1", false},
		{"trialing", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "trialing")), SubscriptionTrialing, "sub_1", false},
		{"past due", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "past_due")), SubscriptionUnpaid, "sub_1", false},
		{"over", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "canceled")), SubscriptionOver, "sub_1", false},
		{"open preferred", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "canceled")+","+sub("sub_2", "k", "active")), SubscriptionPaid, "sub_2", false},
		{"unknown status", map[string]string{"": page(false, cus("cus_1", "k"))}, page(false, sub("sub_1", "k", "new_status")), "", "", true},
		{"not a list", map[string]string{"": `{"id":"cus_1"}`}, "", "", "", true},
		{"truncated", map[string]string{"": page(true, "")}, "", "", "", true},
		{"subscriptions not a list", map[string]string{"": page(false, cus("cus_1", "k"))}, `{}`, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				switch {
				case r.Method != http.MethodGet:
					t.Errorf("a lookup sent %s %s", r.Method, r.URL)
				case r.URL.Path == "/v1/customers":
					if q.Get("email") != "a@example.com" || q.Get("created[gte]") != "400" {
						t.Errorf("customers query %v", q)
					}
					fmt.Fprint(w, tc.customers[q.Get("starting_after")])
				case r.URL.Path == "/v1/subscriptions":
					if q.Get("customer") != "cus_1" || q.Get("status") != "all" {
						t.Errorf("subscriptions query %v", q)
					}
					fmt.Fprint(w, tc.subs)
				default:
					fmt.Fprint(w, `{"card":{"brand":"visa","last4":"4242"}}`)
				}
			}))
			defer srv.Close()
			s, err := NewStripe("sk_test_x", "pk_test_x", "price_x", "whsec_x")
			if err != nil {
				t.Fatal(err)
			}
			s.BaseURL = srv.URL
			state, out, err := s.FindSubscription(context.Background(), SubscribeParams{Email: "a@example.com", CardToken: "pm_1", Key: "k"}, time.Unix(1000, 0))
			if (err != nil) != tc.fails || state != tc.state || out.SubscriptionRef != tc.ref {
				t.Fatalf("%q %+v %v", state, out, err)
			}
			if tc.state == SubscriptionPaid && (out.CustomerRef != "cus_1" || out.ChargeRef != "in_"+tc.ref || out.PaymentRef != "pm_1" || out.Card.Last4 != "4242" || out.PeriodEnd.Unix() != 200) {
				t.Fatalf("%+v", out)
			}
		})
	}
}

// Slice 36 pass 3: only a definitive answer says a Subscribe did nothing.
func TestSlice36_NothingDone(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&DeclineError{Reason: "no"}, true},
		{fmt.Errorf("wrapped: %w", &DeclineError{Reason: "no"}), true},
		{&CanceledError{}, true},
		{&ProcessorError{Status: 400, Type: "invalid_request_error"}, true},
		{&ProcessorError{Status: 400, Type: "idempotency_error"}, false},
		{&ProcessorError{Status: 409, Type: "idempotency_error"}, false},
		{&ProcessorError{Status: 401, Type: "invalid_request_error"}, false},
		{&ProcessorError{Status: 429, Type: "invalid_request_error"}, false},
		{&ProcessorError{Status: 500, Type: "api_error"}, false},
		{&ProcessorError{Status: 404}, false}, // a proxy, not the processor
		{&PendingError{}, false},
		{errors.New("card processor unreachable: timeout"), false},
	} {
		if got := NothingDone(tc.err); got != tc.want {
			t.Errorf("NothingDone(%#v) = %v", tc.err, got)
		}
	}
}
