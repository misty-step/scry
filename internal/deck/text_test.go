package deck

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestS106_3_KindOf(t *testing.T) {
	for name, want := range map[string]string{
		"Spanish.apkg": KindAPKG, "DECK.APKG": KindAPKG, "cards.csv": KindCSV, "cards.CSV": KindCSV,
		"cards.tsv": KindTSV, "notes.md": KindMD, "Notes.Markdown": KindMD,
	} {
		if got, err := KindOf(name); err != nil || got != want {
			t.Errorf("KindOf(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	for _, name := range []string{"photo.png", "deck.colpkg", "README", "book.pdf"} {
		if _, err := KindOf(name); err == nil || !strings.HasSuffix(err.Error(), ".") {
			t.Errorf("KindOf(%q) err = %v; want a sentence", name, err)
		}
	}
}

func TestS106_3_PlainTextFromHTML(t *testing.T) {
	for in, want := range map[string]string{
		`<b>Bold</b> &amp; plain`:                              "Bold & plain",
		`one<br>two<br/>three`:                                 "one\ntwo\nthree",
		`<div>a</div><div>b</div>`:                             "a\nb",
		`x <img src="cat.png" alt="a > b"> y`:                  "x y",
		`hear [sound:word.mp3] this`:                           "hear this",
		`a &lt; b &nbsp; c`:                                    "a < b c",
		`1 < 2 and 3 > 2`:                                      "1 < 2 and 3 > 2",
		`<style>p{color:red}</style>shown<script>x()</script>`: "shown",
		`<!-- hidden -->kept`:                                  "kept",
		`<ul><li>one</li><li>two</li></ul>`:                    "one\ntwo",
		`<table><tr><td>a</td><td>b</td></tr></table>`:         "a b",
		"  spaced   \n\n\n  out  ":                             "spaced\nout",
		`&lt;b&gt;not a tag&lt;/b&gt;`:                         "<b>not a tag</b>",
	} {
		if got := PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestS106_3_CSVHeaderOptional(t *testing.T) {
	want2 := []Card{{Question: "What is 2+2?", Answer: "4"}, {Question: "Capital of France?", Answer: "Paris"}}
	for name, in := range map[string]string{
		"no header":        "What is 2+2?,4\nCapital of France?,Paris\n",
		"header":           "question,answer\nWhat is 2+2?,4\nCapital of France?,Paris\n",
		"header any case":  "Question,ANSWER\r\nWhat is 2+2?,4\r\nCapital of France?,Paris\r\n",
		"bom and blanks":   "\uFEFFquestion,answer\n\nWhat is 2+2?,4\n , \n\nCapital of France?,Paris",
		"front/back":       "front,back\nWhat is 2+2?,4\nCapital of France?,Paris\n",
		"trailing columns": "What is 2+2?,4,,\nCapital of France?,Paris,\n",
	} {
		got, err := ParseDelimited([]byte(in), ',')
		if err != nil || !reflect.DeepEqual(got, want2) {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}

	want4 := []Card{
		{Idea: "Maths", Question: "What is 2+2?", Answer: "4"},
		{Idea: "", Question: "Pi to 5 places?", Answer: "3.14159", Exact: true},
		{Idea: "Poems", Question: "First line?", Answer: "Shall I compare thee\nto a summer's day?", Exact: true},
	}
	for name, in := range map[string]string{
		"no header": "Maths,What is 2+2?,4,\n,Pi to 5 places?,3.14159,yes\nPoems,First line?,\"Shall I compare thee\nto a summer's day?\",exact\n",
		"header":    "idea,question,answer,exact\nMaths,What is 2+2?,4,false\n,Pi to 5 places?,3.14159,TRUE\nPoems,First line?,\"Shall I compare thee\nto a summer's day?\",1\n",
		"reordered": "exact,answer,question,idea\nno,4,What is 2+2?,Maths\ny,3.14159,Pi to 5 places?,\ntrue,\"Shall I compare thee\nto a summer's day?\",First line?,Poems\n",
	} {
		got, err := ParseDelimited([]byte(in), ',')
		if err != nil || !reflect.DeepEqual(got, want4) {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}

	got, err := ParseDelimited([]byte("Maths\tWhat is 2+2?\t4\nQuotes\tWho said \"hi\"?\tShe did\n"), '\t')
	want := []Card{{Idea: "Maths", Question: "What is 2+2?", Answer: "4"}, {Idea: "Quotes", Question: `Who said "hi"?`, Answer: "She did"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("tsv: got %+v, %v", got, err)
	}
}

func TestS106_3_CSVRefusedInWords(t *testing.T) {
	long := strings.Repeat("a", MaxAnswerBytes+1)
	for in, want := range map[string]string{
		"q,a\njust one\n":                   "Row 2 has only one column.",
		"q,a\n1,2,3,4,5\n":                  "Row 2 has 5 columns.",
		"q,a\nq2,a2\nq3," + long + "\n":     "Row 3's answer is longer than 2,000 characters.",
		strings.Repeat("x", 1001) + ",a\n":  "Row 1's question is longer than 1,000 characters.",
		strings.Repeat("x", 121) + ",q,a\n": "Row 1's idea title is longer than 120 characters.",
		"idea,q,a,maybe\n":                  `Row 1's exact column says "maybe". Use true or false.`,
		"question,answer\nq,\n":             "Row 2 has no answer.",
		"question,answer\n":                 "That file has no cards in it.",
		"q,\"unclosed\n":                    "Row 1 isn't valid CSV: check its quotes.",
		"question,answer\nq,a,extra\n":      "Row 2 has 3 columns, but the header names 2.",
		"q,a\n\xff\xfe,bad\n":               "That file isn't UTF-8 text.",
	} {
		_, err := ParseDelimited([]byte(in), ',')
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%.40q: err = %v; want %q", in, err, want)
		}
	}
	var many strings.Builder
	for i := 0; i <= MaxCards; i++ {
		fmt.Fprintf(&many, "q%d,a\n", i)
	}
	if _, err := ParseDelimited([]byte(many.String()), ','); err == nil || !strings.Contains(err.Error(), "more than 5,000 cards") {
		t.Errorf("too many: %v", err)
	}
}

func TestS106_3_MarkdownSimpleBlocks(t *testing.T) {
	in := "# Spanish basics\n\nWhat is \"cat\" in Spanish?\ngato\n---\nWhat is \"dog\"?   perro\n\n---\n\n  How do you say\n  \"thank you\"?\n\ngracias\n(or muchas gracias)\n---\n"
	got, structured, err := ParseMarkdown(in)
	want := []Card{
		{Idea: "Spanish basics", Question: `What is "cat" in Spanish?`, Answer: "gato"},
		{Idea: "Spanish basics", Question: `What is "dog"?`, Answer: "perro"},
		{Idea: "Spanish basics", Question: "How do you say\n  \"thank you\"?", Answer: "gracias\n(or muchas gracias)"},
	}
	if err != nil || !structured || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v, %v", got, structured, err)
	}
	// Without a title the idea is left to the caller.
	got, structured, err = ParseMarkdown("Q one? A one\n---\nQ two? A two")
	if err != nil || !structured || len(got) != 2 || got[0].Idea != "" || got[1].Answer != "A two" {
		t.Fatalf("untitled: %+v, %v, %v", got, structured, err)
	}
	// Front matter and empty blocks are fine.
	got, structured, err = ParseMarkdown("---\n---\nWhy? Because.\n---\n\n---\n")
	if err != nil || !structured || len(got) != 1 {
		t.Fatalf("front matter: %+v, %v, %v", got, structured, err)
	}
	// A structured file with a card over the limits is refused in words.
	_, structured, err = ParseMarkdown("Why? " + strings.Repeat("b", MaxAnswerBytes+1) + "\n---\nok? ok")
	if !structured || err == nil || err.Error() != "Card 1's answer is longer than 2,000 characters." {
		t.Fatalf("long: %v, %v", structured, err)
	}
}

func TestS106_3_MarkdownUnstructuredGoesToDrafting(t *testing.T) {
	for name, in := range map[string]string{
		"prose with headings": "# The Federalist Papers\n\n## No. 10\n\nMadison argues that a large republic controls faction. Why? Because many interests check each other.\n\n## No. 51\n\nAmbition must counteract ambition.\n",
		"no separators":       "What is a cat? An animal.\n\nWhat is a dog? Another animal.",
		"block without ?":     "What is a cat? An animal.\n---\nJust a note.\n",
		"empty answer":        "What is a cat?\n---\nWhat is a dog? An animal.\n",
		"heading in block":    "What is a cat? An animal.\n---\n## Dogs\nWhat is a dog? Another animal.\n",
		"empty":               "",
		"only rules":          "---\n---\n",
	} {
		cards, structured, err := ParseMarkdown(in)
		if err != nil || structured || cards != nil {
			t.Errorf("%s: %+v, %v, %v", name, cards, structured, err)
		}
	}
	if _, _, err := ParseMarkdown("bad \xff utf8"); err == nil {
		t.Error("invalid UTF-8 accepted")
	}
}

// tricky holds the cards an export must carry back intact.
var tricky = []Card{
	{Idea: "Federalist Papers", Question: "Who wrote No. 10?", Answer: "James Madison"},
	{Idea: "Federalist Papers", Question: "What does it argue?\nIn short?", Answer: "A large republic\n\ncontrols faction?\n---\n# not a heading\n## nor this\nQ: nor a question\nA: nor an answer\nexact: true\n  indented line\n\n\nend"},
	{Idea: "Poems", Question: "Sonnet 18: how does it begin?", Answer: "Shall I compare thee to a summer's day?", Exact: true},
	{Idea: "", Question: "No idea here?", Answer: "a, \"quoted\", answer\twith tab"},
	{Idea: "## Odd: title, with \"quotes\" ---", Question: "---", Answer: "#"},
	{Idea: "Poems", Question: "Back to poems?", Answer: "yes", Exact: false},
	{Idea: "日本語", Question: "犬は英語で?", Answer: "dog", Exact: true},
}

func TestS106_3_MarkdownExportRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, tricky); err != nil {
		t.Fatal(err)
	}
	got, structured, err := ParseMarkdown(buf.String())
	if err != nil || !structured {
		t.Fatalf("%v %v\n%s", structured, err, buf.String())
	}
	if !reflect.DeepEqual(got, tricky) {
		t.Fatalf("round trip:\n got %+v\nwant %+v\n%s", got, tricky, buf.String())
	}
	if !strings.HasPrefix(buf.String(), "# Scry export\n\n## Federalist Papers\n\nQ: Who wrote No. 10?\nA: James Madison\n\n---\n\nQ: What does it argue?\n  In short?\n") {
		t.Fatalf("format:\n%s", buf.String())
	}
	// CRLF line endings from an editor still read back.
	got, _, err = ParseMarkdown(strings.ReplaceAll(buf.String(), "\n", "\r\n"))
	if err != nil || !reflect.DeepEqual(got, tricky) {
		t.Fatalf("crlf: %v", err)
	}
}

func TestS106_3_CSVExportRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, tricky); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "idea,question,answer,exact\nFederalist Papers,Who wrote No. 10?,James Madison,false\n") {
		t.Fatalf("format:\n%s", buf.String())
	}
	got, err := ParseDelimited(buf.Bytes(), ',')
	if err != nil || !reflect.DeepEqual(got, tricky) {
		t.Fatalf("round trip: %v\n got %+v\nwant %+v", err, got, tricky)
	}
}

func TestS106_3_DamagedExportRefusedInWords(t *testing.T) {
	for in, want := range map[string]string{
		"# Scry export\nQ: before idea\nA: x\n":          "Line 2 of this Scry export has a question before any \"## \" idea heading",
		"# Scry export\n## I\nQ: q\n---\n":               "Line 4 of this Scry export follows a question that has no answer",
		"# Scry export\n## I\nA: a\n":                    "Line 3 of this Scry export has an answer without a question",
		"# Scry export\n## I\nQ: q\nA: a\nstray text\n":  "Line 5 of this Scry export isn't an idea heading",
		"# Scry export\n## I\n  indented\n":              "Line 3 of this Scry export is indented",
		"# Scry export\n## I\nQ: q\nexact: true\nA: a\n": "Line 4 of this Scry export marks a card exact before its answer",
		"# Scry export\n":                                "This Scry export has no cards in it.",
		"# Scry export\n## I\nQ: q\nA:\n":                "Card 1 has no answer.",
	} {
		cards, structured, err := ParseMarkdown(in)
		if !structured || cards != nil || err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: %v, %v; want %q", in, structured, err, want)
		}
	}
}

// fragments are the pieces generated text is made of, chosen to collide
// with every part of the formats.
var fragments = []string{
	"x", "word", " ", "  ", "\t", "\n", "\n\n", "\r\n", "\r", "?", "---", "#", "## ", "Q:", "A: ", "exact: true",
	",", "\"", "'", "é", "日本", "\u00a0", "\\", "<b>", "&amp;",
}

func genText(min int) *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		parts := rapid.SliceOfN(rapid.SampledFrom(fragments), min, 12).Draw(t, "parts")
		return strings.Join(parts, "")
	})
}

func genCards() *rapid.Generator[[]Card] {
	return rapid.Custom(func(t *rapid.T) []Card {
		ideas := rapid.SliceOfN(genText(0), 1, 4).Draw(t, "ideas")
		n := rapid.IntRange(1, 12).Draw(t, "n")
		var cards []Card
		for i := 0; i < n; i++ {
			c := Card{
				Idea:     rapid.SampledFrom(ideas).Draw(t, "idea"),
				Question: genText(1).Draw(t, "q"),
				Answer:   genText(1).Draw(t, "a"),
				Exact:    rapid.Bool().Draw(t, "exact"),
			}
			if fits(clean(c)) {
				cards = append(cards, c)
			}
		}
		if len(cards) == 0 {
			cards = append(cards, Card{Question: "q?", Answer: "a"})
		}
		return cards
	})
}

func cleaned(cards []Card) []Card {
	out := make([]Card, len(cards))
	for i, c := range cards {
		out[i] = clean(c)
	}
	return out
}

func TestS106_3_ExportRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cards := genCards().Draw(t, "cards")
		want := cleaned(cards)

		var md bytes.Buffer
		if err := WriteMarkdown(&md, cards); err != nil {
			t.Fatal(err)
		}
		got, structured, err := ParseMarkdown(md.String())
		if err != nil || !structured || !reflect.DeepEqual(got, want) {
			t.Fatalf("markdown: %v %v\n got %+v\nwant %+v\n%s", structured, err, got, want, md.String())
		}

		var csv bytes.Buffer
		if err := WriteCSV(&csv, cards); err != nil {
			t.Fatal(err)
		}
		got, err = ParseDelimited(csv.Bytes(), ',')
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("csv: %v\n got %+v\nwant %+v\n%s", err, got, want, csv.String())
		}

		// Writing what was read gives the same bytes: the clean form is stable.
		var again bytes.Buffer
		WriteMarkdown(&again, got)
		if again.String() != md.String() {
			t.Fatalf("markdown not stable:\n%s\n---\n%s", md.String(), again.String())
		}
	})
}
