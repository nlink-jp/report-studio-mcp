package markdown

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// The inline phase follows commonmark.js inlines.js and CommonMark 0.31.2
// Appendix A ("Processing emphasis"): a delimiter stack for emphasis and a
// bracket stack for links. Where the reference implementations had to fix
// quadratic behaviour (code-span closers, unclosed HTML comments, link
// opener deactivation, emphasis openers_bottom), the fix is kept here.

// maxInlineDepth bounds how deep emphasis, links and images may nest. Deeper
// matches keep their text but lose their markup, so the tree never grows a
// depth that a recursive consumer could not walk.
const maxInlineDepth = 100

type ikind uint8

const (
	iText ikind = iota
	iSoftBreak
	iHardBreak
	iCode
	iEmph
	iStrong
	iStrike
	iLink
	iImage
	iHTML
	iFootnoteRef
	// iGroup holds children whose wrapping construct was dropped by the
	// depth bound; conversion splices them into the parent.
	iGroup
)

type inode struct {
	kind                    ikind
	parent                  *inode
	first, last, prev, next *inode

	lit      string // text, code, raw HTML, footnote label
	dest     string
	title    string
	autolink bool
	line     int // raw HTML
	depth    int
	footnote *bnode
}

func (n *inode) appendChild(c *inode) {
	c.parent = n
	c.prev = n.last
	c.next = nil
	if n.last != nil {
		n.last.next = c
	} else {
		n.first = c
	}
	n.last = c
}

func (n *inode) unlink() {
	if n.prev != nil {
		n.prev.next = n.next
	} else if n.parent != nil {
		n.parent.first = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else if n.parent != nil {
		n.parent.last = n.prev
	}
	n.parent, n.prev, n.next = nil, nil, nil
}

func (n *inode) insertAfter(s *inode) {
	s.unlink()
	s.next = n.next
	if s.next != nil {
		s.next.prev = s
	}
	s.prev = n
	n.next = s
	s.parent = n.parent
	if s.parent != nil && s.next == nil {
		s.parent.last = s
	}
}

func (n *inode) insertBefore(s *inode) {
	s.unlink()
	s.prev = n.prev
	if s.prev != nil {
		s.prev.next = s
	}
	s.next = n
	n.prev = s
	s.parent = n.parent
	if s.parent != nil && s.prev == nil {
		s.parent.first = s
	}
}

// adoptChildren moves the siblings after from (up to, not including, until)
// into c and sets c's depth, demoting c to a group past maxInlineDepth.
func adoptChildren(c, from, until *inode) {
	maxd := 0
	for t := from.next; t != nil && t != until; {
		next := t.next
		t.unlink()
		c.appendChild(t)
		if t.depth > maxd {
			maxd = t.depth
		}
		t = next
	}
	if maxd+1 > maxInlineDepth {
		c.kind = iGroup
		c.depth = maxd
	} else {
		c.depth = maxd + 1
	}
}

type delim struct {
	c                 byte
	num, orig         int
	node              *inode
	pos               int
	prev, next        *delim
	canOpen, canClose bool
}

type bracket struct {
	node         *inode
	prev         *bracket
	index        int // position of '['
	image        bool
	seq          int
	bracketAfter bool
}

type inlineParser struct {
	refs      map[string]linkRef
	footnotes map[string]*bnode
	ext       bool // GFM extensions and footnotes

	subj     string
	pos      int
	root     *inode
	delims   *delim
	brackets *bracket

	bracketSeq  int
	linkBarrier int // non-image openers with seq below this are inactive

	baseLine int
	newlines []int
	nlDone   bool

	tickRuns map[int][]int // backtick run length → starts, built on demand
	lastTerm map[string]int
}

func parseInlines(subj string, baseLine int, refs map[string]linkRef, footnotes map[string]*bnode, ext bool) *inode {
	ip := &inlineParser{refs: refs, footnotes: footnotes, subj: subj, root: &inode{kind: iGroup}, baseLine: baseLine, ext: ext}
	for ip.pos < len(ip.subj) {
		ip.parseInline()
	}
	ip.processEmphasis(-1)
	if ext {
		postprocessEmails(ip.root)
	}
	return ip.root
}

func (ip *inlineParser) text(s string) *inode {
	n := &inode{kind: iText, lit: s}
	ip.root.appendChild(n)
	return n
}

func (ip *inlineParser) lineAt(pos int) int {
	if !ip.nlDone {
		for i := 0; i < len(ip.subj); i++ {
			if ip.subj[i] == '\n' {
				ip.newlines = append(ip.newlines, i)
			}
		}
		ip.nlDone = true
	}
	return ip.baseLine + sort.SearchInts(ip.newlines, pos)
}

var inlineSpecial = func() (t [256]bool) {
	for _, c := range []byte("\n\\`*_~[]!<&") {
		t[c] = true
	}
	return t
}()

// autolinkBoundary: GFM extended autolinks start at the beginning of a
// line, after whitespace, or after one of * _ ~ (.
func (ip *inlineParser) autolinkBoundary(i int) bool {
	if i == 0 {
		return true
	}
	c := ip.subj[i-1]
	return isWhitespaceByte(c) || c == '*' || c == '_' || c == '~' || c == '('
}

func (ip *inlineParser) mayStartExtAutolink(i int) bool {
	c := ip.subj[i]
	// cmark-gfm makes no extended autolink while a bracket is open.
	return ip.ext && (c == 'w' || c == 'h' || c == 'H') && ip.brackets == nil && ip.autolinkBoundary(i)
}

func (ip *inlineParser) parseInline() {
	c := ip.subj[ip.pos]
	switch c {
	case '\n':
		ip.parseNewline()
	case '\\':
		ip.parseBackslash()
	case '`':
		ip.parseBackticks()
	case '*', '_':
		ip.handleDelim(c)
	case '~':
		if ip.ext {
			ip.handleTilde()
		} else {
			ip.text("~")
			ip.pos++
		}
	case '[':
		ip.text("[")
		ip.addBracket(ip.root.last, ip.pos, false)
		ip.pos++
	case '!':
		if ip.pos+1 < len(ip.subj) && ip.subj[ip.pos+1] == '[' {
			ip.text("![")
			ip.addBracket(ip.root.last, ip.pos+1, true)
			ip.pos += 2
		} else {
			ip.text("!")
			ip.pos++
		}
	case ']':
		ip.parseCloseBracket()
	case '<':
		if !ip.parseAutolink() && !ip.parseHTMLTag() {
			ip.text("<")
			ip.pos++
		}
	case '&':
		if t, n := scanEntity(ip.subj, ip.pos); n > 0 {
			ip.text(t)
			ip.pos += n
		} else {
			ip.text("&")
			ip.pos++
		}
	default:
		if ip.mayStartExtAutolink(ip.pos) && ip.parseExtAutolink() {
			return
		}
		ip.parseString()
	}
}

func (ip *inlineParser) parseString() {
	s := ip.subj
	j := ip.pos + 1
	for j < len(s) && !inlineSpecial[s[j]] && !ip.mayStartExtAutolink(j) {
		j++
	}
	t := s[ip.pos:j]
	if j < len(s) && s[j] == '\n' {
		t = strings.TrimRight(t, " \t") // final spaces of a line (§6.7, §6.8)
	}
	if t != "" {
		ip.text(t)
	}
	ip.pos = j
}

func (ip *inlineParser) skipLineStart() {
	for ip.pos < len(ip.subj) && isSpaceOrTab(ip.subj[ip.pos]) {
		ip.pos++
	}
}

func (ip *inlineParser) parseNewline() {
	nl := ip.pos
	ip.pos++
	ip.skipLineStart()
	if nl >= 2 && ip.subj[nl-1] == ' ' && ip.subj[nl-2] == ' ' {
		ip.root.appendChild(&inode{kind: iHardBreak})
	} else {
		ip.root.appendChild(&inode{kind: iSoftBreak})
	}
}

func (ip *inlineParser) parseBackslash() {
	ip.pos++
	switch {
	case ip.pos < len(ip.subj) && ip.subj[ip.pos] == '\n':
		ip.pos++
		ip.skipLineStart()
		ip.root.appendChild(&inode{kind: iHardBreak})
	case ip.pos < len(ip.subj) && isASCIIPunct(ip.subj[ip.pos]):
		ip.text(ip.subj[ip.pos : ip.pos+1])
		ip.pos++
	default:
		ip.text("\\")
	}
}

// findTickRun returns the start of the first backtick run of exactly n
// characters at or after from, or -1. All runs are indexed once, so a
// paragraph full of unmatched backticks stays linear (cmark keeps the same
// memo).
func (ip *inlineParser) findTickRun(n, from int) int {
	if ip.tickRuns == nil {
		ip.tickRuns = map[int][]int{}
		s := ip.subj
		for i := 0; i < len(s); {
			if s[i] != '`' {
				i++
				continue
			}
			j := i
			for j < len(s) && s[j] == '`' {
				j++
			}
			ip.tickRuns[j-i] = append(ip.tickRuns[j-i], i)
			i = j
		}
	}
	runs := ip.tickRuns[n]
	k := sort.SearchInts(runs, from)
	if k < len(runs) {
		return runs[k]
	}
	return -1
}

func (ip *inlineParser) parseBackticks() {
	s := ip.subj
	start := ip.pos
	for ip.pos < len(s) && s[ip.pos] == '`' {
		ip.pos++
	}
	n := ip.pos - start
	closeAt := ip.findTickRun(n, ip.pos)
	if closeAt < 0 {
		ip.text(s[start:ip.pos])
		return
	}
	contents := strings.ReplaceAll(s[ip.pos:closeAt], "\n", " ")
	if len(contents) >= 2 && contents[0] == ' ' && contents[len(contents)-1] == ' ' && strings.Trim(contents, " ") != "" {
		contents = contents[1 : len(contents)-1]
	}
	ip.root.appendChild(&inode{kind: iCode, lit: contents})
	ip.pos = closeAt + n
}

// scanDelims measures a delimiter run and its flanking (§6.2).
func (ip *inlineParser) scanDelims(c byte) (n int, left, right, before, after bool) {
	s := ip.subj
	start := ip.pos
	for ip.pos+n < len(s) && s[ip.pos+n] == c {
		n++
	}
	rb, ra := '\n', '\n'
	if start > 0 {
		rb, _ = utf8.DecodeLastRuneInString(s[:start])
	}
	if start+n < len(s) {
		ra, _ = utf8.DecodeRuneInString(s[start+n:])
	}
	afterWS, afterP := isUnicodeWhitespace(ra), isUnicodePunct(ra)
	beforeWS, beforeP := isUnicodeWhitespace(rb), isUnicodePunct(rb)
	left = !afterWS && (!afterP || beforeWS || beforeP)
	right = !beforeWS && (!beforeP || afterWS || afterP)
	return n, left, right, beforeP, afterP
}

func (ip *inlineParser) pushDelim(c byte, n int, node *inode, canOpen, canClose bool) {
	d := &delim{c: c, num: n, orig: n, node: node, pos: ip.pos, prev: ip.delims, canOpen: canOpen, canClose: canClose}
	if ip.delims != nil {
		ip.delims.next = d
	}
	ip.delims = d
}

func (ip *inlineParser) handleDelim(c byte) {
	n, left, right, beforeP, afterP := ip.scanDelims(c)
	var canOpen, canClose bool
	if c == '_' {
		canOpen = left && (!right || beforeP)
		canClose = right && (!left || afterP)
	} else {
		canOpen, canClose = left, right
	}
	node := ip.text(ip.subj[ip.pos : ip.pos+n])
	if canOpen || canClose {
		ip.pushDelim(c, n, node, canOpen, canClose)
	}
	ip.pos += n
}

// handleTilde: GFM strikethrough takes runs of one or two tildes; three or
// more never strike (GFM example 493).
func (ip *inlineParser) handleTilde() {
	n, left, right, _, _ := ip.scanDelims('~')
	node := ip.text(ip.subj[ip.pos : ip.pos+n])
	if n <= 2 && (left || right) {
		ip.pushDelim('~', n, node, left, right)
	}
	ip.pos += n
}

func (ip *inlineParser) removeDelim(d *delim) {
	if d.prev != nil {
		d.prev.next = d.next
	}
	if d.next != nil {
		d.next.prev = d.prev
	} else {
		ip.delims = d.prev
	}
	d.prev, d.next = nil, nil
}

func bottomIndex(d *delim) int {
	o := 0
	if d.canOpen {
		o = 3
	}
	switch d.c {
	case '_':
		return 2 + o + d.orig%3
	case '*':
		return 8 + o + d.orig%3
	}
	return 14 + d.orig - 1 // '~', length 1 or 2
}

// processEmphasis resolves the delimiters after position bottom (exclusive).
func (ip *inlineParser) processEmphasis(bottom int) {
	var openersBottom [16]int
	for i := range openersBottom {
		openersBottom[i] = bottom
	}
	closer := ip.delims
	if closer == nil || closer.pos <= bottom {
		return
	}
	for closer.prev != nil && closer.prev.pos > bottom {
		closer = closer.prev
	}
	for closer != nil {
		if !closer.canClose {
			closer = closer.next
			continue
		}
		idx := bottomIndex(closer)
		opener := closer.prev
		found := false
		for opener != nil && opener.pos > bottom && opener.pos > openersBottom[idx] {
			if opener.c == closer.c && opener.canOpen {
				if closer.c == '~' {
					found = opener.orig == closer.orig
				} else {
					odd := (closer.canOpen || opener.canClose) && closer.orig%3 != 0 && (opener.orig+closer.orig)%3 == 0
					found = !odd
				}
				if found {
					break
				}
			}
			opener = opener.prev
		}
		old := closer
		if found {
			closer = ip.insertEmph(opener, closer)
			continue
		}
		closer = closer.next
		openersBottom[idx] = old.pos - 1
		if !old.canOpen {
			ip.removeDelim(old)
		}
	}
	for ip.delims != nil && ip.delims.pos > bottom {
		ip.removeDelim(ip.delims)
	}
}

func (ip *inlineParser) insertEmph(opener, closer *delim) *delim {
	use := 1
	kind := iEmph
	switch {
	case closer.c == '~':
		use = closer.num
		kind = iStrike
	case closer.num >= 2 && opener.num >= 2:
		use = 2
		kind = iStrong
	}
	oi, ci := opener.node, closer.node
	opener.num -= use
	closer.num -= use
	oi.lit = oi.lit[:len(oi.lit)-use]
	ci.lit = ci.lit[:len(ci.lit)-use]
	e := &inode{kind: kind}
	adoptChildren(e, oi, ci)
	oi.insertAfter(e)
	for d := closer.prev; d != nil && d != opener; {
		p := d.prev
		ip.removeDelim(d)
		d = p
	}
	if opener.num == 0 {
		oi.unlink()
		ip.removeDelim(opener)
	}
	if closer.num == 0 {
		ci.unlink()
		next := closer.next
		ip.removeDelim(closer)
		return next
	}
	return closer
}

func (ip *inlineParser) addBracket(node *inode, index int, image bool) {
	if ip.brackets != nil {
		ip.brackets.bracketAfter = true
	}
	ip.brackets = &bracket{node: node, prev: ip.brackets, index: index, image: image, seq: ip.bracketSeq}
	ip.bracketSeq++
}

func (ip *inlineParser) parseCloseBracket() {
	s := ip.subj
	ip.pos++
	startpos := ip.pos
	opener := ip.brackets
	if opener == nil {
		ip.text("]")
		return
	}
	if !opener.image && opener.seq < ip.linkBarrier {
		// Links may not contain links (§6.3): this opener was deactivated.
		ip.text("]")
		ip.brackets = opener.prev
		return
	}

	var dest, title string
	matched := false
	if ip.pos < len(s) && s[ip.pos] == '(' {
		p := skipSpnl(s, ip.pos+1)
		if d, p2, ok := scanLinkDest(s, p); ok {
			p3 := skipSpnl(s, p2)
			t := ""
			if p3 > p2 { // a title must be separated from the destination
				if tt, p4, ok := scanLinkTitle(s, p3); ok {
					t = tt
					p3 = skipSpnl(s, p4)
				}
			}
			if p3 < len(s) && s[p3] == ')' {
				dest, title, matched = d, t, true
				ip.pos = p3 + 1
			}
		}
	}
	if !matched {
		label, have := "", false
		end := scanLinkLabel(s, ip.pos)
		if end >= 0 && end-ip.pos > 2 {
			label, have = s[ip.pos+1:end-1], true
		} else if !opener.bracketAfter {
			// Collapsed or shortcut reference: the link text is the label.
			// It cannot contain a bracket, which bracketAfter would show.
			label, have = s[opener.index+1:startpos-1], true
		}
		if have && len(label) <= 4*maxLinkLabel {
			if ref, ok := ip.refs[normalizeLabel(label)]; ok {
				dest, title, matched = ref.dest, ref.title, true
				if end >= 0 {
					ip.pos = end
				}
			}
		}
	}
	// Footnote reference. A definition's label holds no whitespace and no
	// ']' (scanFootnoteDefStart), and a reference label cannot hold an
	// unescaped '[' (it would be another opener), so such labels are not
	// normalized at all: nested "[^" openers would otherwise normalize a
	// label of up to 4 KB at every ']'.
	if !matched && ip.ext && !opener.image && !opener.bracketAfter && opener.index+1 < len(s) && s[opener.index+1] == '^' {
		label := s[opener.index+2 : startpos-1]
		if label != "" && len(label) <= 4*maxLinkLabel && isFootnoteLabel(label) {
			if def := ip.footnotes[normalizeLabel(label)]; def != nil {
				for ip.delims != nil && ip.delims.pos > opener.index {
					ip.removeDelim(ip.delims)
				}
				for opener.node.next != nil {
					opener.node.next.unlink()
				}
				ip.root.appendChild(&inode{kind: iFootnoteRef, lit: label, footnote: def})
				opener.node.unlink()
				ip.brackets = opener.prev
				ip.pos = startpos
				return
			}
		}
	}
	if !matched {
		ip.brackets = opener.prev
		ip.pos = startpos
		ip.text("]")
		return
	}

	ip.processEmphasis(opener.index)
	kind := iLink
	if opener.image {
		kind = iImage
	}
	n := &inode{kind: kind, dest: dest, title: title}
	adoptChildren(n, opener.node, nil)
	ip.root.appendChild(n)
	ip.brackets = opener.prev
	opener.node.unlink()
	if !opener.image {
		ip.linkBarrier = ip.bracketSeq
	}
}

// isFootnoteLabel reports whether a reference's label could match a
// definition: none of the bytes scanFootnoteDefStart stops at. (An escaped
// "\[" may stand in both; an unescaped '[' is excluded by bracketAfter.)
func isFootnoteLabel(label string) bool {
	for i := 0; i < len(label); i++ {
		switch label[i] {
		case ' ', '\t', '\n', '\r', ']':
			return false
		}
	}
	return true
}

// parseAutolink reads a URI or email autolink (§6.5). As cmark does, entity
// references in it are resolved; backslash escapes are not.
func (ip *inlineParser) parseAutolink() bool {
	s := ip.subj
	i := ip.pos + 1
	if i >= len(s) {
		return false
	}
	if isASCIILetter(s[i]) {
		k := i + 1
		for k < len(s) && k-i < 32 && (isAlnum(s[k]) || s[k] == '.' || s[k] == '+' || s[k] == '-') {
			k++
		}
		if k-i >= 2 && k < len(s) && s[k] == ':' {
			m := k + 1
			for m < len(s) && s[m] > ' ' && s[m] != '<' && s[m] != '>' {
				m++
			}
			if m < len(s) && s[m] == '>' {
				uri := decodeEntities(s[i:m])
				ip.addAutolink(normalizeURI(uri), uri)
				ip.pos = m + 1
				return true
			}
		}
	}
	if e := scanEmailAutolink(s, i); e > 0 {
		addr := decodeEntities(s[i:e])
		ip.addAutolink(normalizeURI("mailto:"+addr), addr)
		ip.pos = e + 1
		return true
	}
	return false
}

func (ip *inlineParser) addAutolink(dest, text string) {
	l := &inode{kind: iLink, dest: dest, autolink: true, depth: 1}
	l.appendChild(&inode{kind: iText, lit: text})
	ip.root.appendChild(l)
}

// scanEmailAutolink matches the email address of §6.5 at s[i] and returns
// the index of the closing '>', or -1.
func scanEmailAutolink(s string, i int) int {
	j := i
	for j < len(s) && (isAlnum(s[j]) || strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", s[j]) >= 0) {
		j++
	}
	if j == i || j >= len(s) || s[j] != '@' {
		return -1
	}
	j++
	for {
		start := j
		for j < len(s) && (isAlnum(s[j]) || s[j] == '-') && j-start < 63 {
			j++
		}
		if j == start || s[start] == '-' || s[j-1] == '-' {
			return -1
		}
		if j < len(s) && s[j] == '.' {
			j++
			continue
		}
		break
	}
	if j < len(s) && s[j] == '>' {
		return j
	}
	return -1
}

func decodeEntities(s string) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '&' {
			if t, n := scanEntity(s, i); n > 0 {
				b.WriteString(t)
				i += n
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// findTerm finds term at or after from. The last occurrence is looked up
// once, so the many attempts an unclosed "<!--" invites cost nothing after
// the first: cmark keeps the same memo for comments, processing instructions,
// declarations and CDATA.
func (ip *inlineParser) findTerm(term string, from int) int {
	if ip.lastTerm == nil {
		ip.lastTerm = map[string]int{}
	}
	last, ok := ip.lastTerm[term]
	if !ok {
		last = strings.LastIndex(ip.subj, term)
		ip.lastTerm[term] = last
	}
	if last < from {
		return -1
	}
	return from + strings.Index(ip.subj[from:], term)
}

func (ip *inlineParser) parseHTMLTag() bool {
	s, i := ip.subj, ip.pos
	end := -1
	rest := s[i:]
	switch {
	case strings.HasPrefix(rest, "<!-->"):
		end = i + 5
	case strings.HasPrefix(rest, "<!--->"):
		end = i + 6
	case strings.HasPrefix(rest, "<!--"):
		if k := ip.findTerm("-->", i+4); k >= 0 {
			end = k + 3
		}
	case strings.HasPrefix(rest, "<?"):
		if k := ip.findTerm("?>", i+2); k >= 0 {
			end = k + 2
		}
	case strings.HasPrefix(rest, "<![CDATA["):
		if k := ip.findTerm("]]>", i+9); k >= 0 {
			end = k + 3
		}
	case len(rest) > 2 && rest[1] == '!' && isASCIILetter(rest[2]):
		if k := ip.findTerm(">", i+2); k >= 0 {
			end = k + 1
		}
	case len(rest) > 1 && rest[1] == '/':
		end = scanClosingTag(s, i)
	default:
		end = scanOpenTag(s, i)
	}
	if end < 0 {
		return false
	}
	ip.root.appendChild(&inode{kind: iHTML, lit: s[i:end], line: ip.lineAt(i)})
	ip.pos = end
	return true
}
