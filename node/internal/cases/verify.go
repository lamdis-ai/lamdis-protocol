package cases

import (
	"strings"
	"unicode"
)

// The check that makes a letter worth sending: a quote counts only when the
// words are really in the document it names. Matching forgives what reading
// a photo gets wrong without understanding anything: case, curly quotes,
// dashes, line breaks and hyphens split across lines. It never forgives a
// different word.

// norm folds text to a comparable form, keeping punctuation.
func norm(s string) string {
	r := strings.NewReplacer(
		"‘", "'", "’", "'", "“", "\"", "”", "\"",
		"–", "-", "—", "-", "−", "-", " ", " ", "­", "",
		"-\r\n", "-", "-\n", "-")
	s = r.Replace(s)
	var b strings.Builder
	space := false
	for _, c := range strings.ToLower(s) {
		if unicode.IsSpace(c) {
			if !space {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(c)
	}
	return strings.TrimSpace(b.String())
}

// bare keeps only letters, digits and single spaces.
func bare(s string) string {
	var b strings.Builder
	space := false
	for _, c := range norm(s) {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			b.WriteRune(c)
			space = false
			continue
		}
		// A hyphen joins: "weight-related", and "re-quest" split across
		// lines, read the same as the whole word.
		if c == '-' {
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Contains reports whether quote appears in text, word for word.
func Contains(text, quote string) bool {
	q := strings.TrimSpace(quote)
	q = strings.Trim(q, "\"'“”‘’. …")
	if len([]rune(bare(q))) < 4 {
		return false
	}
	if strings.Contains(norm(text), norm(q)) {
		return true
	}
	// Punctuation is what photographs get wrong most; words are not. A
	// quote may stop at a line break inside a word, so only its start has
	// to sit on a word boundary.
	return strings.Contains(" "+bare(text)+" ", " "+bare(q))
}
