package slug

import (
	"strings"
)

const MaxLength = 100

// Slugify canonicalizes s into a URL-safe slug. It is the single source of
// truth for all server-side slug generation. Callers must use this and must
// not replicate the transliteration or truncation logic.
//
// Invariants:
//   - TrimSpace, lowercase, transliterate common Latin + Polish diacritics
//   - separators/punctuation become "-"
//   - repeated "-" collapsed, leading/trailing "-" trimmed
//   - truncated to MaxLength without trailing "-"
//   - deterministic, pure, no I/O
//   - returns "" on empty/punctuation-only input; fallback naming belongs to caller
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(
		"ą", "a", "ć", "c", "ę", "e", "ł", "l", "ń", "n", "ó", "o", "ś", "s", "ź", "z", "ż", "z",
		"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "æ", "ae",
		"è", "e", "é", "e", "ê", "e", "ë", "e",
		"ì", "i", "í", "i", "î", "i", "ï", "i",
		"ò", "o", "ô", "o", "õ", "o", "ö", "o", "ø", "o",
		"ù", "u", "ú", "u", "û", "u", "ü", "u",
		"ý", "y", "ÿ", "y",
		"ñ", "n", "ç", "c",
		"ß", "ss",
	).Replace(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if r == '-' {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		} else {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > MaxLength {
		res = strings.Trim(strings.TrimSpace(res[:MaxLength]), "-")
	}
	return res
}

// Alias for convenience — both names refer to the same canonical behavior.
func Make(s string) string { return Slugify(s) }
