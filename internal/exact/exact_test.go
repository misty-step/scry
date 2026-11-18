package exact

import (
	"strings"
	"testing"
)

const line1 = "I believe in one God, the Father Almighty,"

func kinds(r Result) string {
	var b []string
	for _, o := range r.Ops {
		b = append(b, string(o.Kind))
	}
	return strings.Join(b, " ")
}

func TestPerfectIgnoresCaseAndPunctuation(t *testing.T) {
	r := Diff(line1, "i believe in one god the father almighty")
	if !r.Perfect() || r.Verdict() != "right" || r.Why() != "Word for word." {
		t.Fatalf("got %+v %q", r, r.Why())
	}
}

func TestOneMissingOneSwapped(t *testing.T) {
	// "one" is left out; "Lord" stands where "Father" belongs.
	r := Diff(line1, "I believe in God, the Lord Almighty")
	if r.Missing != 1 || r.Swapped != 1 || r.Extra != 0 {
		t.Fatalf("counts %+v", r)
	}
	if got := r.Why(); got != "One word missing, one swapped." {
		t.Fatalf("why = %q", got)
	}
	if got := kinds(r); got != "same same same missing same same wrong same" {
		t.Fatalf("ops = %s", got)
	}
	for _, o := range r.Ops {
		if o.Kind == Wrong && (o.Word != "Lord" || o.Fix != "Father") {
			t.Fatalf("swap = %+v", o)
		}
		if o.Kind == Missing && o.Word != "one" {
			t.Fatalf("missing = %+v", o)
		}
	}
	if r.Verdict() != "missed" {
		t.Fatalf("verdict = %s", r.Verdict())
	}
}

func TestExtraAndMissed(t *testing.T) {
	r := Diff("Alfa Bravo Charlie", "Alfa really Bravo Charlie")
	if r.Extra != 1 || r.Why() != "One extra word." {
		t.Fatalf("%+v %q", r, r.Why())
	}
	r = Diff(line1, "something else entirely")
	if r.Verdict() != "missed" {
		t.Fatalf("verdict = %s (%+v)", r.Verdict(), r)
	}
	r = Diff(line1, "")
	if r.Missing != 8 || r.Why() != "Eight words missing." || r.Verdict() != "missed" {
		t.Fatalf("%+v %q", r, r.Why())
	}
	r = Diff("a b c d", "a x y d z")
	if r.Why() != "Two words swapped, one extra." {
		t.Fatalf("why = %q", r.Why())
	}
}
