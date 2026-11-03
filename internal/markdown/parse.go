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
// Before parsing, invalid UTF-8 and NUL become U+FFFD (as cmark does) and a
// leading byte order mark is dropped, so every string in the tree, Meta
// included, is valid UTF-8 without NUL.
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
	// Spec section 2.3 insecure characters, before the front matter is read: its values
	// reach the cover and the document properties too.
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "\uFFFD")
	}

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
// that is "---", single-level "key: value" lines, and a closing line "---";
// either fence may have trailing spaces or tabs. Without the closing line
// there is no front matter; when the second line still looks like a known
// "key: value", that is reported, since the author most likely meant one.
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
			if !isFrontMatterFence(text) {
				return s, 1
			}
		case isFrontMatterFence(text):
			for _, l := range lines {
				c.frontMatterLine(l.text, l.n)
			}
			return s[next:], n + 1
		default:
			lines = append(lines, line{text, n})
		}
		pos = next
	}
	if len(lines) > 0 {
		if key, _, ok := splitFrontMatterLine(lines[0].text); ok && frontMatterKeys[strings.ToLower(key)] {
			c.warn(1, "unclosed_front_matter", "front matter opened by \"---\" on line 1 is never closed by another \"---\" line, so it is read as Markdown")
		}
	}
	return s, 1
}

// isFrontMatterFence: "---" with nothing after it but spaces or tabs.
func isFrontMatterFence(text string) bool {
	return strings.HasPrefix(text, "---") && strings.Trim(text[3:], " \t") == ""
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
	if _, dup := c.doc.Meta[key]; dup {
		c.warn(n, "duplicate_front_matter_key", fmt.Sprintf("front matter key %q is given again; this later value is used", key))
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
	inImage  int // image descriptions being converted
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
		children := c.blocks(n.children, false)
		if len(children) == 0 && len(n.children) > 0 {
			return nil // it held only footnote definitions, now hoisted
		}
		return &BlockQuote{Children: children, Line: Line(n.startLine)}
	case bList:
		return c.list(n)
	case bCodeBlock:
		return &CodeBlock{Fenced: n.fenced, Info: n.info, Text: n.literal, Line: Line(n.startLine)}
	case bHTMLBlock:
		c.warn(n.startLine, "raw_html", "raw HTML block is shown as text")
		return &HTMLBlock{Raw: n.literal, Line: Line(n.startLine)}
	case bTable:
		t := &Table{Align: n.align, Line: Line(n.startLine), Rows: make([][][]Inline, 0, len(n.rows)-1)}
		for k, row := range n.rows {
			// Short rows were stored without their padding; the cells past
			// the written ones stay nil (an empty cell).
			cells := make([][]Inline, len(n.align))
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

// list converts a list. Footnote definitions inside it are hoisted into
// Document.Footnotes; an item left with nothing is dropped, and so is a list
// left with no item. When anything was dropped, tightness is decided again
// over what remains (see keptTight). An item that was empty in the
// manuscript ("- ", "- [ ]") stays.
func (c *converter) list(n *bnode) Block {
	l := &List{Ordered: n.list.ordered, Start: n.list.start, Tight: n.list.tight}
	keepItem := make([]bool, len(n.children))
	keepChild := make([][]bool, len(n.children))
	dropped := false
	for i, it := range n.children {
		var children []Block
		keep := make([]bool, len(it.children))
		for j, ch := range it.children {
			if b := c.block(ch, false); b != nil {
				children = append(children, b)
				keep[j] = true
			} else {
				dropped = true
			}
		}
		keepChild[i] = keep
		if len(children) == 0 && len(it.children) > 0 {
			continue
		}
		keepItem[i] = true
		if l.Items == nil {
			l.Line = Line(it.startLine)
		}
		l.Items = append(l.Items, &ListItem{Task: it.task, Children: children, Line: Line(it.startLine)})
	}
	if len(l.Items) == 0 {
		return nil
	}
	if dropped {
		l.Tight = keptTight(n.children, keepItem, keepChild)
	}
	return l
}

// keptTight is listIsTight over the items and children that remain after
// footnote definitions were hoisted out. A dropped block is transparent: a
// blank line before or after it still separates the blocks that remain on
// either side, but one at the very end of the list separates nothing.
func keptTight(items []*bnode, keepItem []bool, keepChild [][]bool) bool {
	lastItem := -1
	for i := range items {
		if keepItem[i] {
			lastItem = i
		}
	}
	gap := false  // a blank line follows the last item kept so far
	seen := false // an item was kept
	for i, it := range items {
		if !keepItem[i] {
			// The blank line may be recorded on the item or on its last
			// block (a footnote definition keeps it on itself).
			gap = gap || seen && (it.lastLineBlank || endsWithBlankLine(it.lastChild()))
			continue
		}
		seen = true
		if gap {
			return false
		}
		gap = it.lastLineBlank
		// Within the item: a blank line after a child counts unless nothing
		// visible follows it in the list.
		pending, seenKept := false, false
		for j, ch := range it.children {
			if keepChild[i][j] {
				if pending {
					return false
				}
				seenKept = true
			}
			if seenKept && endsWithBlankLine(ch) {
				pending = true
			}
		}
		if pending && i != lastItem {
			return false
		}
	}
	return true
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
		case iFootnoteRef:
			if c.inImage > 0 {
				// An image description is shown as plain text, where a
				// reference could not be seen: it stays the text it was,
				// as for an undefined label, and numbers no footnote.
				tb.WriteString("[^" + unescapeString(n.lit) + "]")
				pending = true
				break
			}
			flush()
			out = append(out, c.inline(n))
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
		c.inImage++
		alt := c.inlineList(n.first)
		c.inImage--
		return &Image{Dest: n.dest, Title: n.title, Alt: alt}
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
