package deck

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ankiNote is one note of a test collection.
type ankiNote struct {
	Model  int64 // model id
	Deck   int64 // deck id of its cards
	Fields []string
	Cards  int // cards the note has (default 1)
}

type ankiFixture struct {
	Models map[int64]int    // model id → type (0 standard, 1 cloze)
	Decks  map[int64]string // deck id → name
	Notes  []ankiNote
}

// collection builds a legacy (schema 11) Anki collection, as Anki writes
// it with "Support older Anki versions", and returns the file's bytes.
func collection(t testing.TB, fx ankiFixture) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collection.anki2")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE col (id integer primary key, crt integer not null, mod integer not null, scm integer not null,
			ver integer not null, dty integer not null, usn integer not null, ls integer not null,
			conf text not null, models text not null, decks text not null, dconf text not null, tags text not null)`,
		`CREATE TABLE notes (id integer primary key, guid text not null, mid integer not null, mod integer not null,
			usn integer not null, tags text not null, flds text not null, sfld integer not null, csum integer not null,
			flags integer not null, data text not null)`,
		`CREATE TABLE cards (id integer primary key, nid integer not null, did integer not null, ord integer not null,
			mod integer not null, usn integer not null, type integer not null, queue integer not null, due integer not null,
			ivl integer not null, factor integer not null, reps integer not null, lapses integer not null, left integer not null,
			odue integer not null, odid integer not null, flags integer not null, data text not null)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	models := map[string]any{}
	for id, typ := range fx.Models {
		flds := []map[string]any{{"name": "Front", "ord": 0}, {"name": "Back", "ord": 1}}
		name := "Basic"
		if typ == 1 {
			flds = []map[string]any{{"name": "Text", "ord": 0}, {"name": "Back Extra", "ord": 1}}
			name = "Cloze"
		}
		models[fmt.Sprint(id)] = map[string]any{"id": id, "name": name, "type": typ, "flds": flds}
	}
	decks := map[string]any{}
	for id, name := range fx.Decks {
		decks[fmt.Sprint(id)] = map[string]any{"id": id, "name": name}
	}
	mj, _ := json.Marshal(models)
	dj, _ := json.Marshal(decks)
	if _, err := db.Exec(`INSERT INTO col VALUES (1, 0, 0, 0, 11, 0, 0, 0, '{}', ?, ?, '{}', '{}')`, string(mj), string(dj)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	cardID := int64(1)
	for i, n := range fx.Notes {
		nid := int64(1000 + i)
		if _, err := tx.Exec(`INSERT INTO notes VALUES (?, ?, ?, 0, 0, '', ?, '', 0, 0, '')`,
			nid, fmt.Sprintf("guid%d", i), n.Model, strings.Join(n.Fields, "\x1f")); err != nil {
			t.Fatal(err)
		}
		count := n.Cards
		if count == 0 {
			count = 1
		}
		for ord := 0; ord < count; ord++ {
			if _, err := tx.Exec(`INSERT INTO cards VALUES (?, ?, ?, ?, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, '')`,
				cardID, nid, n.Deck, ord); err != nil {
				t.Fatal(err)
			}
			cardID++
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type zipEntry struct {
	Name string
	Data []byte
}

// zipped packs entries into a zip, as Anki does, with fixed timestamps.
func zipped(t testing.TB, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.Name, Method: zip.Deflate, Modified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.Data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// apkg is a whole package: the collection, a media map and one media file.
func apkg(t testing.TB, fx ankiFixture) []byte {
	return zipped(t,
		zipEntry{"collection.anki2", collection(t, fx)},
		zipEntry{"media", []byte(`{"0": "brain.png"}`)},
		zipEntry{"0", []byte("\x89PNG\r\n\x1a\nnot really an image")},
	)
}

// sampleFixture is the deck behind testdata/decks/sample.apkg: two basic
// notes (one with HTML and an image) and a cloze note with two clozes.
var sampleFixture = ankiFixture{
	Models: map[int64]int{1: 0, 2: 1},
	Decks:  map[int64]string{1: "Default", 10: "Neuroscience::Memory"},
	Notes: []ankiNote{
		{Model: 1, Deck: 10, Fields: []string{
			`What does the <b>hippocampus</b> do?<br><img src="brain.png">`,
			`<div>Forms new long-term</div><div>memories &amp; maps space.</div>`}},
		{Model: 1, Deck: 10, Fields: []string{`Who described the forgetting curve?`, `Hermann Ebbinghaus [sound:ebbinghaus.mp3]`}},
		{Model: 2, Deck: 10, Cards: 2, Fields: []string{
			`The {{c1::spacing effect}} means reviews spread over time beat {{c2::massed practice::a cram}}.`, `Extra`}},
	},
}

var sampleCards = []Card{
	{Idea: "Neuroscience · Memory", Question: "What does the hippocampus do?", Answer: "Forms new long-term\nmemories & maps space."},
	{Idea: "Neuroscience · Memory", Question: "Who described the forgetting curve?", Answer: "Hermann Ebbinghaus"},
	{Idea: "Neuroscience · Memory", Question: "The […] means reviews spread over time beat massed practice.", Answer: "spacing effect"},
	{Idea: "Neuroscience · Memory", Question: "The spacing effect means reviews spread over time beat [a cram].", Answer: "massed practice"},
}

func TestS106_3_ImportAnkiBasicAndCloze(t *testing.T) {
	got, err := ParseAPKG(apkg(t, sampleFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, sampleCards) {
		t.Fatalf("cards:\n got %+v\nwant %+v", got, sampleCards)
	}
}

func TestS106_3_AnkiDeckNamesAndSkippedNotes(t *testing.T) {
	long := strings.Repeat("word ", 300) // over 1,000 bytes
	fx := ankiFixture{
		Models: map[int64]int{1: 0, 2: 1},
		Decks:  map[int64]string{1: "Default", 2: "Spanish"},
		Notes: []ankiNote{
			{Model: 1, Deck: 1, Fields: []string{"hola", "hello"}},                              // Default deck
			{Model: 1, Deck: 2, Fields: []string{`<img src="a.png">`, "only media"}},            // empty question
			{Model: 1, Deck: 2, Fields: []string{"[sound:x.mp3]", "[sound:y.mp3]"}},             // only sound
			{Model: 1, Deck: 2, Fields: []string{long, "too long"}},                             // over the limit
			{Model: 1, Deck: 2, Fields: []string{"lonely field"}},                               // no answer field
			{Model: 2, Deck: 2, Fields: []string{"no clozes here", ""}},                         // cloze without clozes
			{Model: 99, Deck: 2, Fields: []string{"{{c1::Madrid}} is the capital", ""}},         // unknown model, cloze markup
			{Model: 99, Deck: 77, Fields: []string{"gato", "cat"}},                              // unknown model and deck
			{Model: 2, Deck: 2, Fields: []string{"{{c1::uno}}, {{c1::dos}}, {{c2::tres}}", ""}}, // one cloze twice
		},
	}
	got, err := ParseAPKG(apkg(t, fx))
	if err != nil {
		t.Fatal(err)
	}
	want := []Card{
		{Idea: "Anki import", Question: "hola", Answer: "hello"},
		{Idea: "Spanish", Question: "[…] is the capital", Answer: "Madrid"},
		{Idea: "Anki import", Question: "gato", Answer: "cat"},
		{Idea: "Spanish", Question: "[…], […], tres", Answer: "uno, dos"},
		{Idea: "Spanish", Question: "uno, dos, […]", Answer: "tres"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cards:\n got %+v\nwant %+v", got, want)
	}
}

func TestS106_3_AnkiNothingReadableRefused(t *testing.T) {
	fx := ankiFixture{Models: map[int64]int{1: 0}, Decks: map[int64]string{1: "Default"},
		Notes: []ankiNote{{Model: 1, Deck: 1, Fields: []string{`<img src="a.png">`, `<img src="b.png">`}}}}
	_, err := ParseAPKG(apkg(t, fx))
	if err == nil || !strings.Contains(err.Error(), "no cards Scry can read") {
		t.Fatalf("err = %v", err)
	}
}

func TestS106_3_AnkiPrefersAnki21OverStub(t *testing.T) {
	stub := collection(t, ankiFixture{Models: map[int64]int{1: 0}, Decks: map[int64]string{1: "Default"},
		Notes: []ankiNote{{Model: 1, Deck: 1, Fields: []string{"Please update to the latest Anki version", "then import again"}}}})
	real := collection(t, ankiFixture{Models: map[int64]int{1: 0}, Decks: map[int64]string{5: "Real"},
		Notes: []ankiNote{{Model: 1, Deck: 5, Fields: []string{"Real question?", "Real answer"}}}})
	got, err := ParseAPKG(zipped(t, zipEntry{"collection.anki2", stub}, zipEntry{"collection.anki21", real}, zipEntry{"media", []byte("{}")}))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Card{{Idea: "Real", Question: "Real question?", Answer: "Real answer"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestS106_3_AnkiNewFormatRefusedInWords(t *testing.T) {
	stub := collection(t, ankiFixture{Models: map[int64]int{1: 0}, Decks: map[int64]string{1: "Default"},
		Notes: []ankiNote{{Model: 1, Deck: 1, Fields: []string{"Please update to the latest Anki version", "x"}}}})
	_, err := ParseAPKG(zipped(t, zipEntry{"collection.anki2", stub}, zipEntry{"collection.anki21b", []byte("\x28\xb5\x2f\xfdzstd")}, zipEntry{"media", []byte{}}))
	if err != errNewAnki || !strings.Contains(err.Error(), "Support older Anki versions") {
		t.Fatalf("err = %v", err)
	}
}

func TestS106_3_NotAnAnkiDeckRefusedInWords(t *testing.T) {
	cases := map[string][]byte{
		"not a zip":         []byte("hello, world"),
		"zip without deck":  zipped(t, zipEntry{"readme.txt", []byte("hi")}),
		"collection not db": zipped(t, zipEntry{"collection.anki2", []byte("SQLite format 3\x00 but not really")}),
		"db without tables": zipped(t, zipEntry{"collection.anki2", emptyDB(t)}),
	}
	for name, data := range cases {
		_, err := ParseAPKG(data)
		if err != errNotAnki {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if !strings.HasPrefix(errNotAnki.Error(), "That file isn't an Anki deck Scry can read.") {
		t.Fatal(errNotAnki)
	}
}

func emptyDB(t *testing.T) []byte {
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE other (x)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	b, _ := os.ReadFile(path)
	return b
}

func TestS106_3_ZipBombRefused(t *testing.T) {
	old := maxUnzipped
	maxUnzipped = 1 << 20
	t.Cleanup(func() { maxUnzipped = old })
	zeros := make([]byte, 4<<20)

	// An honest header that says the collection is too big.
	if _, err := ParseAPKG(zipped(t, zipEntry{"collection.anki2", zeros})); err != errUnzipLimit {
		t.Fatalf("honest bomb: err = %v", err)
	}

	// A header that lies about the size: the limit still holds.
	var comp bytes.Buffer
	fw, _ := flate.NewWriter(&comp, flate.BestCompression)
	fw.Write(zeros)
	fw.Close()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{Name: "collection.anki2", Method: zip.Deflate,
		CompressedSize64: uint64(comp.Len()), UncompressedSize64: 100, CRC32: 0})
	if err != nil {
		t.Fatal(err)
	}
	w.Write(comp.Bytes())
	zw.Close()
	if comp.Len() > 64<<10 {
		t.Fatalf("bomb not small: %d", comp.Len())
	}
	if _, err := ParseAPKG(buf.Bytes()); err != errUnzipLimit && err != errNotAnki {
		t.Fatalf("lying bomb: err = %v", err)
	}
	if !strings.Contains(errUnzipLimit.Error(), "200 MiB") {
		t.Fatal(errUnzipLimit)
	}

	// Too many entries.
	oldEntries := maxZipEntries
	maxZipEntries = 10
	t.Cleanup(func() { maxZipEntries = oldEntries })
	var many []zipEntry
	for i := 0; i < 11; i++ {
		many = append(many, zipEntry{fmt.Sprint(i), []byte("x")})
	}
	if _, err := ParseAPKG(zipped(t, many...)); err != errNotAnki {
		t.Fatalf("many entries: err = %v", err)
	}

	// Over the upload cap.
	if _, err := ParseAPKG(make([]byte, MaxBytes+1)); err != errTooBig {
		t.Fatalf("too big: err = %v", err)
	}
}

func TestS106_3_AnkiLeavesNoTempFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	if _, err := ParseAPKG(apkg(t, sampleFixture)); err != nil {
		t.Fatal(err)
	}
	old := maxUnzipped
	maxUnzipped = 1 << 10
	defer func() { maxUnzipped = old }()
	if _, err := ParseAPKG(apkg(t, sampleFixture)); err != errUnzipLimit {
		t.Fatalf("err = %v", err)
	}
	left, _ := os.ReadDir(tmp)
	if len(left) != 0 {
		t.Fatalf("left behind: %v", left)
	}
}

func TestS106_3_AnkiTooManyCardsRefused(t *testing.T) {
	fx := ankiFixture{Models: map[int64]int{1: 0}, Decks: map[int64]string{1: "Big"}}
	for i := 0; i <= MaxCards; i++ {
		fx.Notes = append(fx.Notes, ankiNote{Model: 1, Deck: 1, Fields: []string{fmt.Sprintf("q%d", i), "a"}})
	}
	_, err := ParseAPKG(apkg(t, fx))
	if err == nil || !strings.Contains(err.Error(), "more than 5,000 cards") {
		t.Fatalf("err = %v", err)
	}
}
