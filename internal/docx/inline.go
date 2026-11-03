package docx

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

type pieceKind int

const (
	pText pieceKind = iota
	pSoft
	pHard
	pNote
)

// piece is one run-to-be: inline text with its properties, flattened from
// the inline tree so that soft breaks can see their neighbours.
type piece struct {
	kind pieceKind
	text string
	r    rpr
	link int    // link instance, 0 for none; consecutive pieces of one link share a w:hyperlink
	url  string // its destination
	note int    // footnote index
}

type flat struct {
	w     *writer
	out   []piece
	links int
}

// inlines converts inline nodes into runs, starting from base properties.
func (w *writer) inlines(nodes []markdown.Inline, base rpr) string {
	f := &flat{w: w}
	f.walk(nodes, base, 0, "")
	f.resolveSoftBreaks()
	return f.xml()
}

func (f *flat) add(p piece) { f.out = append(f.out, p) }

func (f *flat) walk(nodes []markdown.Inline, r rpr, link int, url string) {
	for _, n := range nodes {
		switch v := n.(type) {
		case *markdown.Text:
			f.add(piece{kind: pText, text: v.Value, r: r, link: link, url: url})
		case *markdown.Code:
			rr := r
			rr.style = "CodeChar"
			if link > 0 { // the code style replaces Hyperlink; keep the link's look
				rr.color, rr.u = hexRGB(f.w.th.Link), true
			}
			f.add(piece{kind: pText, text: v.Value, r: rr, link: link, url: url})
		case *markdown.Emph:
			rr := r
			rr.i = true
			f.walk(v.Children, rr, link, url)
		case *markdown.Strong:
			rr := r
			rr.b = true
			f.walk(v.Children, rr, link, url)
		case *markdown.Strike:
			rr := r
			rr.s = true
			f.walk(v.Children, rr, link, url)
		case *markdown.Link:
			if clickable(v.Dest) {
				f.links++
				rr := r
				rr.style = "Hyperlink"
				f.walk(v.Children, rr, f.links, v.Dest)
				continue
			}
			if v.Dest != "" {
				f.w.warn("link_scheme", "the link to "+strconv.Quote(v.Dest)+" is shown as text: only http, https and mailto links are clickable")
			}
			f.walk(v.Children, r, link, url)
		case *markdown.Image:
			// An image inside running text cannot be placed; its description is.
			f.w.warn("inline_image", "an image inside a paragraph is shown as its description; put it on a line of its own")
			f.add(piece{kind: pText, text: "[" + plain(v.Alt) + "]", r: r, link: link, url: url})
		case *markdown.RawHTML:
			f.add(piece{kind: pText, text: v.Raw, r: r, link: link, url: url})
		case *markdown.SoftBreak:
			f.add(piece{kind: pSoft, text: " ", r: r, link: link, url: url})
		case *markdown.HardBreak:
			f.add(piece{kind: pHard, text: " ", r: r, link: link, url: url})
		case *markdown.FootnoteRef:
			f.add(piece{kind: pNote, text: strconv.Itoa(v.Index), r: r, link: link, url: url, note: v.Index})
		}
	}
}

// resolveSoftBreaks drops a soft break between two characters of a script
// written without spaces (Japanese, Chinese): there a line ending in the
// source is not a space. Elsewhere it stays a space. Same rule as the PDF
// typesetter's.
func (f *flat) resolveSoftBreaks() {
	for i, p := range f.out {
		if p.kind != pSoft {
			continue
		}
		var before, after rune
		for j := i - 1; j >= 0 && before == 0; j-- {
			if t := f.out[j].text; t != "" {
				before, _ = utf8.DecodeLastRuneInString(t)
			}
		}
		for j := i + 1; j < len(f.out) && after == 0; j++ {
			if t := f.out[j].text; t != "" {
				after, _ = utf8.DecodeRuneInString(t)
			}
		}
		if wide(before) && wide(after) {
			f.out[i].text = ""
		}
	}
}

func (f *flat) xml() string {
	var b strings.Builder
	ps := f.out
	for i := 0; i < len(ps); {
		if ps[i].link == 0 {
			b.WriteString(f.w.piece(ps[i]))
			i++
			continue
		}
		j := i
		for j < len(ps) && ps[j].link == ps[i].link {
			j++
		}
		var runs strings.Builder
		for k := i; k < j; k++ {
			runs.WriteString(f.w.piece(ps[k]))
		}
		if runs.Len() > 0 {
			rid := f.w.cur.add(relLink, f.w.clean(ps[i].url), true)
			b.WriteString(`<w:hyperlink r:id="` + rid + `" w:history="1">` + runs.String() + `</w:hyperlink>`)
		}
		i = j
	}
	return b.String()
}

// piece writes one piece as a run. A footnote's first reference in the body
// is a real Word footnote reference; Word allows one reference per note and
// none inside notes, so any other reference is its number in the reference
// style.
func (w *writer) piece(p piece) string {
	switch p.kind {
	case pHard:
		return `<w:r>` + p.r.xml() + `<w:br/></w:r>`
	case pNote:
		r := p.r
		r.style = "FootnoteReference"
		if !w.inNote && p.note >= 1 && p.note <= len(w.doc.Footnotes) && !w.noteSeen[p.note] {
			w.noteSeen[p.note] = true
			return `<w:r>` + r.xml() + `<w:footnoteReference w:id="` + itoa(p.note) + `"/></w:r>`
		}
		return w.run(p.text, r)
	}
	return w.run(p.text, p.r)
}

// run writes text as one run: tabs become w:tab and line ends w:br, so that
// nothing depends on a reader keeping whitespace inside w:t.
func (w *writer) run(s string, r rpr) string {
	s = w.clean(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<w:r>` + r.xml())
	start := 0
	flush := func(end int) {
		if end > start {
			b.WriteString(`<w:t xml:space="preserve">` + textEscaper.Replace(s[start:end]) + `</w:t>`)
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\t':
			flush(i)
			b.WriteString(`<w:tab/>`)
			start = i + 1
		case '\n':
			flush(i)
			b.WriteString(`<w:br/>`)
			start = i + 1
		case '\r':
			flush(i)
			start = i + 1
		}
	}
	flush(len(s))
	b.WriteString(`</w:r>`)
	return b.String()
}

func wide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) ||
		(r >= 0x3000 && r <= 0x303f) || (r >= 0xff00 && r <= 0xffef)
}

// clickable reports whether a destination may become a hyperlink: http,
// https and mailto only, the scheme in any case (RFP §2, links).
func clickable(dest string) bool {
	d := strings.ToLower(dest)
	return strings.HasPrefix(d, "http://") || strings.HasPrefix(d, "https://") || strings.HasPrefix(d, "mailto:")
}

// plain returns inlines as plain text (image descriptions, captions, table
// of contents entries).
func plain(nodes []markdown.Inline) string {
	var b strings.Builder
	var walk func([]markdown.Inline)
	walk = func(ns []markdown.Inline) {
		for _, n := range ns {
			switch v := n.(type) {
			case *markdown.Text:
				b.WriteString(v.Value)
			case *markdown.Code:
				b.WriteString(v.Value)
			case *markdown.Emph:
				walk(v.Children)
			case *markdown.Strong:
				walk(v.Children)
			case *markdown.Strike:
				walk(v.Children)
			case *markdown.Link:
				walk(v.Children)
			case *markdown.Image:
				walk(v.Alt)
			case *markdown.SoftBreak, *markdown.HardBreak:
				b.WriteString(" ")
			}
		}
	}
	walk(nodes)
	return b.String()
}

// textWidth estimates text's width in half-ems: characters of the wide
// scripts count two, others one. Only the proportions matter (table columns).
func textWidth(s string) int {
	n := 0
	for _, r := range s {
		if wide(r) || unicode.Is(unicode.Hangul, r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}
