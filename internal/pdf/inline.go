package pdf

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// textStyle is the inline context a block sets: the fonts, size and colour
// its text starts from.
type textStyle struct {
	font, bold string
	size       float64
	color      RGB
}

// rich is a piece of inline text with what decorates it beyond its style.
type rich struct {
	text   string
	style  Style
	url    string // a clickable link
	strike bool
	code   bool // a code span: background
}

// inliner flattens inline nodes into rich text for one block.
type inliner struct {
	th   Theme
	warn func(code, msg string)
	out  []rich
	soft []int // indices in out of soft breaks, resolved once neighbours are known
}

type inlineCtx struct {
	ts      textStyle
	bold    bool
	oblique bool
	strike  bool
	url     string
	link    bool
}

func (c inlineCtx) style() Style {
	f := c.ts.font
	if c.bold {
		f = c.ts.bold
	}
	col := c.ts.color
	return Style{Font: f, Size: c.ts.size, R: col[0], G: col[1], B: col[2], Oblique: c.oblique, Underline: c.link}
}

// flatten converts inlines in a text style to rich text.
func flatten(th Theme, ts textStyle, nodes []markdown.Inline, warn func(code, msg string)) []rich {
	in := &inliner{th: th, warn: warn}
	in.walk(nodes, inlineCtx{ts: ts})
	in.resolveSoftBreaks()
	return in.out
}

func (in *inliner) emit(text string, c inlineCtx) {
	if text == "" {
		return
	}
	in.out = append(in.out, rich{text: text, style: c.style(), url: c.url, strike: c.strike})
}

func (in *inliner) walk(nodes []markdown.Inline, c inlineCtx) {
	for _, n := range nodes {
		switch v := n.(type) {
		case *markdown.Text:
			in.emit(v.Value, c)
		case *markdown.Code:
			cc := c
			cc.ts.font, cc.ts.bold, cc.ts.size = in.th.Code, in.th.Code, c.ts.size*0.9
			in.out = append(in.out, rich{text: v.Value, style: cc.style(), url: c.url, strike: c.strike, code: true})
		case *markdown.Emph:
			cc := c
			cc.oblique = true
			in.walk(v.Children, cc)
		case *markdown.Strong:
			cc := c
			cc.bold = true
			in.walk(v.Children, cc)
		case *markdown.Strike:
			cc := c
			cc.strike = true
			in.walk(v.Children, cc)
		case *markdown.Link:
			cc := c
			if clickable(v.Dest) {
				cc.url, cc.link = linkTarget(v.Dest), true
				cc.ts.color = in.th.Link
			} else if v.Dest != "" {
				in.warn("link_scheme", "the link to "+strconv.Quote(v.Dest)+" is shown as text: only http, https and mailto links are clickable")
			}
			in.walk(v.Children, cc)
		case *markdown.Image:
			// An image inside running text cannot be placed; its description is.
			in.warn("inline_image", "an image inside a paragraph is shown as its description; put it on a line of its own")
			in.emit("["+plain(v.Alt)+"]", c)
		case *markdown.RawHTML:
			in.emit(v.Raw, c)
		case *markdown.SoftBreak:
			in.soft = append(in.soft, len(in.out))
			in.out = append(in.out, rich{text: " ", style: c.style()})
		case *markdown.HardBreak:
			in.out = append(in.out, rich{text: " ", style: c.style()})
		case *markdown.FootnoteRef:
			st := c.style()
			st.Superscript, st.Underline = true, false
			in.out = append(in.out, rich{text: strconv.Itoa(v.Index), style: st})
		}
	}
}

// resolveSoftBreaks drops a soft break between two characters of a script
// written without spaces (Japanese, Chinese): there a line ending in the source
// is not a space. Elsewhere it stays a space.
func (in *inliner) resolveSoftBreaks() {
	for _, i := range in.soft {
		var before, after rune
		for j := i - 1; j >= 0 && before == 0; j-- {
			if t := in.out[j].text; t != "" {
				before, _ = utf8.DecodeLastRuneInString(t)
			}
		}
		for j := i + 1; j < len(in.out) && after == 0; j++ {
			if t := in.out[j].text; t != "" {
				after, _ = utf8.DecodeRuneInString(t)
			}
		}
		if wide(before) && wide(after) {
			in.out[i].text = ""
		}
	}
}

func wide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) ||
		(r >= 0x3000 && r <= 0x303f) || (r >= 0xff00 && r <= 0xffef)
}

// clickable reports whether a destination may become a link annotation.
func clickable(dest string) bool {
	d := strings.ToLower(dest)
	return strings.HasPrefix(d, "http://") || strings.HasPrefix(d, "https://") || strings.HasPrefix(d, "mailto:")
}

// linkTarget is the destination a link annotation gets: a mailto keeps only
// its address, since a query can ask an old mail client to attach a local
// file (?attach=/etc/passwd: independent review).
func linkTarget(dest string) string {
	if strings.HasPrefix(strings.ToLower(dest), "mailto:") {
		if i := strings.IndexByte(dest, '?'); i >= 0 {
			return dest[:i]
		}
	}
	return dest
}

// PlainText returns inlines as plain text, as titles, captions and the table
// of contents show them: code spans and raw HTML keep their text.
func PlainText(nodes []markdown.Inline) string { return plain(nodes) }

// plain returns inlines as plain text (image descriptions, captions, titles).
func plain(nodes []markdown.Inline) string { return markdown.PlainText(nodes) }
