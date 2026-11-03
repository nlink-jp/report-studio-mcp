package markdown

import "strings"

// Scanners shared by the block and inline phases. Each reads at a position
// of a string and either returns where its construct ends or reports that
// there is none; none of them looks further than its construct allows.

// maxLinkLabel is the limit of §4.7: at most 999 characters between the
// brackets.
const maxLinkLabel = 999

// maxDestParens bounds parenthesis nesting in a link destination. §6.3
// allows a limit and asks for at least three levels; cmark uses 32.
const maxDestParens = 32

// scanLinkLabel reads a link label at s[i] == '[' and returns the index just
// past its ']', or -1.
func scanLinkLabel(s string, i int) int {
	if i >= len(s) || s[i] != '[' {
		return -1
	}
	n := 0
	for j := i + 1; j < len(s); {
		c := s[j]
		switch c {
		case '[':
			return -1
		case ']':
			return j + 1
		case '\\':
			j++
			if j < len(s) && isASCIIPunct(s[j]) {
				j++
			}
			n++
		default:
			if c&0xC0 != 0x80 { // count characters, not bytes
				n++
			}
			j++
		}
		if n > maxLinkLabel {
			return -1
		}
	}
	return -1
}

// skipSpnl skips spaces and tabs, at most one line ending, and spaces and
// tabs again.
func skipSpnl(s string, i int) int {
	for i < len(s) && isSpaceOrTab(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '\n' {
		i++
		for i < len(s) && isSpaceOrTab(s[i]) {
			i++
		}
	}
	return i
}

// scanLinkDest reads a link destination (§6.3) at s[i]. The result is
// unescaped and percent-encoded.
func scanLinkDest(s string, i int) (string, int, bool) {
	if i < len(s) && s[i] == '<' {
		for j := i + 1; j < len(s); j++ {
			switch s[j] {
			case '\\':
				if j+1 < len(s) && isASCIIPunct(s[j+1]) {
					j++
				}
			case '>':
				return normalizeURI(unescapeString(s[i+1 : j])), j + 1, true
			case '<', '\n':
				return "", 0, false
			}
		}
		return "", 0, false
	}
	j, depth := i, 0
loop:
	for j < len(s) {
		c := s[j]
		switch {
		case c == '\\' && j+1 < len(s) && isASCIIPunct(s[j+1]):
			j += 2
		case c == '(':
			depth++
			if depth > maxDestParens {
				return "", 0, false
			}
			j++
		case c == ')':
			if depth == 0 {
				break loop
			}
			depth--
			j++
		case c <= ' ' || c == 0x7f:
			break loop
		default:
			j++
		}
	}
	if j == i && !(j < len(s) && s[j] == ')') || depth != 0 {
		return "", 0, false
	}
	return normalizeURI(unescapeString(s[i:j])), j, true
}

// scanLinkTitle reads a link title (§6.3) at s[i]; the result is unescaped.
func scanLinkTitle(s string, i int) (string, int, bool) {
	if i >= len(s) {
		return "", 0, false
	}
	var closer byte
	switch s[i] {
	case '"':
		closer = '"'
	case '\'':
		closer = '\''
	case '(':
		closer = ')'
	default:
		return "", 0, false
	}
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '\\' && j+1 < len(s) && isASCIIPunct(s[j+1]):
			j++
		case c == closer:
			return unescapeString(s[i+1 : j]), j + 1, true
		case closer == ')' && c == '(':
			return "", 0, false
		}
	}
	return "", 0, false
}

// --- Raw HTML (§6.6) ---

func isTagWS(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// scanTagName reads [A-Za-z][A-Za-z0-9-]* at s[i].
func scanTagName(s string, i int) int {
	if i >= len(s) || !isASCIILetter(s[i]) {
		return -1
	}
	i++
	for i < len(s) && (isAlnum(s[i]) || s[i] == '-') {
		i++
	}
	return i
}

// scanOpenTag returns the end of an open tag at s[i] == '<', or -1.
func scanOpenTag(s string, i int) int {
	j := scanTagName(s, i+1)
	if j < 0 {
		return -1
	}
	for {
		k := j
		for k < len(s) && isTagWS(s[k]) {
			k++
		}
		if k == j || k >= len(s) || !(isASCIILetter(s[k]) || s[k] == '_' || s[k] == ':') {
			j = k
			break
		}
		k++
		for k < len(s) && (isAlnum(s[k]) || strings.IndexByte("_.:-", s[k]) >= 0) {
			k++
		}
		// Optional value specification.
		m := k
		for m < len(s) && isTagWS(s[m]) {
			m++
		}
		if m < len(s) && s[m] == '=' {
			m++
			for m < len(s) && isTagWS(s[m]) {
				m++
			}
			if m >= len(s) {
				return -1
			}
			switch q := s[m]; q {
			case '"', '\'':
				e := strings.IndexByte(s[m+1:], q)
				if e < 0 {
					return -1
				}
				k = m + 1 + e + 1
			default:
				e := m
				for e < len(s) && s[e] > ' ' && strings.IndexByte("\"'=<>`", s[e]) < 0 {
					e++
				}
				if e == m {
					return -1
				}
				k = e
			}
		}
		j = k
	}
	if j < len(s) && s[j] == '/' {
		j++
	}
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return -1
}

// scanClosingTag returns the end of a closing tag at s[i] == '<', or -1.
func scanClosingTag(s string, i int) int {
	if i+1 >= len(s) || s[i+1] != '/' {
		return -1
	}
	j := scanTagName(s, i+2)
	if j < 0 {
		return -1
	}
	for j < len(s) && isTagWS(s[j]) {
		j++
	}
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return -1
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

var htmlBlockType1 = []string{"pre", "script", "style", "textarea"}

var htmlBlockType6 = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields(`address article aside base basefont blockquote body
		caption center col colgroup dd details dialog dir div dl dt fieldset
		figcaption figure footer form frame frameset h1 h2 h3 h4 h5 h6 head
		header hr html iframe legend li link main menu menuitem nav noframes ol
		optgroup option p param search section summary table tbody td tfoot th
		thead title tr track ul`) {
		m[n] = true
	}
	return m
}()

// htmlBlockStartType returns which of the seven HTML block start conditions
// (§4.6) the line s (starting with '<') meets, or 0. Type 7 cannot interrupt
// a paragraph.
func htmlBlockStartType(s string, interruptsParagraph bool) int {
	if len(s) < 2 {
		return 0
	}
	for _, name := range htmlBlockType1 {
		if hasPrefixFold(s[1:], name) {
			k := 1 + len(name)
			if k == len(s) || s[k] == ' ' || s[k] == '\t' || s[k] == '>' {
				return 1
			}
		}
	}
	switch {
	case strings.HasPrefix(s, "<!--"):
		return 2
	case strings.HasPrefix(s, "<?"):
		return 3
	case len(s) > 2 && s[1] == '!' && isASCIILetter(s[2]):
		return 4
	case strings.HasPrefix(s, "<![CDATA["):
		return 5
	}
	j := 1
	if s[j] == '/' {
		j++
	}
	k := j
	for k < len(s) && isAlnum(s[k]) {
		k++
	}
	if k > j && htmlBlockType6[strings.ToLower(s[j:k])] {
		if k == len(s) || s[k] == ' ' || s[k] == '\t' || s[k] == '>' || strings.HasPrefix(s[k:], "/>") {
			return 6
		}
	}
	if !interruptsParagraph {
		n := scanOpenTag(s, 0)
		if n < 0 {
			n = scanClosingTag(s, 0)
		}
		if n > 0 && isBlank(s[n:]) {
			return 7
		}
	}
	return 0
}

// htmlBlockEnds reports whether line meets the end condition of an HTML
// block of type 1 to 5.
func htmlBlockEnds(t int, line string) bool {
	switch t {
	case 1:
		l := strings.ToLower(line)
		for _, name := range htmlBlockType1 {
			if strings.Contains(l, "</"+name+">") {
				return true
			}
		}
		return false
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}

// --- GFM tables (GFM 0.29-gfm §4.10, cells as cmark-gfm's row_from_string) ---

// parseTableRow splits one line into trimmed cells. A pipe preceded by a
// backslash does not separate cells, and "\|" becomes "|" before inline
// parsing, even inside code spans (GFM example 200).
func parseTableRow(s string) ([]string, bool) {
	s = strings.TrimRight(s, " \t")
	i := 0
	if i < len(s) && s[i] == '|' {
		i++
	}
	var cells []string
	for i < len(s) {
		j := i
		for j < len(s) && s[j] != '|' {
			if s[j] == '\\' && j+1 < len(s) && s[j+1] == '|' {
				j++
			}
			j++
		}
		cells = append(cells, trimSpaceTab(unescapePipes(s[i:j])))
		if j < len(s) {
			j++ // the pipe
		}
		i = j
	}
	if len(cells) == 0 {
		return nil, false
	}
	return cells, true
}

func unescapePipes(s string) string {
	if !strings.Contains(s, `\|`) {
		return s
	}
	return strings.ReplaceAll(s, `\|`, "|")
}

// parseDelimiterRow reads a table delimiter row: cells of hyphens with an
// optional colon on either side.
func parseDelimiterRow(s string) ([]Align, bool) {
	if strings.IndexByte(s, '-') < 0 {
		return nil, false
	}
	cells, ok := parseTableRow(s)
	if !ok {
		return nil, false
	}
	aligns := make([]Align, len(cells))
	for k, c := range cells {
		left := strings.HasPrefix(c, ":")
		right := strings.HasSuffix(c, ":")
		inner := c
		if left {
			inner = inner[1:]
		}
		if right && len(inner) > 0 {
			inner = inner[:len(inner)-1]
		}
		if inner == "" || strings.Trim(inner, "-") != "" {
			return nil, false
		}
		switch {
		case left && right:
			aligns[k] = AlignCenter
		case left:
			aligns[k] = AlignLeft
		case right:
			aligns[k] = AlignRight
		}
	}
	return aligns, true
}
