package markdown

import (
	"bytes"
	"strconv"
	"strings"
)

// renderHTML writes the tree as the CommonMark reference renderer (cmark's
// html.c) would, so the spec examples can be compared byte for byte. GFM
// constructs follow the GFM spec's examples (tables, <del>, task list
// checkboxes in the attribute order the spec shows). Alerts and footnotes
// have no spec output; they are rendered in a simple form of GitHub's markup
// for the tests' sake. Nothing here reaches a browser: the typesetters do not
// use HTML.
func renderHTML(d *Document) string {
	r := &htmlRenderer{}
	r.blocks(d.Children, false)
	if len(d.Footnotes) > 0 {
		r.cr()
		r.s("<section class=\"footnotes\">\n<ol>\n")
		for _, f := range d.Footnotes {
			r.s("<li id=\"fn-" + strconv.Itoa(f.Index) + "\">\n")
			r.blocks(f.Children, false)
			r.cr()
			r.s("</li>\n")
		}
		r.s("</ol>\n</section>\n")
	}
	return r.b.String()
}

type htmlRenderer struct{ b bytes.Buffer }

func (r *htmlRenderer) s(t string) { r.b.WriteString(t) }

func (r *htmlRenderer) cr() {
	if n := r.b.Len(); n > 0 && r.b.Bytes()[n-1] != '\n' {
		r.b.WriteByte('\n')
	}
}

func (r *htmlRenderer) esc(t string) {
	for i := 0; i < len(t); i++ {
		switch c := t[i]; c {
		case '&':
			r.s("&amp;")
		case '<':
			r.s("&lt;")
		case '>':
			r.s("&gt;")
		case '"':
			r.s("&quot;")
		default:
			r.b.WriteByte(c)
		}
	}
}

// escHref finishes what normalizeURI began: the destination is already
// percent-encoded, so only the ampersand and the apostrophe need HTML
// escaping (houdini_escape_href).
func (r *htmlRenderer) escHref(t string) {
	for i := 0; i < len(t); i++ {
		switch c := t[i]; c {
		case '&':
			r.s("&amp;")
		case '\'':
			r.s("&#x27;")
		default:
			r.b.WriteByte(c)
		}
	}
}

func (r *htmlRenderer) blocks(bs []Block, tight bool) {
	for _, b := range bs {
		r.block(b, tight)
	}
}

var alertTitles = map[AlertKind]string{
	AlertNote: "Note", AlertTip: "Tip", AlertImportant: "Important",
	AlertWarning: "Warning", AlertCaution: "Caution",
}

func (r *htmlRenderer) block(b Block, tight bool) {
	switch b := b.(type) {
	case *Paragraph:
		if tight {
			r.inlines(b.Children)
			return
		}
		r.cr()
		r.s("<p>")
		r.inlines(b.Children)
		r.s("</p>\n")
	case *Heading:
		h := strconv.Itoa(b.Level)
		r.cr()
		r.s("<h" + h + ">")
		r.inlines(b.Children)
		r.s("</h" + h + ">\n")
	case *ThematicBreak:
		r.cr()
		r.s("<hr />\n")
	case *BlockQuote:
		r.cr()
		r.s("<blockquote>\n")
		r.blocks(b.Children, false)
		r.cr()
		r.s("</blockquote>\n")
	case *Alert:
		r.cr()
		r.s("<div class=\"markdown-alert markdown-alert-" + strings.ToLower(string(b.Kind)) + "\">\n")
		r.s("<p class=\"markdown-alert-title\">" + alertTitles[b.Kind] + "</p>\n")
		r.blocks(b.Children, false)
		r.cr()
		r.s("</div>\n")
	case *List:
		r.cr()
		switch {
		case !b.Ordered:
			r.s("<ul>\n")
		case b.Start == 1:
			r.s("<ol>\n")
		default:
			r.s("<ol start=\"" + strconv.Itoa(b.Start) + "\">\n")
		}
		for _, it := range b.Items {
			r.cr()
			r.s("<li>")
			if it.Task != nil {
				if *it.Task {
					r.s("<input checked=\"\" disabled=\"\" type=\"checkbox\"> ")
				} else {
					r.s("<input disabled=\"\" type=\"checkbox\"> ")
				}
			}
			r.blocks(it.Children, b.Tight)
			r.s("</li>\n")
		}
		if b.Ordered {
			r.s("</ol>\n")
		} else {
			r.s("</ul>\n")
		}
	case *CodeBlock:
		r.cr()
		if b.Info == "" {
			r.s("<pre><code>")
		} else {
			word := b.Info
			if i := strings.IndexAny(word, " \t\n\v\f\r"); i >= 0 {
				word = word[:i]
			}
			r.s("<pre><code class=\"language-")
			r.esc(word)
			r.s("\">")
		}
		r.esc(b.Text)
		r.s("</code></pre>\n")
	case *HTMLBlock:
		r.cr()
		r.s(b.Raw)
		r.cr()
	case *Table:
		r.cr()
		r.s("<table>\n<thead>\n")
		r.row(b.Header, b.Align, "th")
		r.s("</thead>\n")
		if len(b.Rows) > 0 {
			r.s("<tbody>\n")
			for _, row := range b.Rows {
				r.row(row, b.Align, "td")
			}
			r.s("</tbody>\n")
		}
		r.s("</table>\n")
	}
}

func (r *htmlRenderer) row(cells [][]Inline, align []Align, tag string) {
	r.s("<tr>\n")
	for i, cell := range cells {
		r.s("<" + tag)
		switch align[i] {
		case AlignLeft:
			r.s(" align=\"left\"")
		case AlignCenter:
			r.s(" align=\"center\"")
		case AlignRight:
			r.s(" align=\"right\"")
		}
		r.s(">")
		r.inlines(cell)
		r.s("</" + tag + ">\n")
	}
	r.s("</tr>\n")
}

func (r *htmlRenderer) inlines(ns []Inline) {
	for _, n := range ns {
		r.inline(n)
	}
}

func (r *htmlRenderer) inline(n Inline) {
	switch n := n.(type) {
	case *Text:
		r.esc(n.Value)
	case *SoftBreak:
		r.s("\n")
	case *HardBreak:
		r.s("<br />\n")
	case *Code:
		r.s("<code>")
		r.esc(n.Value)
		r.s("</code>")
	case *Emph:
		r.s("<em>")
		r.inlines(n.Children)
		r.s("</em>")
	case *Strong:
		r.s("<strong>")
		r.inlines(n.Children)
		r.s("</strong>")
	case *Strike:
		r.s("<del>")
		r.inlines(n.Children)
		r.s("</del>")
	case *Link:
		r.s("<a href=\"")
		r.escHref(n.Dest)
		if n.Title != "" {
			r.s("\" title=\"")
			r.esc(n.Title)
		}
		r.s("\">")
		r.inlines(n.Children)
		r.s("</a>")
	case *Image:
		r.s("<img src=\"")
		r.escHref(n.Dest)
		r.s("\" alt=\"")
		r.plain(n.Alt)
		if n.Title != "" {
			r.s("\" title=\"")
			r.esc(n.Title)
		}
		r.s("\" />")
	case *RawHTML:
		r.s(n.Raw)
	case *FootnoteRef:
		i := strconv.Itoa(n.Index)
		r.s("<sup class=\"footnote-ref\"><a href=\"#fn-" + i + "\" id=\"fnref-" + i + "\">" + i + "</a></sup>")
	}
}

// plain writes an image description as cmark does: text only, breaks as
// spaces.
func (r *htmlRenderer) plain(ns []Inline) {
	for _, n := range ns {
		switch n := n.(type) {
		case *Text:
			r.esc(n.Value)
		case *Code:
			r.esc(n.Value)
		case *RawHTML:
			r.esc(n.Raw)
		case *SoftBreak, *HardBreak:
			r.s(" ")
		case *Emph:
			r.plain(n.Children)
		case *Strong:
			r.plain(n.Children)
		case *Strike:
			r.plain(n.Children)
		case *Link:
			r.plain(n.Children)
		case *Image:
			r.plain(n.Alt)
		case *FootnoteRef:
			r.s(strconv.Itoa(n.Index))
		}
	}
}
