package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestG11_StripeSignatures(t *testing.T) {
	s, err := NewStripe("sk_test_local", "pk_test_local", "price_local", "whsec_local")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	sign := func(body []byte, at time.Time) string {
		stamp := strconv.FormatInt(at.Unix(), 10)
		m := hmac.New(sha256.New, []byte("whsec_local"))
		m.Write([]byte(stamp + "."))
		m.Write(body)
		return "t=" + stamp + ",v1=bad,v1=" + hex.EncodeToString(m.Sum(nil))
	}
	body := []byte(`{"id":"evt_local","type":"invoice.paid","livemode":false,"data":{"object":{"subscription":"sub_local","amount_paid":500}}}`)
	for _, tc := range []struct {
		name string
		body []byte
		sig  string
		good bool
	}{
		{"valid", body, sign(body, now), true}, {"tampered", append(append([]byte{}, body...), byte(' ')), sign(body, now), false},
		{"expired", body, sign(body, now.Add(-6*time.Minute)), false}, {"future", body, sign(body, now.Add(6*time.Minute)), false}, {"missing", body, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := s.ParseWebhook(context.Background(), tc.body, tc.sig, now)
			if (err == nil) != tc.good {
				t.Fatalf("err %v", err)
			}
			if tc.good && (len(ev) != 1 || ev[0].AmountCents != 500) {
				t.Fatal(ev)
			}
		})
	}
	live := []byte(`{"id":"evt_live","livemode":true}`)
	if _, err := s.ParseWebhook(context.Background(), live, sign(live, now), now); err == nil {
		t.Fatal("accepted live webhook")
	}
}
