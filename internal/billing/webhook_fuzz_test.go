package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fuzzWebhookSecret = "whsec_fuzz"

// offlineTransport fails every request at once: ParseWebhook's one lookup
// (a failed invoice's PaymentIntent) must cope without the network.
type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline")
}

func fuzzStripe(t testing.TB) *Stripe {
	s, err := NewStripe("sk_test_fuzz", "pk_test_fuzz", "price_fuzz", fuzzWebhookSecret)
	if err != nil {
		t.Fatal(err)
	}
	s.BaseURL = "http://stripe.invalid"
	s.HTTP = &http.Client{Transport: offlineTransport{}}
	return s
}

func fuzzSign(stamp string, payload []byte) string {
	m := hmac.New(sha256.New, []byte(fuzzWebhookSecret))
	m.Write([]byte(stamp + "."))
	m.Write(payload)
	return hex.EncodeToString(m.Sum(nil))
}

var fuzzNow = time.Unix(1_800_000_000, 0)

// webhookSeeds are real-shaped Stripe events for every type the adapter
// understands, plus some it does not.
var webhookSeeds = []string{
	`{"id":"evt_1","type":"invoice.paid","created":1800000000,"livemode":false,"data":{"object":{"id":"in_1","subscription":"sub_1","amount_paid":500,"lines":{"data":[{"period":{"start":1800000000,"end":1802592000}}]}}}}`,
	`{"id":"evt_2","type":"invoice.paid","created":1800000000,"livemode":false,"data":{"object":{"id":"in_2","subscription":"sub_1","amount_paid":0}}}`,
	`{"id":"evt_3","type":"invoice.payment_failed","created":1800000000,"data":{"object":{"id":"in_3","subscription":"sub_1","amount_due":500,"payment_intent":"pi_3"}}}`,
	`{"id":"evt_4","type":"customer.subscription.deleted","created":1800000000,"data":{"object":{"id":"sub_1"}}}`,
	`{"id":"evt_5","type":"payment_intent.succeeded","created":1800000000,"data":{"object":{"id":"pi_5","customer":"cus_1","metadata":{"scry_key":"topup-1-abc"}}}}`,
	`{"id":"evt_6","type":"payment_intent.payment_failed","created":1800000000,"data":{"object":{"id":"pi_6","customer":"cus_1","metadata":{},"last_payment_error":{"message":"Your card has insufficient funds."}}}}`,
	`{"id":"evt_7","type":"payment_intent.canceled","created":1800000000,"data":{"object":{"id":"pi_7","metadata":null}}}`,
	`{"id":"evt_8","type":"payment_intent.succeeded","created":1800000000,"data":{"object":{"id":"","metadata":{"scry_key":"topup-1-abc"}}}}`,
	`{"id":"evt_9","type":"payment_intent.succeeded","livemode":true,"data":{"object":{"id":"pi_9"}}}`,
	`{"id":"evt_10","type":"charge.refunded","data":{"object":{"id":"ch_1"}}}`,
	`{}`, `null`, `[]`, `{"type":1}`, ``,
}

// livemodeOf reports how encoding/json reads the event's livemode flag,
// independently of the adapter's own struct.
func livemodeOf(payload []byte) (live, ok bool) {
	var probe struct {
		Livemode bool `json:"livemode"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil {
		return false, false
	}
	return probe.Livemode, true
}

// checkEvents holds for whatever a verified payload decodes to.
func checkEvents(t *testing.T, payload []byte, evs []Event) {
	t.Helper()
	if len(evs) > 1 {
		t.Fatalf("one webhook produced %d events", len(evs))
	}
	if live, ok := livemodeOf(payload); ok && live && len(evs) > 0 {
		t.Fatalf("live-mode webhook accepted: %s", payload)
	}
	for _, ev := range evs {
		switch ev.Kind {
		case TopupPaid, TopupFailed:
			// Never a top-up settlement without the processor's charge id.
			if ev.ChargeRef == "" {
				t.Fatalf("top-up event with no charge id: %+v", ev)
			}
		case InvoicePaid:
			if ev.SubscriptionRef == "" || ev.AmountCents == 0 {
				t.Fatalf("invoice paid with no subscription or amount: %+v", ev)
			}
		case InvoiceFailed:
			if ev.SubscriptionRef == "" || ev.Reason == "" {
				t.Fatalf("invoice failure with no subscription or reason: %+v", ev)
			}
		case SubscriptionEnded:
		default:
			t.Fatalf("unknown event kind %q", ev.Kind)
		}
	}
}

// FuzzStripeParseEvent feeds arbitrary payloads, correctly signed, to
// ParseWebhook: it never panics, never accepts a live-mode event, never
// produces a top-up settlement without a charge id, and the same payload
// with its signature broken, its timestamp stale, or its body altered by
// one byte is always refused.
func FuzzStripeParseEvent(f *testing.F) {
	for _, seed := range webhookSeeds {
		f.Add([]byte(seed), int64(0), byte(0))
	}
	f.Add([]byte(webhookSeeds[4]), int64(301), byte(1))
	f.Add([]byte(webhookSeeds[0]), int64(-301), byte(255))
	s := fuzzStripe(f)
	ctx := context.Background()
	f.Fuzz(func(t *testing.T, payload []byte, skew int64, flip byte) {
		stamp := strconv.FormatInt(fuzzNow.Unix(), 10)
		good := "t=" + stamp + ",v1=" + fuzzSign(stamp, payload)
		evs, err := s.ParseWebhook(ctx, payload, good, fuzzNow)
		if err == nil {
			checkEvents(t, payload, evs)
		} else if evs != nil {
			t.Fatalf("events returned with an error: %v", err)
		}
		if err == nil && !json.Valid(payload) {
			t.Fatalf("invalid JSON accepted: %q", payload)
		}

		// A wrong signature, under the right timestamp, is refused.
		bad := []byte(fuzzSign(stamp, payload))
		bad[int(flip)%len(bad)] ^= 1
		if _, err := s.ParseWebhook(ctx, payload, "t="+stamp+",v1="+string(bad), fuzzNow); err == nil {
			t.Fatal("a corrupted signature was accepted")
		}
		// The body changed by one byte under the original signature is refused.
		if len(payload) > 0 {
			altered := bytes.Clone(payload)
			altered[int(flip)%len(altered)] ^= 1 | flip
			if _, err := s.ParseWebhook(ctx, altered, good, fuzzNow); err == nil {
				t.Fatal("an altered body was accepted under the original signature")
			}
		}
		// A correctly signed but stale (or future) timestamp is refused.
		if skew < 0 {
			skew = -skew
		}
		if skew > 0 {
			off := time.Duration(skew%1_000_000+int64(webhookTolerance/time.Second)+1) * time.Second
			for _, at := range []time.Time{fuzzNow.Add(-off), fuzzNow.Add(off)} {
				st := strconv.FormatInt(at.Unix(), 10)
				if _, err := s.ParseWebhook(ctx, payload, "t="+st+",v1="+fuzzSign(st, payload), fuzzNow); err == nil {
					t.Fatalf("a webhook signed %v away was accepted", at.Sub(fuzzNow))
				}
			}
		}
	})
}

// FuzzStripeVerifySignature feeds arbitrary signature headers, payloads
// and clock offsets to the verifier. It never panics, and it accepts a
// header only if some v1 entry really is the HMAC of "t.payload" under the
// webhook secret, for a t parsed from the header that is inside the
// tolerance of now.
func FuzzStripeVerifySignature(f *testing.F) {
	stamp := strconv.FormatInt(fuzzNow.Unix(), 10)
	body := []byte(webhookSeeds[4])
	f.Add(body, "t="+stamp+",v1="+fuzzSign(stamp, body), int64(0))
	f.Add(body, "t="+stamp+",v1=deadbeef,v1="+fuzzSign(stamp, body), int64(10))
	f.Add(body, "v1="+fuzzSign(stamp, body)+",t="+stamp, int64(-299))
	f.Add(body, "t="+stamp+",v0="+fuzzSign(stamp, body), int64(0))
	f.Add(body, "t= "+stamp+" , v1 = "+fuzzSign(stamp, body), int64(0))
	f.Add([]byte(""), "t=,v1=", int64(0))
	f.Add([]byte("{}"), "t=99999999999999999999,v1=00", int64(0))
	f.Add([]byte("{}"), "garbage", int64(400))
	s := fuzzStripe(f)
	f.Fuzz(func(t *testing.T, payload []byte, header string, offset int64) {
		now := fuzzNow.Add(time.Duration(offset%100_000) * time.Second)
		err := s.verify(payload, header, now)
		if err != nil {
			return
		}
		// Accepted: re-derive why, independently of verify.
		var stamps []string
		var sigs []string
		for _, part := range strings.Split(header, ",") {
			k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
			switch k {
			case "t":
				stamps = append(stamps, v)
			case "v1":
				sigs = append(sigs, v)
			}
		}
		if len(stamps) == 0 {
			t.Fatalf("accepted a header with no timestamp: %q", header)
		}
		st := stamps[len(stamps)-1] // the last t= wins, as in verify
		unix, perr := strconv.ParseInt(st, 10, 64)
		if perr != nil {
			t.Fatalf("accepted an unparsable timestamp %q", st)
		}
		if d := now.Sub(time.Unix(unix, 0)); d > webhookTolerance || d < -webhookTolerance {
			t.Fatalf("accepted a timestamp %v away", d)
		}
		want := fuzzSign(st, payload)
		for _, sig := range sigs {
			if strings.EqualFold(sig, want) {
				return
			}
		}
		t.Fatalf("accepted header %q with no matching v1 signature", header)
	})
}
