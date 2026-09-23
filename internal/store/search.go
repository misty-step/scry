package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"unicode"
)

// The search index is derived data, maintained in the same transaction as the
// rows it describes and rebuildable from them at any time.

func unindex(ctx context.Context, tx *sql.Tx, kind, ref string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM search_index WHERE kind=? AND ref=?", kind, ref)
	return err
}

func index(ctx context.Context, tx *sql.Tx, kind, ref, title, body string) error {
	if err := unindex(ctx, tx, kind, ref); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO search_index(kind,ref,title,body) VALUES(?,?,?,?)", kind, ref, title, body)
	return err
}

func indexQuizContent(ctx context.Context, tx *sql.Tx, quizID string, content GeneratedQuiz) error {
	body := strings.Join(append([]string{content.Answer, content.Explanation}, content.Choices...), "\n")
	return index(ctx, tx, "question", quizID, content.Prompt, body)
}

// rebuildSearchIndex recreates every index row from durable data.
func rebuildSearchIndex(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM search_index"); err != nil {
		return err
	}
	statements := []string{
		`INSERT INTO search_index(kind,ref,title,body) SELECT 'concept',id,name,description FROM concepts WHERE origin<>'foundation' AND status='active'`,
		// One row per concept's current standard note, keyed by the concept,
		// exactly as publication indexes it.
		`INSERT INTO search_index(kind,ref,title,body) SELECT 'note',n.concept_id,n.title,n.body FROM notes n JOIN concepts c ON c.id=n.concept_id
		 WHERE c.status='active' AND c.origin<>'foundation'
		 AND n.rowid=(SELECT x.rowid FROM notes x WHERE x.concept_id=n.concept_id ORDER BY x.created_at DESC,x.rowid DESC LIMIT 1)`,
		`INSERT INTO search_index(kind,ref,title,body) SELECT 'source',id,text,'' FROM sources WHERE archived=0`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT q.id,v.content FROM quizzes q JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 JOIN sources src ON src.id=q.source_id WHERE q.archived=0 AND src.archived=0`)
	if err != nil {
		return err
	}
	type pending struct {
		id      string
		content GeneratedQuiz
	}
	var quizzes []pending
	for rows.Next() {
		var id, encoded string
		if err = rows.Scan(&id, &encoded); err != nil {
			rows.Close()
			return err
		}
		var content GeneratedQuiz
		if err = json.Unmarshal([]byte(encoded), &content); err != nil {
			rows.Close()
			return err
		}
		quizzes = append(quizzes, pending{id, content})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, q := range quizzes {
		if err = indexQuizContent(ctx, tx, q.id, q.content); err != nil {
			return err
		}
	}
	return nil
}

// ftsQuery turns learner text into a safe FTS5 query: each word is quoted
// (so operators and punctuation are inert) and the last word matches as a
// prefix. It returns "" when the text has no searchable words.
func ftsQuery(text string) string {
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) > 12 {
		words = words[:12]
	}
	terms := make([]string, 0, len(words))
	for i, word := range words {
		term := `"` + strings.ReplaceAll(word, `"`, "") + `"`
		if i == len(words)-1 && len([]rune(word)) >= 2 {
			term += "*"
		}
		terms = append(terms, term)
	}
	return strings.Join(terms, " ")
}

// ftsAny is like ftsQuery but matches any word; used to find related concepts.
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
