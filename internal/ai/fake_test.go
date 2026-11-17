package ai

import (
	"context"
	"scry/internal/srs"
	"testing"
)

func TestRule_TestJudgeIsScriptedNotAHeuristic(t *testing.T) {
	f := &Fake{}
	for _, answer := range []string{"Paris", "not Paris", "", "unrelated"} {
		j, _, err := f.Judge(context.Background(), JudgeInput{Expected: "Paris", Answer: answer})
		if err != nil || j.Verdict != srs.Right || j.Reason != "Test fixture: right (not an assessment of your answer)." {
			t.Fatalf("%+v %v", j, err)
		}
	}
	f.Judgment = &Judgment{Verdict: srs.Missed, Reason: "Scripted miss."}
	j, _, _ := f.Judge(context.Background(), JudgeInput{Expected: "Paris", Answer: "Paris"})
	if j.Verdict != srs.Missed {
		t.Fatal(j)
	}
}
