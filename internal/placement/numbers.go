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
// more), as byte ranges.
func NumberTokens(s string) [][]int {
	var out [][]int
	for _, m := range numberRE.FindAllStringIndex(s, -1) {
		if utf8.RuneCountInString(s[m[0]:m[1]]) >= 2 {
			out = append(out, m)
		}
	}
	return out
}

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
