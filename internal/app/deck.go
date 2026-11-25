package app

import (
	"context"
	"strings"
	"unicode/utf8"

	"scry/internal/deck"
	"scry/internal/store"
)

// Imported is what an import did (S106-3): either cards went straight
// into the library (IdeaID, Cards), or unstructured Markdown went to the
// drafting agent as a draft outline (DraftID).
type Imported struct {
	IdeaID  int64
	Cards   int
	DraftID int64
}

// Import reads a deck file the learner uploaded in Create. Anki packages,
// CSV/TSV and structured Markdown (Scry's export, or `---` card blocks
// split by `?`) go straight into the library at no allowance cost.
// Unstructured Markdown is pasted material: it goes to the drafting agent
// like text typed into Create, drawn from the allowance like any draft.
//
// ideaTitle is optional. When given, it is the idea for every card that
// does not name its own (CSV rows of two columns, `---` blocks, an Anki
// package's cards, which otherwise take their deck's name); rows that name
// their idea (CSV idea column, Scry's Markdown export) keep it.
//
// Every refusal is a *FieldError whose message can be shown as it is.
func (a *App) Import(ctx context.Context, u store.User, filename string, data []byte, ideaTitle string) (Imported, error) {
	if _, err := a.Plan(ctx, u.ID); err != nil {
		return Imported{}, err
	}
	ideaTitle = strings.Join(strings.Fields(ideaTitle), " ")
	if len(ideaTitle) > deck.MaxIdeaBytes {
		return Imported{}, fieldErr("idea", "Keep the idea title under 120 characters.")
	}
	if len(data) == 0 {
		return Imported{}, fieldErr("file", "Choose a file to import: an Anki .apkg, a .csv or .tsv, or a .md file.")
	}
	if len(data) > deck.MaxBytes {
		return Imported{}, fieldErr("file", "That file is larger than 20 MiB. Split it and import the parts.")
	}
	kind, err := deck.KindOf(filename)
	if err != nil {
		return Imported{}, fieldErr("file", err.Error())
	}
	var cards []deck.Card
	overrideAll := false // the idea title replaces an Anki deck's name
	switch kind {
	case deck.KindAPKG:
		cards, err = deck.ParseAPKG(data)
		overrideAll = true
	case deck.KindCSV:
		cards, err = deck.ParseDelimited(data, ',')
	case deck.KindTSV:
		cards, err = deck.ParseDelimited(data, '\t')
	case deck.KindMD:
		if !utf8.Valid(data) {
			return Imported{}, fieldErr("file", "That Markdown file isn't UTF-8 text.")
		}
		text := strings.TrimPrefix(string(data), "\ufeff")
		var structured bool
		cards, structured, err = deck.ParseMarkdown(text)
		if err == nil && !structured {
			// Unstructured notes: the drafting agent writes the cards.
			material := text
			if ideaTitle != "" && !strings.HasPrefix(strings.TrimSpace(text), "#") {
				material = "# " + ideaTitle + "\n\n" + text
			}
			if utf8.RuneCountInString(material) > MaxLearnChars {
				return Imported{}, fieldErr("file", "That Markdown has no card blocks, so Scry's drafting agent would write its cards, but it is longer than 16,000 characters. Import it in parts, or split it into `---` card blocks.")
			}
			id, err := a.Learn(ctx, u, material)
			if err != nil {
				return Imported{}, err
			}
			return Imported{DraftID: id}, nil
		}
	}
	if err != nil {
		return Imported{}, fieldErr("file", err.Error())
	}
	if ideaTitle != "" {
		for i := range cards {
			if overrideAll || cards[i].Idea == "" {
				cards[i].Idea = ideaTitle
			}
		}
	}
	defaultIdea := ideaTitle
	if defaultIdea == "" {
		defaultIdea = ideaFromFilename(filename)
	}
	source := map[string]string{deck.KindAPKG: "Anki", deck.KindCSV: "CSV", deck.KindTSV: "TSV", deck.KindMD: "Markdown"}[kind] + " · " + baseName(filename)
	ideaID, n, err := a.Store.For(u.ID).ImportCards(ctx, cards, defaultIdea, truncateRunes(source, 200), a.now())
	if err != nil {
		return Imported{}, fieldErr("file", err.Error())
	}
	a.Log.InfoContext(ctx, "deck imported", "user", u.ID, "kind", kind, "cards", n)
	return Imported{IdeaID: ideaID, Cards: n}, nil
}

// baseName is the file's own name, without any client path.
func baseName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSpace(name)
}

// ideaFromFilename names the idea for cards that name none: the file's
// name without its extension ("spanish-verbs.csv" -> "spanish verbs").
func ideaFromFilename(name string) string {
	name = baseName(name)
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }), " ")
	if name == "" {
		return "Imported cards"
	}
	return truncateRunes(name, 80)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n]))
}

// DeckExport is every live card of the learner, for the Markdown and CSV
// downloads in Settings → Your data. It works on any plan state.
func (a *App) DeckExport(ctx context.Context, u store.User) ([]deck.Card, error) {
	return a.Store.For(u.ID).DeckCards(ctx)
}
