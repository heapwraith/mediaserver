package scanner

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NormTitle returns the grouping key of a title: lowercase, without accents
// on Latin letters, without punctuation and with single spaces.
//
// Only marks that follow a Latin letter are removed, so Japanese kana keep
// their dakuten (ブ stays ブ, not フ).
func NormTitle(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevLatin := false
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			if prevLatin {
				continue // drop the accent
			}
			b.WriteRune(r)
		case r == '&':
			if b.Len() > 0 && !space {
				b.WriteByte(' ')
			}
			b.WriteString("and")
			space, prevLatin = false, false
		case r == '\'' || r == '’' || r == '`':
			// "Grey's" groups with "Greys"
			prevLatin = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(foldLatin(r)))
			prevLatin = unicode.Is(unicode.Latin, r)
			space = false
		default:
			if b.Len() > 0 && !space {
				b.WriteByte(' ')
				space = true
			}
			prevLatin = false
		}
	}
	return norm.NFC.String(strings.TrimSpace(b.String()))
}

// foldLatin maps Latin letters that NFD does not decompose.
func foldLatin(r rune) rune {
	switch r {
	case 'ø', 'Ø':
		return 'o'
	case 'đ', 'Đ':
		return 'd'
	case 'ł', 'Ł':
		return 'l'
	case 'ß':
		return 's'
	case 'æ', 'Æ':
		return 'a'
	case 'œ', 'Œ':
		return 'o'
	}
	return r
}

// titleScore rates how "clean" a display title is. Higher is better:
// mixed case, accents and non-ASCII letters carry more information than an
// all-lowercase or all-uppercase release name.
func titleScore(s string) int {
	if s == "" {
		return -1 << 30
	}
	score := 0
	hasUpper, hasLower := false, false
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		}
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			score += 2 // é, ç, ã, kana...
		}
		if r == '.' || r == '_' {
			score -= 3
		}
	}
	if hasUpper && hasLower {
		score += 10
	}
	return score
}

// cleanTitle turns separators into spaces and tidies the result.
// In-word hyphens (Spider-Man) and apostrophes are kept.
func cleanTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '.' || r == '_' {
			return ' '
		}
		return r
	}, s)
	// Standalone dashes inside the title stay ("Mission Impossible - Dead
	// Reckoning"); the ones at the edges are leftovers of removed tags.
	return strings.Trim(strings.Join(strings.Fields(s), " "), " -–—:,;+~")
}
