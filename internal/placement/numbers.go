package placement

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Numbers in table cells are not broken across lines (RFP §2). What Core
// Text does inside and around a number was measured (RFP Discussion Log 17):
// half-width numbers have no break opportunity inside, but are broken inside
// their characters when the unit Core Text cannot separate is wider than the
// column; that unit includes an opening bracket before the number, closing
// punctuation after it, letters run into it, and the character before a
// leading hyphen-minus. Full-width digits have a break opportunity between
// every two characters.

var numberRE = regexp.MustCompile(`[-+−▲△¥￥$＄]?[0-9０-９]+(?:[,，.．][0-9０-９]+)*[%％]?`)

const (
	opening = "([{（［｛「『【〔〈《“‘"
	closing = ")]}）］｝」』】〕〉》”’、。，．,.!?！？:;：；%％"
)

// NumberTokens finds the numbers in s worth keeping whole (two characters or
// more), as byte ranges. Digits in an ASCII word where a letter comes before
// a digit (a hash, a UUID, CVE-2024-1234, ID1234, v1.2.3) are an identifier,
// not a number: they are left out, so an identifier never widens a column.
// Letters only after the digits are a unit (100mg, 1,234KB, +1.5pt) and the
// number stays a number, as does one with a currency sign after letters
// (US$1,234).
func NumberTokens(s string) [][]int {
	var out [][]int
	for _, m := range numberRE.FindAllStringIndex(s, -1) {
		if utf8.RuneCountInString(s[m[0]:m[1]]) >= 2 && !inIdentifier(s, m) {
			out = append(out, m)
		}
	}
	return out
}

// inIdentifier reports whether the match sits in a run of ASCII word
// characters and joiners (- . , _ + %) in which a letter comes before a
// digit. The run does not reach back past a sign or currency symbol that
// starts the match.
func inIdentifier(s string, m []int) bool {
	start, end := m[0], m[1]
	// A number starting with a currency or other sign (¥ $ ▲ ...) is its own
	// word: what precedes the sign is not part of it.
	if r, _ := utf8.DecodeRuneInString(s[start:]); r == '-' || r == '+' || isDigit(r) {
		for start > 0 {
			r, n := utf8.DecodeLastRuneInString(s[:start])
			if !isIDRune(r) {
				break
			}
			start -= n
		}
	}
	for end < len(s) {
		r, n := utf8.DecodeRuneInString(s[end:])
		if !isIDRune(r) {
			break
		}
		end += n
	}
	letter := false
	for _, r := range s[start:end] {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			letter = true
		case isDigit(r) && letter:
			return true
		}
	}
	return false
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' || r >= '０' && r <= '９' }

func isIDRune(r rune) bool { return isASCIIWord(r) || strings.ContainsRune("-.,+%", r) }

// BoundUnit widens the number at m to what a line breaker will not separate
// from it.
func BoundUnit(s string, m []int) string {
	start, end := m[0], m[1]
	if strings.HasPrefix(s[start:end], "-") && start > 0 {
		_, n := utf8.DecodeLastRuneInString(s[:start])
		start -= n // a hyphen-minus is not broken from what precedes it
	}
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(s[:start])
		if !strings.ContainsRune(opening, r) && !isASCIIWord(r) {
			break
		}
		start -= n
	}
	for end < len(s) {
		r, n := utf8.DecodeRuneInString(s[end:])
		if !strings.ContainsRune(closing, r) && !isASCIIWord(r) {
			break
		}
		end += n
	}
	return s[start:end]
}

func isASCIIWord(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}
