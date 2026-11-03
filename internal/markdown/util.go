package markdown

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Character classes as CommonMark 0.31.2 §2.1 defines them.

func isSpaceOrTab(c byte) bool { return c == ' ' || c == '\t' }

// isWhitespaceByte is cmark's isspace: what ends a link destination, an
// extended autolink and similar runs.
func isWhitespaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool       { return isASCIILetter(c) || isDigit(c) }
func isHex(c byte) bool         { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

// isUnicodeWhitespace: Zs, tab, line feed, form feed, carriage return.
func isUnicodeWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// isUnicodePunct: Unicode P or S categories (0.31.2 widened this from P).
func isUnicodePunct(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIPunct(byte(r))
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// isBlank reports whether s holds only spaces, tabs and line endings.
func isBlank(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
		default:
			return false
		}
	}
	return true
}

// trimSpaceTab removes leading and trailing spaces, tabs and line endings.
func trimSpaceTab(s string) string {
	i, j := 0, len(s)
	for i < j && isWhitespaceByte(s[i]) {
		i++
	}
	for j > i && isWhitespaceByte(s[j-1]) {
		j--
	}
	return s[i:j]
}

// scanEntity reads an entity or numeric character reference at s[i] == '&'
// (§2.5). It returns the decoded text and the length consumed, or 0 when
// there is no valid reference. Named references are resolved through
// html.UnescapeString, which carries the full HTML5 table; CommonMark wants
// the semicolon, so only "&name;" is offered and a prefix match (Go resolves
// legacy names without one, e.g. "&notit;" → "¬it;") is rejected.
func scanEntity(s string, i int) (string, int) {
	j := i + 1
	if j >= len(s) {
		return "", 0
	}
	if s[j] == '#' {
		j++
		cp := 0
		start := j
		if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
			j++
			start = j
			for j < len(s) && j-start < 6 && isHex(s[j]) {
				cp = cp*16 + hexVal(s[j])
				j++
			}
		} else {
			for j < len(s) && j-start < 7 && isDigit(s[j]) {
				cp = cp*10 + int(s[j]-'0')
				j++
			}
		}
		if j == start || j >= len(s) || s[j] != ';' {
			return "", 0
		}
		if cp == 0 || cp > unicode.MaxRune || cp >= 0xD800 && cp <= 0xDFFF {
			cp = unicode.ReplacementChar
		}
		return string(rune(cp)), j + 1 - i
	}
	start := j
	if !isASCIILetter(s[j]) {
		return "", 0
	}
	for j < len(s) && j-start < 32 && isAlnum(s[j]) {
		j++
	}
	if j-start < 2 || j >= len(s) || s[j] != ';' {
		return "", 0
	}
	ref := s[i : j+1]
	u := html.UnescapeString(ref)
	if u == ref || strings.HasSuffix(u, ";") && ref != "&semi;" {
		return "", 0
	}
	return u, j + 1 - i
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// unescapeString resolves backslash escapes and entity references, as link
// destinations, titles and info strings require (§2.4, §2.5).
func unescapeString(s string) string {
	if strings.IndexByte(s, '\\') < 0 && strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if c == '&' {
			if t, n := scanEntity(s, i); n > 0 {
				b.WriteString(t)
				i += n
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// hrefSafe is the byte set cmark's reference renderer leaves alone in a URL
// (houdini_escape_href); every other byte is percent-encoded. '&' and '\” are
// kept here too: they are safe in a URL and need only HTML escaping, which is
// the renderer's business, not the tree's.
var hrefSafe = func() (t [256]bool) {
	for c := 'a'; c <= 'z'; c++ {
		t[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		t[c] = true
	}
	for c := '0'; c <= '9'; c++ {
		t[c] = true
	}
	for _, c := range []byte("-_.+!*'(),%#@?=;:/,+&$~") {
		t[c] = true
	}
	return t
}()

// normalizeURI percent-encodes what cmark's renderer would encode, so that
// Link.Dest and Image.Dest are the href the reference renderer writes (minus
// HTML escaping).
func normalizeURI(s string) string {
	n := 0
	for i := 0; i < len(s); i++ {
		if !hrefSafe[s[i]] {
			n++
		}
	}
	if n == 0 {
		return s
	}
	const hexDigits = "0123456789ABCDEF"
	b := make([]byte, 0, len(s)+2*n)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if hrefSafe[c] {
			b = append(b, c)
		} else {
			b = append(b, '%', hexDigits[c>>4], hexDigits[c&15])
		}
	}
	return string(b)
}

// normalizeLabel turns a link label (without its brackets) into its matching
// key (§4.7): Unicode case fold, trim, and collapse internal whitespace.
func normalizeLabel(s string) string {
	s = trimSpaceTab(s)
	if k := asciiLabelKey(s); k != "" || s == "" {
		return k
	}
	return foldLabel(s)
}

// foldLabel is normalizeLabel for any trimmed label, ASCII or not.
func foldLabel(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		if f, ok := fullCaseFold[r]; ok {
			for _, fr := range f {
				b.WriteRune(foldRune(fr))
			}
			continue
		}
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

// asciiLabelKey is normalizeLabel's fast path for an all-ASCII label that is
// already trimmed: it returns "" when s holds a non-ASCII byte. A label that
// is already its own key (lower case, single spaces) is returned without
// copying.
func asciiLabelKey(s string) string {
	same := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= utf8.RuneSelf:
			return ""
		case c >= 'A' && c <= 'Z', c == '\t' || c == '\n' || c == '\r':
			same = false
		case c == ' ' && i > 0 && s[i-1] == ' ':
			same = false
		}
	}
	if same {
		return s
	}
	b := make([]byte, 0, len(s))
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			space = true
			continue
		}
		if space {
			b = append(b, ' ')
			space = false
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b = append(b, c)
	}
	return string(b)
}

// foldRune is simple case folding to a canonical member of the rune's case
// orbit. Going through upper case first is what makes 'ſ', 'ς' and the Kelvin
// sign meet 's', 'σ' and 'k'.
func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	return unicode.ToLower(unicode.ToUpper(r))
}
