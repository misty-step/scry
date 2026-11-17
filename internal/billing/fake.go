package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Fake is the test-mode processor. It needs no keys, keeps no state, and
// behaves like Stripe's test cards:
//
//	4242 4242 4242 4242  works
//	5555 5555 5555 4444  works (Mastercard)
//	4000 0000 0000 0002  declined
//	4000 0000 0000 9995  insufficient funds
//	4000 0000 0000 0069  expired card
//	4000 0000 0000 0127  incorrect security code
//	4000 0000 0000 0341  attaches, then every charge is declined
//
// Any other number is refused, so a real card can never be used in test
// mode. How a card behaves is carried in its token.
type Fake struct{}

func (Fake) Name() string           { return "test" }
func (Fake) PublishableKey() string { return "" }

var testCards = map[string]struct{ behavior, brand, reason string }{
	"4242424242424242": {"ok", "Visa", ""},
	"5555555555554444": {"ok", "Mastercard", ""},
	"4000000000000002": {"decline", "Visa", "Your card was declined."},
	"4000000000009995": {"decline", "Visa", "Your card has insufficient funds."},
	"4000000000000069": {"decline", "Visa", "Your card has expired."},
	"4000000000000127": {"decline", "Visa", "Your card's security code is incorrect."},
	"4000000000000341": {"chargefail", "Visa", ""},
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func luhn(n string) bool {
	if len(n) < 12 {
		return false
	}
	sum, alt := 0, false
	for i := len(n) - 1; i >= 0; i-- {
		d := int(n[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

func (Fake) Tokenize(number, expiry, cvc string, now time.Time) (string, error) {
	n := digits(number)
	if n == "" {
		return "", &DeclineError{"Enter your card number."}
	}
	if !luhn(n) {
		return "", &DeclineError{"Your card number is invalid."}
	}
	exp := digits(expiry)
	if len(exp) != 4 && len(exp) != 6 {
		return "", &DeclineError{"Your card's expiration date is incomplete."}
	}
	month, _ := strconv.Atoi(exp[:2])
	year, _ := strconv.Atoi(exp[2:])
	if year < 100 {
		year += 2000
	}
	if month < 1 || month > 12 {
		return "", &DeclineError{"Your card's expiration month is invalid."}
	}
	if year < now.Year() || (year == now.Year() && month < int(now.Month())) {
		return "", &DeclineError{"Your card's expiration date is in the past."}
	}
	if c := digits(cvc); len(c) < 3 || len(c) > 4 {
		return "", &DeclineError{"Your card's security code is incomplete."}
	}
	card, ok := testCards[n]
	if !ok {
		return "", &DeclineError{"Scry is in test mode. Use a test card such as 4242 4242 4242 4242."}
	}
	if card.behavior == "decline" {
		return "", &DeclineError{card.reason}
	}
	return fmt.Sprintf("tok_test_%s_%s_%s", card.behavior, card.brand, n[len(n)-4:]), nil
}

func parseToken(tok string) (behavior string, card Card, err error) {
	parts := strings.Split(tok, "_")
	if len(parts) != 5 || parts[0] != "tok" || parts[1] != "test" {
		return "", Card{}, errors.New("not a test card token")
	}
	return parts[2], Card{Brand: parts[3], Last4: parts[4]}, nil
}

func ref(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func (Fake) Subscribe(_ context.Context, p SubscribeParams) (Subscribed, error) {
	behavior, card, err := parseToken(p.CardToken)
	if err != nil {
		return Subscribed{}, err
	}
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := Subscribed{CustomerRef: ref("cus_test_"), SubscriptionRef: ref("sub_test_"), PaymentRef: p.CardToken, Card: card, PeriodStart: now}
	if p.TrialEnd.IsZero() {
		if behavior == "chargefail" {
			return Subscribed{}, &DeclineError{"Your card was declined."}
		}
		out.PeriodEnd = now.AddDate(0, 1, 0)
		out.ChargeRef = ref("ch_test_")
	} else {
		out.PeriodEnd = p.TrialEnd
	}
	return out, nil
}

func (Fake) UpdateCard(_ context.Context, _, _, cardToken string) (string, Card, error) {
	_, card, err := parseToken(cardToken)
	return cardToken, card, err
}

func (Fake) Charge(_ context.Context, _, paymentRef string, _ int64, _, _ string) (string, error) {
	behavior, _, err := parseToken(paymentRef)
	if err != nil {
		return "", err
	}
	if behavior == "chargefail" {
		return "", &DeclineError{"Your card was declined."}
	}
	return ref("ch_test_"), nil
}

func (Fake) SetCancelAtPeriodEnd(context.Context, string, bool) error { return nil }

func (Fake) ParseWebhook(context.Context, []byte, string, time.Time) ([]Event, error) {
	return nil, errors.New("the test processor sends no webhooks")
}

// graceDays is how long the test processor retries a failed card before
// it gives up and ends the plan.
const graceDays = 7

// Renew says what a processor would have done to a plan whose period ran
// out: end it if it was cancelled, charge it, or report the failure.
func (Fake) Renew(subscriptionRef, paymentRef string, cancelAtEnd, pastDue bool, periodEnd time.Time, priceCents int64, now time.Time) Event {
	ev := Event{SubscriptionRef: subscriptionRef, At: now, AmountCents: priceCents}
	stamp := strconv.FormatInt(periodEnd.Unix(), 10)
	behavior, _, _ := parseToken(paymentRef)
	switch {
	case cancelAtEnd:
		ev.ID, ev.Kind = "evt_test_end_"+subscriptionRef+"_"+stamp, SubscriptionEnded
	case behavior == "chargefail" && pastDue && now.Sub(periodEnd) > graceDays*24*time.Hour:
		ev.ID, ev.Kind = "evt_test_end_"+subscriptionRef+"_"+stamp, SubscriptionEnded
	case behavior == "chargefail":
		ev.ID, ev.Kind, ev.Reason = "evt_test_fail_"+subscriptionRef+"_"+stamp, InvoiceFailed, "Your card was declined."
	default:
		ev.ID, ev.Kind = "evt_test_paid_"+subscriptionRef+"_"+stamp, InvoicePaid
		ev.PeriodStart, ev.PeriodEnd = periodEnd, periodEnd.AddDate(0, 1, 0)
		if ev.PeriodEnd.Before(now) { // the app was off for more than a period
			ev.PeriodStart, ev.PeriodEnd = now, now.AddDate(0, 1, 0)
		}
	}
	return ev
}
