package markdown

import (
	"bytes"
	"strings"
)

// The block phase is a line-by-line port of the reference implementations'
// algorithm (CommonMark 0.31.2 Appendix A "A parsing strategy"; commonmark.js
// blocks.js for the structure, cmark blocks.c for blank-line bookkeeping and
// list tightness, cmark-gfm for the table and footnote-definition blocks).

const (
	codeIndent = 4

	// maxBlockDepth bounds how many container blocks (block quotes, lists,
	// list items, footnote definitions) may nest. Past it a marker is plain
	// text. cmark-gfm bounds list nesting the same way (MAX_LIST_DEPTH);
	// without a bound a hostile manuscript of 100,000 '>' builds a tree that
	// every consumer would have to walk recursively.
	maxBlockDepth = 100

	// maxAutocompleted bounds the empty cells added to pad short table rows
	// (cmark-gfm's MAX_AUTOCOMPLETED_CELLS). A wide header followed by many
	// one-cell rows would otherwise make the tree quadratic in the input.
	maxAutocompleted = 0x80000
)

type bkind uint8

const (
	bDocument bkind = iota
	bBlockQuote
	bList
	bItem
	bFootnote
	bParagraph
	bHeading
	bThematicBreak
	bCodeBlock
	bHTMLBlock
	bTable
)

type listData struct {
	ordered      bool
	bulletChar   byte
	start        int
	delim        byte
	padding      int
	markerOffset int
	tight        bool
}

type tableRow struct {
	cells []string
	line  int
}

// bnode is a block while the document is being parsed.
type bnode struct {
	kind      bkind
	parent    *bnode
	children  []*bnode
	open      bool
	startLine int
	depth     int // container blocks from the document down to this node

	lastLineBlank bool

	buf         []byte // accumulated lines
	content     string // after finalize (paragraphs, headings)
	contentLine int    // manuscript line of content's first line
	hadRefDefs  bool

	level int // heading

	fenced      bool // code block
	fenceChar   byte
	fenceLen    int
	fenceOffset int
	info        string
	literal     string

	htmlType int

	list *listData // list and item
	task *bool     // item

	label string // footnote definition

	align         []Align // table
	rows          []tableRow
	autocompleted int
}

func (b *bnode) lastChild() *bnode {
	if len(b.children) == 0 {
		return nil
	}
	return b.children[len(b.children)-1]
}

func isContainer(k bkind) bool {
	return k == bBlockQuote || k == bList || k == bItem || k == bFootnote
}

func canContain(parent, child bkind) bool {
	switch parent {
	case bDocument, bBlockQuote, bItem, bFootnote:
		return child != bItem
	case bList:
		return child == bItem
	}
	return false
}

func acceptsLines(k bkind) bool {
	return k == bParagraph || k == bCodeBlock || k == bHTMLBlock
}

type linkRef struct {
	dest, title string
}

type blockParser struct {
	doc, tip, oldtip *bnode

	line                 string
	lineNumber           int
	offset, column       int
	nextNonspace         int
	nextNonspaceColumn   int
	indent               int
	indented, blank      bool
	partiallyConsumedTab bool
	allClosed            bool
	lastMatchedContainer *bnode
	refs                 map[string]linkRef
	footnotes            map[string]*bnode
	ext                  bool // GFM tables, task lists, footnote definitions
}

func newBlockParser(firstLine int) *blockParser {
	doc := &bnode{kind: bDocument, open: true, startLine: firstLine}
	return &blockParser{
		doc:        doc,
		tip:        doc,
		oldtip:     doc,
		lineNumber: firstLine - 1,
		allClosed:  true,
		refs:       map[string]linkRef{},
		footnotes:  map[string]*bnode{},
	}
}

// parse feeds every line of s; line endings are LF, CRLF or CR (§2.1).
func (p *blockParser) parse(s string) {
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] != '\n' && s[j] != '\r' {
			j++
		}
		line := s[i:j]
		if j < len(s) {
			if s[j] == '\r' && j+1 < len(s) && s[j+1] == '\n' {
				j += 2
			} else {
				j++
			}
		}
		p.incorporateLine(line)
		i = j
	}
	for p.tip != nil {
		p.finalize(p.tip, p.lineNumber)
	}
}

func (p *blockParser) peek(i int) int {
	if i < len(p.line) {
		return int(p.line[i])
	}
	return -1
}

func (p *blockParser) findNextNonspace() {
	i, cols := p.offset, p.column
	for i < len(p.line) {
		c := p.line[i]
		if c == ' ' {
			i++
			cols++
		} else if c == '\t' {
			i++
			cols += 4 - cols%4
		} else {
			break
		}
	}
	p.blank = i >= len(p.line)
	p.nextNonspace = i
	p.nextNonspaceColumn = cols
	p.indent = cols - p.column
	p.indented = p.indent >= codeIndent
}

func (p *blockParser) advanceNextNonspace() {
	p.offset = p.nextNonspace
	p.column = p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

// advanceOffset moves count characters, or count columns when columns is
// set, in which case a tab may be consumed only partially (§2.2).
func (p *blockParser) advanceOffset(count int, columns bool) {
	for count > 0 && p.offset < len(p.line) {
		if p.line[p.offset] == '\t' {
			charsToTab := 4 - p.column%4
			if columns {
				p.partiallyConsumedTab = charsToTab > count
				adv := charsToTab
				if adv > count {
					adv = count
				}
				p.column += adv
				if !p.partiallyConsumedTab {
					p.offset++
				}
				count -= adv
			} else {
				p.partiallyConsumedTab = false
				p.column += charsToTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

func (p *blockParser) addLine() {
	if p.partiallyConsumedTab {
		p.offset++ // skip over the tab and add its remaining columns as spaces
		charsToTab := 4 - p.column%4
		p.tip.buf = append(p.tip.buf, strings.Repeat(" ", charsToTab)...)
	}
	p.tip.buf = append(p.tip.buf, p.line[p.offset:]...)
	p.tip.buf = append(p.tip.buf, '\n')
}

func (p *blockParser) addChild(kind bkind) *bnode {
	for !canContain(p.tip.kind, kind) {
		p.finalize(p.tip, p.lineNumber-1)
	}
	n := &bnode{kind: kind, parent: p.tip, open: true, startLine: p.lineNumber, contentLine: p.lineNumber, depth: p.tip.depth}
	if isContainer(kind) {
		n.depth++
	}
	p.tip.children = append(p.tip.children, n)
	p.tip = n
	return n
}

func (p *blockParser) closeUnmatchedBlocks() {
	if !p.allClosed {
		for p.oldtip != p.lastMatchedContainer {
			parent := p.oldtip.parent
			p.finalize(p.oldtip, p.lineNumber-1)
			p.oldtip = parent
		}
		p.allClosed = true
	}
}

func (p *blockParser) incorporateLine(ln string) {
	allMatched := true
	container := p.doc
	p.oldtip = p.tip
	p.offset, p.column = 0, 0
	p.blank = false
	p.partiallyConsumedTab = false
	p.lineNumber++
	if strings.IndexByte(ln, 0) >= 0 {
		ln = strings.ReplaceAll(ln, "\x00", "\uFFFD") // §2.3 insecure characters
	}
	p.line = ln

	// Each open block, from the document down, tries to continue on this line.
	for {
		last := container.lastChild()
		if last == nil || !last.open {
			break
		}
		container = last
		p.findNextNonspace()
		switch p.continueBlock(container) {
		case 1:
			allMatched = false
		case 2: // closing code fence: the line is done
			return
		}
		if !allMatched {
			container = container.parent
			break
		}
	}
	p.allClosed = container == p.oldtip
	p.lastMatchedContainer = container

	matchedLeaf := container.kind != bParagraph && acceptsLines(container.kind)
	for !matchedLeaf {
		p.findNextNonspace()
		res := p.tryBlockStarts(container)
		if res == 0 {
			p.advanceNextNonspace()
			break
		}
		container = p.tip
		if res == 2 {
			matchedLeaf = true
		}
	}

	// What remains at the offset is text for the container.
	p.markLastLineBlank(container)

	if !p.allClosed && !p.blank && p.tip.kind == bParagraph {
		p.addLine() // lazy continuation line (§5.1)
		return
	}
	p.closeUnmatchedBlocks()
	switch t := container.kind; {
	case acceptsLines(t):
		p.addLine()
		if t == bHTMLBlock && container.htmlType >= 1 && container.htmlType <= 5 &&
			htmlBlockEnds(container.htmlType, p.line[p.offset:]) {
			p.finalize(container, p.lineNumber)
		}
	case p.offset < len(ln) && !p.blank:
		if t == bTable {
			if container.autocompleted <= maxAutocompleted {
				p.addTableRow(container)
				return
			}
			p.finalize(container, p.lineNumber-1)
		}
		p.addChild(bParagraph)
		p.advanceNextNonspace()
		p.addLine()
	}
}

// markLastLineBlank keeps cmark's last-line-blank flags, which decide list
// tightness (§5.3). Block quote lines start with '>', so they are never
// blank; blank lines in fenced code do not count; an empty list item's own
// first line does not count either.
func (p *blockParser) markLastLineBlank(container *bnode) {
	if p.blank && container.lastChild() != nil {
		container.lastChild().lastLineBlank = true
	}
	t := container.kind
	llb := p.blank && t != bBlockQuote && t != bHeading && t != bThematicBreak &&
		!(t == bCodeBlock && container.fenced) &&
		!(t == bItem && len(container.children) == 0 && container.startLine == p.lineNumber)
	container.lastLineBlank = llb
	for c := container; c.parent != nil; c = c.parent {
		c.parent.lastLineBlank = false
	}
}

// continueBlock returns 0 when the block continues on this line, 1 when it
// does not, 2 when the line closed a fenced code block.
func (p *blockParser) continueBlock(c *bnode) int {
	switch c.kind {
	case bDocument, bList:
		return 0
	case bBlockQuote:
		if !p.indented && p.peek(p.nextNonspace) == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if q := p.peek(p.offset); q == ' ' || q == '\t' {
				p.advanceOffset(1, true)
			}
			return 0
		}
		return 1
	case bItem:
		if p.blank {
			if len(c.children) == 0 {
				return 1 // a blank line after an empty item ends it
			}
			p.advanceNextNonspace()
		} else if p.indent >= c.list.markerOffset+c.list.padding {
			p.advanceOffset(c.list.markerOffset+c.list.padding, true)
		} else {
			return 1
		}
		return 0
	case bFootnote:
		// cmark-gfm: continuation lines are indented four columns.
		if p.blank {
			p.advanceNextNonspace()
			return 0
		}
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return 0
		}
		return 1
	case bHeading, bThematicBreak:
		return 1
	case bCodeBlock:
		if c.fenced {
			if p.indent <= 3 && p.peek(p.nextNonspace) == int(c.fenceChar) {
				rest := p.line[p.nextNonspace:]
				n := 0
				for n < len(rest) && rest[n] == c.fenceChar {
					n++
				}
				if n >= c.fenceLen && isBlank(rest[n:]) {
					p.finalize(c, p.lineNumber)
					return 2
				}
			}
			for i := c.fenceOffset; i > 0; i-- {
				if q := p.peek(p.offset); q != ' ' && q != '\t' {
					break
				}
				p.advanceOffset(1, true)
			}
			return 0
		}
		if p.indent >= codeIndent {
			p.advanceOffset(codeIndent, true)
		} else if p.blank {
			p.advanceNextNonspace()
		} else {
			return 1
		}
		return 0
	case bHTMLBlock:
		if p.blank && (c.htmlType == 6 || c.htmlType == 7) {
			return 1
		}
		return 0
	case bParagraph:
		if p.blank {
			return 1
		}
		return 0
	case bTable:
		if p.blank {
			return 1
		}
		if _, ok := parseTableRow(p.line[p.nextNonspace:]); !ok {
			return 1
		}
		return 0
	}
	return 1
}

// tryBlockStarts tries each block start in the order of commonmark.js, then
// the extensions in cmark-gfm's order. It returns 0 when none starts, 1 for
// a new container, 2 for a new leaf.
func (p *blockParser) tryBlockStarts(container *bnode) int {
	rest := p.line[p.nextNonspace:]
	var c byte
	if len(rest) > 0 {
		c = rest[0]
	}

	// Block quote.
	if !p.indented && c == '>' && container.depth < maxBlockDepth {
		p.advanceNextNonspace()
		p.advanceOffset(1, false)
		if q := p.peek(p.offset); q == ' ' || q == '\t' {
			p.advanceOffset(1, true)
		}
		p.closeUnmatchedBlocks()
		p.addChild(bBlockQuote)
		return 1
	}

	// ATX heading.
	if !p.indented && c == '#' {
		n := 0
		for n < len(rest) && n < 7 && rest[n] == '#' {
			n++
		}
		if n <= 6 && (n == len(rest) || isSpaceOrTab(rest[n])) {
			p.advanceNextNonspace()
			p.advanceOffset(n, false)
			p.closeUnmatchedBlocks()
			h := p.addChild(bHeading)
			h.level = n
			h.content = stripClosingHashes(p.line[p.offset:])
			p.advanceOffset(len(p.line)-p.offset, false)
			return 2
		}
	}

	// Fenced code block.
	if !p.indented && (c == '`' || c == '~') {
		n := 0
		for n < len(rest) && rest[n] == c {
			n++
		}
		if n >= 3 && !(c == '`' && strings.IndexByte(rest[n:], '`') >= 0) {
			p.closeUnmatchedBlocks()
			cb := p.addChild(bCodeBlock)
			cb.fenced = true
			cb.fenceLen = n
			cb.fenceChar = c
			cb.fenceOffset = p.indent
			p.advanceNextNonspace()
			p.advanceOffset(n, false)
			return 2
		}
	}

	// HTML block.
	if !p.indented && c == '<' {
		lazy := !p.allClosed && !p.blank && p.tip.kind == bParagraph
		if t := htmlBlockStartType(rest, container.kind == bParagraph || lazy); t > 0 {
			p.closeUnmatchedBlocks()
			b := p.addChild(bHTMLBlock)
			b.htmlType = t
			return 2
		}
	}

	// Setext heading underline.
	if !p.indented && container.kind == bParagraph && (c == '=' || c == '-') {
		if lev := setextLevel(rest); lev > 0 {
			p.closeUnmatchedBlocks()
			content := string(container.buf)
			content = p.extractRefDefs(container, content)
			container.buf = []byte(content)
			if !isBlank(content) {
				container.kind = bHeading
				container.level = lev
				container.content = content
				container.startLine = container.contentLine
				p.advanceOffset(len(p.line)-p.offset, false)
				return 2
			}
		}
	}

	// Thematic break.
	if !p.indented && (c == '*' || c == '-' || c == '_') && isThematicBreak(rest) {
		p.closeUnmatchedBlocks()
		p.addChild(bThematicBreak)
		p.advanceOffset(len(p.line)-p.offset, false)
		return 2
	}

	// Footnote definition (cmark-gfm: before list items, may interrupt a
	// paragraph).
	if p.ext && !p.indented && c == '[' && container.depth < maxBlockDepth {
		if label, n := scanFootnoteDefStart(rest); n > 0 {
			p.closeUnmatchedBlocks()
			p.advanceNextNonspace()
			p.advanceOffset(n, false)
			f := p.addChild(bFootnote)
			f.label = label
			// Registered when opened, so that the first definition in the
			// manuscript wins even when definitions nest.
			if key := normalizeLabel(label); p.footnotes[key] == nil {
				p.footnotes[key] = f
			}
			return 1
		}
	}

	// List item.
	if container.depth+2 <= maxBlockDepth {
		if data := p.parseListMarker(container); data != nil {
			p.closeUnmatchedBlocks()
			if p.tip.kind != bList || !listsMatch(p.tip.list, data) {
				l := p.addChild(bList)
				ld := *data
				l.list = &ld
			}
			it := p.addChild(bItem)
			it.list = data
			return 1
		}
	}

	// Indented code block.
	if p.indented && p.tip.kind != bParagraph && !p.blank {
		p.advanceOffset(codeIndent, true)
		p.closeUnmatchedBlocks()
		p.addChild(bCodeBlock)
		return 2
	}

	// GFM table: a delimiter row under a paragraph whose last line is a
	// header row with as many cells.
	if p.ext && !p.indented && container.kind == bParagraph && p.tip == container {
		if aligns, ok := parseDelimiterRow(rest); ok {
			if p.openTable(container, aligns) {
				p.advanceOffset(len(p.line)-p.offset, false)
				return 2
			}
		}
	}
	return 0
}

func listsMatch(a, b *listData) bool {
	return a.ordered == b.ordered && a.delim == b.delim && a.bulletChar == b.bulletChar
}

// parseListMarker reads a list marker (§5.2) and moves past it and the
// spaces that set the item's content column.
func (p *blockParser) parseListMarker(container *bnode) *listData {
	if p.indent >= 4 {
		return nil
	}
	rest := p.line[p.nextNonspace:]
	if len(rest) == 0 {
		return nil
	}
	d := &listData{tight: true, markerOffset: p.indent}
	mlen := 0
	switch c := rest[0]; {
	case c == '-' || c == '+' || c == '*':
		d.bulletChar = c
		mlen = 1
	case isDigit(c):
		k := 0
		start := 0
		for k < len(rest) && isDigit(rest[k]) {
			if k == 9 {
				return nil
			}
			start = start*10 + int(rest[k]-'0')
			k++
		}
		if k >= len(rest) || rest[k] != '.' && rest[k] != ')' {
			return nil
		}
		// Only a list starting at 1 may interrupt a paragraph.
		if container.kind == bParagraph && start != 1 {
			return nil
		}
		d.ordered = true
		d.start = start
		d.delim = rest[k]
		mlen = k + 1
	default:
		return nil
	}
	if mlen < len(rest) && !isSpaceOrTab(rest[mlen]) {
		return nil
	}
	// An empty item may not interrupt a paragraph.
	if container.kind == bParagraph && isBlank(rest[mlen:]) {
		return nil
	}
	p.advanceNextNonspace()
	p.advanceOffset(mlen, true)
	spacesStartCol, spacesStartOffset := p.column, p.offset
	for {
		p.advanceOffset(1, true)
		q := p.peek(p.offset)
		if !(p.column-spacesStartCol < 5 && (q == ' ' || q == '\t')) {
			break
		}
	}
	blankItem := p.offset >= len(p.line)
	spacesAfter := p.column - spacesStartCol
	if spacesAfter >= 5 || spacesAfter < 1 || blankItem {
		d.padding = mlen + 1
		p.column = spacesStartCol
		p.offset = spacesStartOffset
		p.partiallyConsumedTab = false
		if q := p.peek(p.offset); q == ' ' || q == '\t' {
			p.advanceOffset(1, true)
		}
	} else {
		d.padding = mlen + spacesAfter
	}
	return d
}

func (p *blockParser) finalize(b *bnode, line int) {
	above := b.parent
	b.open = false
	switch b.kind {
	case bParagraph:
		p.finalizeParagraph(b)
	case bCodeBlock:
		content := string(b.buf)
		if b.fenced {
			nl := strings.IndexByte(content, '\n')
			b.info = unescapeString(trimSpaceTab(content[:nl]))
			b.literal = content[nl+1:]
		} else {
			end := len(content)
			for end > 0 {
				ls := strings.LastIndexByte(content[:end-1], '\n') + 1
				if !isBlank(content[ls : end-1]) {
					break
				}
				end = ls
			}
			b.literal = content[:end]
		}
		b.buf = nil
	case bHTMLBlock:
		b.literal = strings.TrimSuffix(string(b.buf), "\n")
		b.buf = nil
	case bList:
		b.list.tight = listIsTight(b)
	}
	p.tip = above
}

func (p *blockParser) finalizeParagraph(b *bnode) {
	content := string(b.buf)
	b.buf = nil
	stripped := false
	if p.ext && b.parent.kind == bItem && b.parent.task == nil && len(b.parent.children) > 0 &&
		b.parent.children[0] == b && b.startLine == b.parent.startLine {
		if checked, n := scanTaskMarker(content); n > 0 {
			b.parent.task = &checked
			content = content[n:]
			stripped = true
		}
	}
	content = p.extractRefDefs(b, content)
	b.content = content
	if (b.hadRefDefs || stripped) && isBlank(content) {
		removeLastChild(b.parent, b)
	}
}

// scanTaskMarker reads a GFM task list item marker at the start of an item's
// first paragraph: "[ ]", "[x]" or "[X]" followed by whitespace or the end.
func scanTaskMarker(s string) (checked bool, n int) {
	if len(s) < 3 || s[0] != '[' || s[2] != ']' {
		return false, 0
	}
	switch s[1] {
	case ' ', '\t':
	case 'x', 'X':
		checked = true
	default:
		return false, 0
	}
	if len(s) > 3 && !isWhitespaceByte(s[3]) {
		return false, 0
	}
	return checked, 3
}

func removeLastChild(parent, b *bnode) {
	for i := len(parent.children) - 1; i >= 0; i-- {
		if parent.children[i] == b {
			parent.children = append(parent.children[:i], parent.children[i+1:]...)
			return
		}
	}
}

// extractRefDefs takes link reference definitions off the front of a
// paragraph's content (§4.7) and returns what is left.
func (p *blockParser) extractRefDefs(b *bnode, content string) string {
	for len(content) > 0 && content[0] == '[' {
		n := p.parseReference(content)
		if n == 0 {
			break
		}
		b.contentLine += strings.Count(content[:n], "\n")
		content = content[n:]
		b.hadRefDefs = true
	}
	return content
}

// parseReference reads one link reference definition at the start of s and
// records it; it returns the bytes consumed, or 0.
func (p *blockParser) parseReference(s string) int {
	end := scanLinkLabel(s, 0)
	if end < 0 || end >= len(s) || s[end] != ':' {
		return 0
	}
	rawLabel := s[1 : end-1]
	pos := skipSpnl(s, end+1)
	dest, pos2, ok := scanLinkDest(s, pos)
	if !ok || pos2 == pos && s[pos] != '<' {
		return 0
	}
	beforeTitle := pos2
	pos = skipSpnl(s, pos2)
	title, hasTitle := "", false
	if pos != beforeTitle {
		if t, p3, ok := scanLinkTitle(s, pos); ok {
			title, hasTitle, pos = t, true, p3
		}
	}
	if !hasTitle {
		pos = beforeTitle
	}
	e, ok := atLineEnd(s, pos)
	if !ok {
		if !hasTitle {
			return 0
		}
		// The title is not at the line end; the definition may still stand
		// without it.
		title = ""
		if e, ok = atLineEnd(s, beforeTitle); !ok {
			return 0
		}
	}
	key := normalizeLabel(rawLabel)
	if key == "" {
		return 0
	}
	if _, exists := p.refs[key]; !exists {
		p.refs[key] = linkRef{dest: dest, title: title}
	}
	return e
}

func atLineEnd(s string, pos int) (int, bool) {
	for pos < len(s) && isSpaceOrTab(s[pos]) {
		pos++
	}
	if pos == len(s) {
		return pos, true
	}
	if s[pos] == '\n' {
		return pos + 1, true
	}
	return pos, false
}

// listIsTight follows cmark: a list is loose if a non-final item ends with a
// blank line, or blank lines separate two children of an item.
func listIsTight(b *bnode) bool {
	for i, item := range b.children {
		last := i == len(b.children)-1
		if item.lastLineBlank && !last {
			return false
		}
		for j, sub := range item.children {
			if (!last || j < len(item.children)-1) && endsWithBlankLine(sub) {
				return false
			}
		}
	}
	return true
}

func endsWithBlankLine(b *bnode) bool {
	for b != nil {
		if b.lastLineBlank {
			return true
		}
		if b.kind != bList && b.kind != bItem {
			return false
		}
		b = b.lastChild()
	}
	return false
}

func stripClosingHashes(s string) string {
	t := strings.TrimRight(s, " \t")
	k := len(t)
	for k > 0 && t[k-1] == '#' {
		k--
	}
	if k == len(t) {
		return s
	}
	if isBlank(t[:k]) {
		return ""
	}
	if isSpaceOrTab(t[k-1]) {
		return t[:k]
	}
	return s
}

func setextLevel(s string) int {
	if s == "" || s[0] != '=' && s[0] != '-' {
		return 0
	}
	c := s[0]
	i := 0
	for i < len(s) && s[i] == c {
		i++
	}
	for i < len(s) && isSpaceOrTab(s[i]) {
		i++
	}
	if i != len(s) {
		return 0
	}
	if c == '=' {
		return 1
	}
	return 2
}

func isThematicBreak(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if c != '*' && c != '-' && c != '_' {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case c:
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// scanFootnoteDefStart matches cmark-gfm's footnote definition opener,
// "[^" label "]:" followed by optional spaces or tabs; the label has no
// whitespace or ']'.
func scanFootnoteDefStart(s string) (string, int) {
	if len(s) < 5 || s[0] != '[' || s[1] != '^' {
		return "", 0
	}
	i := 2
	for i < len(s) && s[i] != ']' && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
		i++
	}
	if i == 2 || i+1 >= len(s) || s[i] != ']' || s[i+1] != ':' {
		return "", 0
	}
	label := s[2:i]
	i += 2
	for i < len(s) && isSpaceOrTab(s[i]) {
		i++
	}
	return label, i
}

// openTable turns the paragraph's last line into a table header when it has
// as many cells as the delimiter row; earlier lines stay a paragraph.
func (p *blockParser) openTable(para *bnode, aligns []Align) bool {
	// Only the last line is converted: a paragraph of near-miss delimiter
	// rows would otherwise be copied once per line.
	if len(para.buf) == 0 { // emptied by link reference definitions
		return false
	}
	body := para.buf[:len(para.buf)-1]
	nl := bytes.LastIndexByte(body, '\n')
	cells, ok := parseTableRow(string(body[nl+1:]))
	if !ok || len(cells) != len(aligns) {
		return false
	}
	headerLine := p.lineNumber - 1
	var t *bnode
	if nl >= 0 {
		para.buf = para.buf[:nl+1]
		p.finalize(para, headerLine-1)
		t = p.addChild(bTable)
		t.startLine = headerLine
	} else {
		t = para
		t.kind = bTable
		t.buf = nil
	}
	t.align = aligns
	t.rows = []tableRow{{cells: cells, line: headerLine}}
	return true
}

func (p *blockParser) addTableRow(t *bnode) {
	cells, _ := parseTableRow(p.line[p.offset:])
	n := len(t.align)
	if len(cells) > n {
		cells = cells[:n]
	}
	for len(cells) < n {
		cells = append(cells, "")
		t.autocompleted++
	}
	t.rows = append(t.rows, tableRow{cells: cells, line: p.lineNumber})
}
