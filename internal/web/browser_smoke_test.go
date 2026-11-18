package web_test

import (
	"os"
	"os/exec"
	"scry/internal/ai"
	"scry/internal/srs"
	"testing"
)

func TestReviewBrowserSmoke(t *testing.T) {
	if os.Getenv("SCRY_BROWSER_SMOKE") != "1" {
		t.Skip("set SCRY_BROWSER_SMOKE=1 and install puppeteer-core")
	}
	w := newWorld(t)
	w.model.script = []ai.Judgment{
		{Verdict: srs.Right, Reason: "Your answer expresses the same idea."},
		{Verdict: srs.Missed, Reason: "The expected answer is different."},
	}
	cmd := exec.Command("node", "../../scripts/browser-smoke.cjs", w.srv.URL)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
	w.model.mu.Lock()
	defer w.model.mu.Unlock()
	if len(w.model.judged) != 2 {
		t.Fatalf("wanted two submissions, got %d", len(w.model.judged))
	}
	var calls int
	if err := w.store.DB().QueryRow("SELECT count(*) FROM usage WHERE kind='speech'").Scan(&calls); err != nil || calls != 1 {
		t.Fatalf("speech metering calls=%d err=%v", calls, err)
	}
}
