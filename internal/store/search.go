package store

import (
	"context"
	"database/sql"
	"strings"
	"unicode"
)

// The concept index finds reuse candidates for new plans. It is derived data,
// maintained in the same transaction as the concepts it describes and
// rebuildable from them at any time.

func indexConcept(ctx context.Context, tx *sql.Tx, id, name, summary string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM concept_index WHERE ref=?", id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO concept_index(ref,name,summary) VALUES(?,?,?)", id, name, summary)
	return err
}

// rebuildConceptIndex recreates every index row from durable concepts.
func rebuildConceptIndex(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM concept_index"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO concept_index(ref,name,summary) SELECT id,name,description FROM concepts WHERE origin<>'foundation' AND status='active'`)
	return err
}

// ftsAny turns text into a safe FTS5 query matching any of its words: each is
// quoted, so operators and punctuation are inert. It returns "" when the text
// has no searchable words.
func ftsAny(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	var terms []string
	for _, word := range words {
		if len([]rune(word)) < 3 || stopword(strings.ToLower(word)) {
			continue
		}
		terms = append(terms, `"`+word+`"`)
		if len(terms) == 16 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}

func stopword(word string) bool {
	switch word {
	case "the", "and", "for", "with", "that", "this", "what", "how", "why", "are", "was", "from", "into", "about", "does", "its", "their", "your", "you", "can", "has", "have", "not", "but", "use", "used", "when", "which", "who", "will":
		return true
	}
	return false
}
