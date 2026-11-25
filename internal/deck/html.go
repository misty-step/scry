package deck

import (
	"html"
	"regexp"
	"strings"
)

// blockTags end a line when they open or close.
var blockTags = map[string]bool{
	"br": true, "div": true, "p": true, "li": true, "ul": true, "ol": true, "tr": true,
	"table": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "hr": true, "dt": true, "dd": true, "section": true,
}

// cellTags separate their content from what follows by a space.
var cellTags = map[string]bool{"td": true, "th": true}

// hiddenTags hide everything inside them.
var hiddenTags = map[string]bool{"script": true, "style": true, "head": true, "title": true}

var soundRef = regexp.MustCompile(`\[sound:[^\]]*\]`)

// PlainText turns a field of HTML (as Anki stores it) into plain text:
// tags are dropped, line-breaking tags (<br>, <div>, <p>, <li>…) become
// newlines, entities are decoded, media (<img>, [sound:…]) is dropped,
// runs of spaces become one space, each line is trimmed and blank lines
// are removed.
func PlainText(s string) string {
	s = soundRef.ReplaceAllString(s, "")
	var b strings.Builder
	hidden := ""
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			if hidden == "" {
				b.WriteString(s)
			}
			break
		}
		if hidden == "" {
			b.WriteString(s[:i])
		}
		s = s[i:]
		// A comment runs to "-->".
		if strings.HasPrefix(s, "<!--") {
			end := strings.Index(s[4:], "-->")
			if end < 0 {
				break
			}
			s = s[4+end+3:]
			continue
		}
		name, closing, rest, ok := tag(s)
		if !ok {
			// A "<" that opens no tag is text ("a < b").
			if hidden == "" {
				b.WriteByte('<')
			}
			s = s[1:]
			continue
		}
		s = rest
		switch {
		case hidden != "":
			if closing && name == hidden {
				hidden = ""
			}
		case hiddenTags[name] && !closing:
			hidden = name
		case blockTags[name]:
			b.WriteByte('\n')
		case cellTags[name]:
			b.WriteByte(' ')
		}
	}
	return tidy(html.UnescapeString(b.String()))
}

// tag reads the tag at the start of s ("<b>", "</div>", "<img src='x>y'>")
// and returns its lower-case name and what follows it.
func tag(s string) (name string, closing bool, rest string, ok bool) {
	i := 1
	if i < len(s) && (s[i] == '!' || s[i] == '?') {
		// <!DOCTYPE …> or <?xml …?>: skip to ">".
		end := strings.IndexByte(s, '>')
		if end < 0 {
			return "", false, "", false
		}
		return "!", false, s[end+1:], true
	}
	if i < len(s) && s[i] == '/' {
		closing = true
		i++
	}
	start := i
	for i < len(s) && (isLetter(s[i]) || (i > start && (s[i] >= '0' && s[i] <= '9' || s[i] == '-' || s[i] == ':'))) {
		i++
	}
	if i == start {
		return "", false, "", false
	}
	name = strings.ToLower(s[start:i])
	// Attributes run to the first ">" outside quotes.
	var quote byte
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return name, closing, s[i+1:], true
		}
	}
	// An unterminated tag swallows the rest, as a browser would.
	return name, closing, "", true
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// tidy makes runs of spaces one space, trims each line and drops blank
// lines.
func tidy(s string) string {
	s = strings.ReplaceAll(newlines(s), "\u00a0", " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}
