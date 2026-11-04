package pdf

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/nlink-jp/report-studio-mcp/internal/placement"
)

// Numbers in table cells are not broken across lines (RFP §2): a column is
// never narrower than its widest number unit (placement.BoundUnit), and the
// characters of every number are joined by a WORD JOINER (U+2060), which
// forbids a break on either side, has no width, and is not returned by PDF
// text extraction (measured: RFP Discussion Log 17). Without it, full-width
// digits break between any two characters, however wide the column.

// wordJoiner forbids a line break on either side and has no width.
const wordJoiner = "\u2060"

// tokenWidth is the width of the widest number unit in a cell.
func (l *layouter) tokenWidth(rs []rich) float64 {
	w := 0.0
	for _, r := range rs {
		for _, m := range placement.NumberTokens(r.text) {
			w = math.Max(w, l.doc.Measure(placement.BoundUnit(r.text, m), l.style(r.style)))
		}
	}
	return w
}

// span16 is a range of runes [start, end) in a cell's text.
type span16 struct{ start, end int }

// joinCells joins the characters of every number in each cell with word
// joiners, and returns where the numbers are in the joined text.
func joinCells(cells [][]rich) ([][]rich, [][]span16) {
	out := make([][]rich, len(cells))
	toks := make([][]span16, len(cells))
	for c, rs := range cells {
		out[c], toks[c] = joinNumbers(rs)
	}
	return out, toks
}

func joinNumbers(rs []rich) ([]rich, []span16) {
	var toks []span16
	out := make([]rich, len(rs))
	at := 0 // runes so far in the joined text
	for i, r := range rs {
		out[i] = r
		ms := placement.NumberTokens(r.text)
		if len(ms) == 0 {
			at += utf8.RuneCountInString(r.text)
			continue
		}
		var b strings.Builder
		prev := 0
		for _, m := range ms {
			b.WriteString(r.text[prev:m[0]])
			at += utf8.RuneCountInString(r.text[prev:m[0]])
			start := at
			for j, ch := range r.text[m[0]:m[1]] {
				if j > 0 {
					b.WriteString(wordJoiner)
					at++
				}
				b.WriteRune(ch)
				at++
			}
			toks = append(toks, span16{start, at})
			prev = m[1]
		}
		b.WriteString(r.text[prev:])
		at += utf8.RuneCountInString(r.text[prev:])
		out[i].text = b.String()
	}
	return out, toks
}

// brokenToken reports whether a line break falls inside a number.
func brokenToken(lines []Line, toks []span16) bool {
	for i := 0; i+1 < len(lines); i++ {
		end := lines[i].End
		for _, t := range toks {
			if end > t.start && end < t.end {
				return true
			}
		}
	}
	return false
}
