package deck

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fixtures is the sample deck directory at the repository root, used by
// the browser walkthrough of S106-3. Regenerate sample.apkg with
//
//	SCRY_WRITE_DECK_FIXTURES=1 go test ./internal/deck -run TestS106_3_SampleFixtures
var fixtures = filepath.Join("..", "..", "testdata", "decks")

func TestS106_3_SampleFixtures(t *testing.T) {
	if os.Getenv("SCRY_WRITE_DECK_FIXTURES") == "1" {
		if err := os.WriteFile(filepath.Join(fixtures, "sample.apkg"), apkg(t, sampleFixture), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	kind, err := KindOf("sample.apkg")
	if err != nil || kind != KindAPKG {
		t.Fatal(kind, err)
	}
	cards, err := ParseAPKG(read("sample.apkg"))
	if err != nil || !reflect.DeepEqual(cards, sampleCards) {
		t.Fatalf("sample.apkg: %v\n got %+v\nwant %+v", err, cards, sampleCards)
	}

	cards, err = ParseDelimited(read("sample.csv"), ',')
	wantCSV := []Card{
		{Idea: "Spanish basics", Question: `What is "cat" in Spanish?`, Answer: "gato"},
		{Idea: "Spanish basics", Question: "How do you say thank you?", Answer: "gracias"},
		{Idea: "Spanish basics", Question: "Count to three, in Spanish.", Answer: "uno, dos, tres", Exact: true},
		{Idea: "World capitals", Question: "What is the capital of Australia?", Answer: "Canberra"},
		{Idea: "", Question: "Which planet is closest to the Sun?", Answer: "Mercury"},
	}
	if err != nil || !reflect.DeepEqual(cards, wantCSV) {
		t.Fatalf("sample.csv: %v\n got %+v", err, cards)
	}

	cards, structured, err := ParseMarkdown(string(read("sample.md")))
	if err != nil || !structured || len(cards) != 3 || cards[0].Idea != "Memory science" ||
		cards[1] != (Card{Idea: "Memory science", Question: "Who first measured the forgetting curve?", Answer: "Hermann Ebbinghaus, in 1885."}) {
		t.Fatalf("sample.md: %v %v %+v", structured, err, cards)
	}

	cards, structured, err = ParseMarkdown(string(read("notes.md")))
	if err != nil || structured || cards != nil {
		t.Fatalf("notes.md should go to drafting: %v %v %+v", structured, err, cards)
	}
}
