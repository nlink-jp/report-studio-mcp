package markdown

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Parse reads a manuscript. It never fails: whatever cannot be shown
// faithfully is reported in Document.Warnings.
//
// Before parsing, invalid UTF-8 becomes U+FFFD (as cmark does) and a leading
// byte order mark is dropped, so every string in the tree is valid UTF-8.
func Parse(src []byte) *Document {
	return parse(src, true)
}

// parse with extensions off is plain CommonMark 0.31.2: no front matter,
// tables, strikethrough, task lists, extended autolinks, footnotes or alerts.
// The spec's own examples are run that way, as cmark-gfm runs them.
func parse(src []byte, ext bool) *Document {
	s := string(src)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	s = strings.TrimPrefix(s, "\uFEFF")

	doc := &Document{Meta: map[string]string{}}
	c := &converter{doc: doc, ext: ext, fnIndex: map[*bnode]int{}}
	body, first := s, 1
	if ext {
		body, first = c.frontMatter(s)
	}
	bp := newBlockParser(first)
	bp.ext = ext
	bp.parse(body)
	c.bp = bp

	doc.Children = c.blocks(bp.doc.children, true)
	// Footnotes are numbered in the order of their first reference; a
	// reference inside a footnote can add another to the queue.
	for i := 0; i < len(c.fnQueue); i++ {
		doc.Footnotes[i].Children = c.blocks(c.fnQueue[i].children, false)
	}
	sort.SliceStable(c.warnings, func(i, j int) bool { return c.warnings[i].Line < c.warnings[j].Line })
	doc.Warnings = c.warnings
	return doc
}

// frontMatterKeys are the keys the RFP accepts (§2 Input).
var frontMatterKeys = map[string]bool{
	"title": true, "subtitle": true, "author": true, "date": true,
	"classification": true, "profile": true, "cover": true, "toc": true,
}

// frontMatter takes a front matter block off the start of s: a first line
// that is exactly "---", single-level "key: value" lines, and a closing line
// that is exactly "---". Without the closing line there is no front matter.
// It returns the body and the manuscript line the body starts on.
func (c *converter) frontMatter(s string) (string, int) {
	type line struct {
		text string
		n    int
	}
	var lines []line
	for pos, n := 0, 1; pos < len(s); n++ {
		j := pos
		for j < len(s) && s[j] != '\n' && s[j] != '\r' {
			j++
		}
		text, next := s[pos:j], j
		if next < len(s) {
			if s[next] == '\r' && next+1 < len(s) && s[next+1] == '\n' {
				next += 2
			} else {
				next++
			}
		}
		switch {
		case n == 1:
			if text != "---" {
				return s, 1
			}
		case text == "---":
			for _, l := range lines {
				c.frontMatterLine(l.text, l.n)
			}
			return s[next:], n + 1
		default:
			lines = append(lines, line{text, n})
		}
		pos = next
	}
	return s, 1
}

func (c *converter) frontMatterLine(text string, n int) {
	if isBlank(text) {
		return
	}
	key, value, ok := splitFrontMatterLine(text)
	if !ok {
		c.warn(n, "invalid_front_matter_line", fmt.Sprintf("front matter line %q is not \"key: value\"", text))
		return
	}
	key = strings.ToLower(key)
	if !frontMatterKeys[key] {
		c.warn(n, "unknown_front_matter_key", fmt.Sprintf("front matter key %q is not one of title, subtitle, author, date, classification, profile, cover, toc", key))
		return
	}
	c.doc.Meta[key] = value
}

// splitFrontMatterLine reads "key: value". The key starts the line (an
// indented line would be nesting, which is not accepted); a value in one pair
// of matching quotes loses them.
func splitFrontMatterLine(text string) (key, value string, ok bool) {
	i := 0
	for i < len(text) && (isAlnum(text[i]) || text[i] == '_' || text[i] == '-') {
		i++
	}
	if i == 0 {
		return "", "", false
	}
	key = text[:i]
	for i < len(text) && isSpaceOrTab(text[i]) {
		i++
	}
	if i >= len(text) || text[i] != ':' {
		return "", "", false
	}
	i++
	if i < len(text) && !isSpaceOrTab(text[i]) {
		return "", "", false
	}
	value = trimSpaceTab(text[i:])
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return key, value, true
}

// converter turns the parse tree into the published tree, parsing inlines on
// the way and collecting warnings.
type converter struct {
	doc      *Document
	bp       *blockParser
	ext      bool
	warnings []Warning
	fnIndex  map[*bnode]int
	fnQueue  []*bnode
}

func (c *converter) warn(line int, code, msg string) {
	c.warnings = append(c.warnings, Warning{Line: line, Code: code, Message: msg})
}

func (c *converter) blocks(ns []*bnode, top bool) []Block {
	var out []Block
	for _, n := range ns {
		if b := c.block(n, top); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func (c *converter) block(n *bnode, top bool) Block {
	switch n.kind {
	case bParagraph:
		return &Paragraph{Children: c.inlines(n.content, n.contentLine), Line: Line(n.contentLine)}
	case bHeading:
		if n.level >= 4 {
			c.warn(n.startLine, "deep_heading", fmt.Sprintf("heading level %d is deeper than 3", n.level))
		}
		return &Heading{Level: n.level, Children: c.inlines(n.content, n.startLine), Line: Line(n.startLine)}
	case bThematicBreak:
		return &ThematicBreak{Line: Line(n.startLine)}
	case bBlockQuote:
		if top && c.ext {
			if a := c.alert(n); a != nil {
				return a
			}
		}
		return &BlockQuote{Children: c.blocks(n.children, false), Line: Line(n.startLine)}
	case bList:
		l := &List{Ordered: n.list.ordered, Start: n.list.start, Tight: n.list.tight, Line: Line(n.startLine)}
		for _, it := range n.children {
			l.Items = append(l.Items, &ListItem{Task: it.task, Children: c.blocks(it.children, false), Line: Line(it.startLine)})
		}
		return l
	case bCodeBlock:
		return &CodeBlock{Fenced: n.fenced, Info: n.info, Text: n.literal, Line: Line(n.startLine)}
	case bHTMLBlock:
		c.warn(n.startLine, "raw_html", "raw HTML block is shown as text")
		return &HTMLBlock{Raw: n.literal, Line: Line(n.startLine)}
	case bTable:
		t := &Table{Align: n.align, Line: Line(n.startLine)}
		for k, row := range n.rows {
			cells := make([][]Inline, len(row.cells))
			for i, cell := range row.cells {
				cells[i] = c.inlines(cell, row.line)
			}
			if k == 0 {
				t.Header = cells
			} else {
				t.Rows = append(t.Rows, cells)
			}
		}
		return t
	}
	return nil // footnote definitions are hoisted into Document.Footnotes
}

var alertKinds = map[string]AlertKind{
	"NOTE": AlertNote, "TIP": AlertTip, "IMPORTANT": AlertImportant,
	"WARNING": AlertWarning, "CAUTION": AlertCaution,
}

// alert makes a top-level block quote whose first line is "[!KIND]" an
// Alert (GitHub Docs, "Alerts"). Alerts cannot nest, so only blocks directly
// under the document qualify; a marker with nothing after it stays a quote.
func (c *converter) alert(n *bnode) *Alert {
	if len(n.children) == 0 || n.children[0].kind != bParagraph {
		return nil
	}
	p := n.children[0]
	first, rest, _ := strings.Cut(p.content, "\n")
	first = trimSpaceTab(first)
	if len(first) < 4 || !strings.HasPrefix(first, "[!") || first[len(first)-1] != ']' {
		return nil
	}
	kind, ok := alertKinds[strings.ToUpper(first[2:len(first)-1])]
	if !ok {
		return nil
	}
	var children []Block
	if !isBlank(rest) {
		children = append(children, &Paragraph{Children: c.inlines(rest, p.contentLine+1), Line: Line(p.contentLine + 1)})
	}
	children = append(children, c.blocks(n.children[1:], false)...)
	if len(children) == 0 {
		return nil
	}
	return &Alert{Kind: kind, Children: children, Line: Line(n.startLine)}
}

func (c *converter) inlines(content string, line int) []Inline {
	content = trimSpaceTab(content)
	if content == "" {
		return nil
	}
	root := parseInlines(content, line, c.bp.refs, c.bp.footnotes, c.ext)
	return c.inlineList(root.first)
}

// inlineList converts a sibling list, merging adjacent text and splicing
// groups in place. Groups may nest arbitrarily (one per dropped construct),
// so they are walked with an explicit stack rather than recursion.
func (c *converter) inlineList(first *inode) []Inline {
	var out []Inline
	var tb strings.Builder
	pending := false
	flush := func() {
		if pending {
			out = append(out, &Text{Value: tb.String()})
			tb.Reset()
			pending = false
		}
	}
	var stack []*inode
	for n := first; ; {
		if n == nil {
			if len(stack) == 0 {
				break
			}
			n = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			continue
		}
		switch n.kind {
		case iGroup:
			stack = append(stack, n.next)
			n = n.first
			continue
		case iText:
			if n.lit != "" {
				tb.WriteString(n.lit)
				pending = true
			}
		default:
			flush()
			out = append(out, c.inline(n))
		}
		n = n.next
	}
	flush()
	return out
}

func (c *converter) inline(n *inode) Inline {
	switch n.kind {
	case iSoftBreak:
		return &SoftBreak{}
	case iHardBreak:
		return &HardBreak{}
	case iCode:
		return &Code{Value: n.lit}
	case iEmph:
		return &Emph{Children: c.inlineList(n.first)}
	case iStrong:
		return &Strong{Children: c.inlineList(n.first)}
	case iStrike:
		return &Strike{Children: c.inlineList(n.first)}
	case iLink:
		return &Link{Dest: n.dest, Title: n.title, Children: c.inlineList(n.first), Autolink: n.autolink}
	case iImage:
		return &Image{Dest: n.dest, Title: n.title, Alt: c.inlineList(n.first)}
	case iHTML:
		c.warn(n.line, "raw_html", "inline raw HTML is shown as text")
		return &RawHTML{Raw: n.lit}
	case iFootnoteRef:
		idx, ok := c.fnIndex[n.footnote]
		if !ok {
			idx = len(c.fnQueue) + 1
			c.fnIndex[n.footnote] = idx
			c.fnQueue = append(c.fnQueue, n.footnote)
			c.doc.Footnotes = append(c.doc.Footnotes, &Footnote{Label: n.footnote.label, Index: idx})
		}
		return &FootnoteRef{Label: n.lit, Index: idx}
	}
	return &Text{Value: n.lit}
}
