package docx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// Namespaces and the XML declaration every part starts with.
const (
	xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	nsW     = `http://schemas.openxmlformats.org/wordprocessingml/2006/main`
	nsR     = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`
	nsWP    = `http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing`
	nsA     = `http://schemas.openxmlformats.org/drawingml/2006/main`
	nsPic   = `http://schemas.openxmlformats.org/drawingml/2006/picture`
	nsRel   = `http://schemas.openxmlformats.org/package/2006/relationships`
	relDoc  = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`
)

// Units. Word measures lengths in twentieths of a point (twips), font sizes
// in half-points, drawings in EMU, and border widths in eighths of a point.
func tw(pt float64) int      { return int(math.Round(pt * 20)) }
func hp(pt float64) int      { return int(math.Round(pt * 2)) }
func emu(pt float64) int64   { return int64(math.Round(pt * 12700)) }
func eighths(pt float64) int { return int(math.Round(pt * 8)) }

// lineAuto is a line spacing in 240ths of a single line for gapEm of extra
// space per line, taking a single line as 1.2 em as the PDF typesetter does.
func lineAuto(gapEm float64) int { return int(math.Round(240 * (1.2 + gapEm) / 1.2)) }

func itoa(n int) string { return strconv.Itoa(n) }

// hexRGB is a colour as Word writes it, RRGGBB.
func hexRGB(c theme.RGB) string {
	b := func(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return fmt.Sprintf("%02X%02X%02X", b(c[0]), b(c[1]), b(c[2]))
}

// xmlChar reports whether XML 1.0 can carry r (size is its encoded length:
// an invalid UTF-8 byte decodes as RuneError of size 1 and is dropped too,
// which is also how a lone surrogate arrives in a Go string).
func xmlChar(r rune, size int) bool {
	switch {
	case r == utf8.RuneError && size == 1:
		return false
	case r == 0x9 || r == 0xA || r == 0xD:
		return true
	case r < 0x20:
		return false
	case r >= 0xD800 && r <= 0xDFFF:
		return false
	case r == 0xFFFE || r == 0xFFFF:
		return false
	}
	return true
}

// clean removes what XML 1.0 cannot carry and reports whether anything was
// removed.
func clean(s string) (string, bool) {
	i := 0
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		if !xmlChar(r, n) {
			break
		}
		i += n
	}
	if i == len(s) {
		return s, false
	}
	var b strings.Builder
	b.WriteString(s[:i])
	for i < len(s) {
		r, n := utf8.DecodeRuneInString(s[i:])
		if xmlChar(r, n) {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String(), true
}

var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	// Attribute values also keep their whitespace: a parser would turn a
	// literal tab or line end into a space.
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"\t", "&#9;", "\n", "&#10;", "\r", "&#13;")
)

// ppr is a paragraph's properties; xml writes them in the order CT_PPr
// requires.
type ppr struct {
	style           string
	keepNext        bool
	keepLines       bool
	pageBreakBefore bool
	num             *numRef
	bdr             string // inner XML of w:pBdr, sides already in order
	tabs            string // inner XML of w:tabs
	before, after   int    // twips; -1 leaves the style's
	line            int    // 240ths of a line; 0 leaves the style's
	left, hanging   int    // twips
	ind             bool   // write w:ind
	jc              string
	outline         int // 0 none, else outline level + 1
}

type numRef struct{ id, ilvl int }

func newPPr(style string) ppr { return ppr{style: style, before: -1, after: -1} }

func (p ppr) xml() string {
	var b strings.Builder
	if p.style != "" {
		b.WriteString(`<w:pStyle w:val="` + p.style + `"/>`)
	}
	if p.keepNext {
		b.WriteString(`<w:keepNext/>`)
	}
	if p.keepLines {
		b.WriteString(`<w:keepLines/>`)
	}
	if p.pageBreakBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	if p.num != nil {
		b.WriteString(`<w:numPr><w:ilvl w:val="` + itoa(p.num.ilvl) + `"/><w:numId w:val="` + itoa(p.num.id) + `"/></w:numPr>`)
	}
	if p.bdr != "" {
		b.WriteString(`<w:pBdr>` + p.bdr + `</w:pBdr>`)
	}
	if p.tabs != "" {
		b.WriteString(`<w:tabs>` + p.tabs + `</w:tabs>`)
	}
	if p.before >= 0 || p.after >= 0 || p.line > 0 {
		b.WriteString(`<w:spacing`)
		if p.before >= 0 {
			b.WriteString(` w:before="` + itoa(p.before) + `"`)
		}
		if p.after >= 0 {
			b.WriteString(` w:after="` + itoa(p.after) + `"`)
		}
		if p.line > 0 {
			b.WriteString(` w:line="` + itoa(p.line) + `" w:lineRule="auto"`)
		}
		b.WriteString(`/>`)
	}
	if p.ind {
		b.WriteString(`<w:ind w:left="` + itoa(p.left) + `"`)
		if p.hanging > 0 {
			b.WriteString(` w:hanging="` + itoa(p.hanging) + `"`)
		}
		b.WriteString(`/>`)
	}
	if p.jc != "" {
		b.WriteString(`<w:jc w:val="` + p.jc + `"/>`)
	}
	if p.outline > 0 {
		b.WriteString(`<w:outlineLvl w:val="` + itoa(p.outline-1) + `"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<w:pPr>` + b.String() + `</w:pPr>`
}

// rpr is a run's properties; xml writes them in the order CT_RPr requires.
type rpr struct {
	style     string
	fonts     string // family for ascii/hAnsi/cs; eastAsia gets fontsEA or the same
	fontsEA   string
	b, i, s   bool
	color     string
	sz        float64 // points; 0 leaves the style's
	u         bool
	shd       string // fill colour
	vertAlign string
}

func (r rpr) xml() string {
	var b strings.Builder
	if r.style != "" {
		b.WriteString(`<w:rStyle w:val="` + r.style + `"/>`)
	}
	if r.fonts != "" {
		b.WriteString(rFonts(r.fonts, r.fontsEA))
	}
	if r.b {
		b.WriteString(`<w:b/><w:bCs/>`)
	}
	if r.i {
		b.WriteString(`<w:i/><w:iCs/>`)
	}
	if r.s {
		b.WriteString(`<w:strike/>`)
	}
	if r.color != "" {
		b.WriteString(`<w:color w:val="` + r.color + `"/>`)
	}
	if r.sz > 0 {
		b.WriteString(`<w:sz w:val="` + itoa(hp(r.sz)) + `"/><w:szCs w:val="` + itoa(hp(r.sz)) + `"/>`)
	}
	if r.u {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if r.shd != "" {
		b.WriteString(`<w:shd w:val="clear" w:color="auto" w:fill="` + r.shd + `"/>`)
	}
	if r.vertAlign != "" {
		b.WriteString(`<w:vertAlign w:val="` + r.vertAlign + `"/>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<w:rPr>` + b.String() + `</w:rPr>`
}

// rFonts names a family for every script; ea, when set, is the East Asian
// one (code fonts carry no Japanese, so Japanese in code keeps the body's).
func rFonts(f, ea string) string {
	if ea == "" {
		ea = f
	}
	// Family names come from the operator's profile, not the manuscript, so
	// cleaning them is no warning to the caller.
	f, _ = clean(f)
	ea, _ = clean(ea)
	f, ea = attrEscaper.Replace(f), attrEscaper.Replace(ea)
	return `<w:rFonts w:ascii="` + f + `" w:hAnsi="` + f + `" w:eastAsia="` + ea + `" w:cs="` + f + `"/>`
}

// border is one side of a paragraph or cell border.
func border(side string, widthPt float64, spacePt int, color string) string {
	return `<w:` + side + ` w:val="single" w:sz="` + itoa(eighths(widthPt)) + `" w:space="` + itoa(spacePt) + `" w:color="` + color + `"/>`
}
