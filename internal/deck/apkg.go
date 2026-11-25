package deck

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Bounds on unzipping an .apkg. Only the collection entry is ever
// decompressed (media is skipped), and it is read through a limit that
// ignores whatever size the zip claims.
var (
	maxUnzipped   int64 = 200 << 20 // bytes decompressed from one package, in all
	maxZipEntries       = 100000    // entries in the zip's directory
	apkgTimeout         = 30 * time.Second
)

var errNotAnki = errors.New("That file isn't an Anki deck Scry can read. In Anki, choose File → Export, pick “Anki Deck Package (.apkg)” and import that file.")

var errNewAnki = errors.New("This deck was exported in the newest Anki format, which Scry can't read yet. In Anki, export it again with “Support older Anki versions” checked, then import that file.")

var errUnzipLimit = errors.New("That Anki deck unpacks to more than 200 MiB, which is more than Scry will read. Export a smaller deck, without media, and import that.")

// ParseAPKG reads an Anki deck package. Basic notes give one card (field
// 1 the question, field 2 the answer); cloze notes give a card per cloze
// number. Each card's idea is its deck's name ("Parent::Child" becomes
// "Parent · Child"), or DefaultAnkiIdea. HTML becomes plain text and media
// is skipped. Notes that come out empty or over a card's limits are skipped;
// a package with none left is refused.
func ParseAPKG(data []byte) ([]Card, error) {
	if len(data) > MaxBytes {
		return nil, errTooBig
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errNotAnki
	}
	if len(zr.File) > maxZipEntries {
		return nil, errNotAnki
	}
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	var coll *zip.File
	switch {
	case entries["collection.anki21"] != nil:
		coll = entries["collection.anki21"]
	case entries["collection.anki21b"] != nil:
		// Newer Anki writes a zstd collection and a stub collection.anki2
		// that only says to upgrade.
		return nil, errNewAnki
	case entries["collection.anki2"] != nil:
		coll = entries["collection.anki2"]
	default:
		return nil, errNotAnki
	}
	dir, err := os.MkdirTemp("", "scry-apkg-*")
	if err != nil {
		return nil, errors.New("Scry couldn't open that deck just now. Try again in a moment.")
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "collection.sqlite")
	if err := unzipTo(coll, path, maxUnzipped); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), apkgTimeout)
	defer cancel()
	cards, err := readCollection(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("That Anki deck took too long to read. Export a smaller deck and import that.")
		}
		return nil, err
	}
	return cards, nil
}

// unzipTo decompresses one entry to path, reading at most limit bytes
// whatever the entry's header claims.
func unzipTo(f *zip.File, path string, limit int64) error {
	if f.UncompressedSize64 > uint64(limit) {
		return errUnzipLimit
	}
	rc, err := f.Open()
	if err != nil {
		return errNotAnki
	}
	defer rc.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("Scry couldn't open that deck just now. Try again in a moment.")
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if n > limit {
		return errUnzipLimit
	}
	if err != nil {
		return errNotAnki
	}
	return nil
}

type ankiModel struct {
	Type int `json:"type"` // 0 standard, 1 cloze
}

type ankiDeck struct {
	Name string `json:"name"`
}

// readCollection reads the notes of an Anki collection (schema 11, as
// "Support older Anki versions" writes it).
func readCollection(ctx context.Context, path string) ([]Card, error) {
	dsn := "file:" + path + "?mode=ro&immutable=1&_pragma=trusted_schema(0)&_pragma=cell_size_check(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errNotAnki
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// The tables must be real tables, not views that compute anything.
	tables := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, errNotAnki
	}
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			tables[n] = true
		}
	}
	rows.Close()
	if !tables["notes"] || !tables["col"] || !tables["cards"] {
		return nil, errNotAnki
	}

	var modelsJSON, decksJSON string
	if err := db.QueryRowContext(ctx, `SELECT CAST(models AS TEXT), CAST(decks AS TEXT) FROM col LIMIT 1`).Scan(&modelsJSON, &decksJSON); err != nil {
		return nil, errNotAnki
	}
	models := map[string]ankiModel{}
	_ = json.Unmarshal([]byte(modelsJSON), &models)
	decks := map[string]ankiDeck{}
	_ = json.Unmarshal([]byte(decksJSON), &decks)
	if len(decks) == 0 && tables["decks"] {
		// A newer collection keeps decks in their own table.
		if rows, err := db.QueryContext(ctx, `SELECT id, name FROM decks`); err == nil {
			for rows.Next() {
				var id int64
				var name string
				if rows.Scan(&id, &name) == nil {
					decks[strconv.FormatInt(id, 10)] = ankiDeck{Name: strings.ReplaceAll(name, "\x1f", "::")}
				}
			}
			rows.Close()
		}
	}

	// The deck of each note's first card (its home deck, if the card sits
	// in a filtered deck for now).
	deckOf := map[int64]int64{}
	rows, err = db.QueryContext(ctx, `SELECT nid, CASE WHEN odid != 0 THEN odid ELSE did END, MIN(ord) FROM cards GROUP BY nid`)
	if err != nil {
		return nil, errNotAnki
	}
	for rows.Next() {
		var nid, did, ord int64
		if rows.Scan(&nid, &did, &ord) == nil {
			deckOf[nid] = did
		}
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT id, mid, CAST(flds AS TEXT) FROM notes ORDER BY id`)
	if err != nil {
		return nil, errNotAnki
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		var id, mid int64
		var flds string
		if err := rows.Scan(&id, &mid, &flds); err != nil {
			return nil, errNotAnki
		}
		idea := ideaName(decks[strconv.FormatInt(deckOf[id], 10)].Name)
		fields := strings.Split(flds, "\x1f")
		m, known := models[strconv.FormatInt(mid, 10)]
		cloze := m.Type == 1 || (!known && clozeRef.MatchString(fields[0]))
		var made []Card
		if cloze {
			made = clozeCards(idea, fields[0])
		} else if len(fields) >= 2 {
			made = []Card{{Idea: idea, Question: PlainText(fields[0]), Answer: PlainText(fields[1])}}
		}
		for _, c := range made {
			if c = clean(c); !fits(c) {
				continue // empty, or over a card's limits
			}
			if len(cards) == MaxCards {
				return nil, tooMany()
			}
			cards = append(cards, c)
		}
	}
	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errNotAnki
	}
	if len(cards) == 0 {
		return nil, errors.New("That Anki deck has no cards Scry can read: every note was empty, only media, or too long for a card.")
	}
	return cards, nil
}

// ideaName turns an Anki deck name into an idea title.
func ideaName(deck string) string {
	parts := strings.Split(deck, "::")
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	name := strings.Join(strings.Fields(strings.Join(kept, " · ")), " ")
	if name == "" || strings.EqualFold(name, "Default") {
		return DefaultAnkiIdea
	}
	return truncate(name, MaxIdeaBytes)
}

// clozeRef matches {{c1::text}} and {{c1::text::hint}}.
var clozeRef = regexp.MustCompile(`(?s)\{\{c(\d+)::(.*?)(?:::(.*?))?\}\}`)

// clozeCards makes one card per cloze number in text: the question shows
// the text with that cloze hidden ("[…]", or "[hint]") and the others
// filled in; the answer is what was hidden.
func clozeCards(idea, text string) []Card {
	var nums []int
	seen := map[int]bool{}
	for _, m := range clozeRef.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var out []Card
	for _, n := range nums {
		var answers []string
		q := clozeRef.ReplaceAllStringFunc(text, func(s string) string {
			m := clozeRef.FindStringSubmatch(s)
			if k, _ := strconv.Atoi(m[1]); k != n {
				return m[2]
			}
			if a := PlainText(m[2]); a != "" && !contains(answers, a) {
				answers = append(answers, a)
			}
			if strings.TrimSpace(PlainText(m[3])) != "" {
				return "[" + m[3] + "]"
			}
			return "[…]"
		})
		out = append(out, Card{Idea: idea, Question: PlainText(q), Answer: strings.Join(answers, ", ")})
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
