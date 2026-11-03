// Package markdown parses a report manuscript — CommonMark 0.31.2 with the
// GitHub extensions of GFM 0.29-gfm (tables, strikethrough, task lists,
// extended autolinks), footnotes and alerts as GitHub Docs describes them, and
// an optional front matter block — into the tree the typesetters consume.
//
// Versions and sources are pinned in docs/ja/phase0-criteria.ja.md (P0-5).
// Nothing here is ever an error: Markdown has no invalid input. What the
// typesetters cannot show faithfully (raw HTML, headings deeper than 3, an
// unknown front-matter key) is reported as a Warning with its line.
package markdown

// Node is any node of the tree.
type Node interface{ node() }

// Block is a block-level node.
type Block interface {
	Node
	block()
}

// Inline is an inline-level node.
type Inline interface {
	Node
	inline()
}

// Document is a parsed manuscript.
type Document struct {
	// Meta holds the front matter's key: value pairs, keys lower-cased.
	Meta map[string]string
	// Children are the top-level blocks.
	Children []Block
	// Footnotes are the definitions that some reference uses, in the order of
	// their first reference; Index is that order, from 1. Unreferenced
	// definitions are dropped, as GitHub does.
	Footnotes []*Footnote
	// Warnings are what the typesetters should tell the caller.
	Warnings []Warning
}

// Warning is a problem worth reporting that does not stop the document.
type Warning struct {
	Line    int    // 1-based line in the manuscript
	Code    string // stable slug, e.g. "raw_html", "deep_heading", "unknown_front_matter_key"
	Message string
}

// Footnote is one footnote definition.
type Footnote struct {
	Label    string // as written, e.g. "1" or "note"
	Index    int    // 1-based display number
	Children []Block
}

// Line is the 1-based manuscript line a block starts on.
type Line int

// Heading is an ATX or setext heading.
type Heading struct {
	Level    int // 1..6
	Children []Inline
	Line     Line
}

// Paragraph is a paragraph.
type Paragraph struct {
	Children []Inline
	Line     Line
}

// ThematicBreak is a horizontal rule.
type ThematicBreak struct{ Line Line }

// BlockQuote is a block quote that is not an alert.
type BlockQuote struct {
	Children []Block
	Line     Line
}

// AlertKind is the kind of a GitHub alert.
type AlertKind string

// The five alert kinds GitHub Docs lists.
const (
	AlertNote      AlertKind = "NOTE"
	AlertTip       AlertKind = "TIP"
	AlertImportant AlertKind = "IMPORTANT"
	AlertWarning   AlertKind = "WARNING"
	AlertCaution   AlertKind = "CAUTION"
)

// Alert is a block quote whose first line is [!KIND].
type Alert struct {
	Kind     AlertKind
	Children []Block
	Line     Line
}

// List is a bullet or ordered list.
type List struct {
	Ordered bool
	Start   int  // first number of an ordered list
	Tight   bool // CommonMark tightness: items render without paragraph spacing
	Items   []*ListItem
	Line    Line
}

// ListItem is one item. Task is nil for an ordinary item, else whether the
// task box is checked.
type ListItem struct {
	Task     *bool
	Children []Block
	Line     Line
}

// CodeBlock is an indented or fenced code block. Info is the fence's info
// string (its first word is the language, e.g. "chart" or "mermaid").
type CodeBlock struct {
	Fenced bool
	Info   string
	Text   string // content, each line ending in "\n"
	Line   Line
}

// HTMLBlock is a raw HTML block, kept verbatim; the typesetters show it as
// text and warn.
type HTMLBlock struct {
	Raw  string
	Line Line
}

// Align is a table column's alignment.
type Align int

// Column alignments.
const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Table is a GFM table. Every row has len(Align) cells.
type Table struct {
	Align  []Align
	Header [][]Inline
	Rows   [][][]Inline
	Line   Line
}

// Text is literal text, entities and escapes already resolved.
type Text struct{ Value string }

// Code is a code span.
type Code struct{ Value string }

// Emph is emphasis.
type Emph struct{ Children []Inline }

// Strong is strong emphasis.
type Strong struct{ Children []Inline }

// Strike is GFM strikethrough.
type Strike struct{ Children []Inline }

// Link is a link or an autolink. Dest is the destination after unescaping and
// percent-encoding as CommonMark's reference HTML renderer does.
type Link struct {
	Dest     string
	Title    string
	Children []Inline
	Autolink bool
}

// Image is an image. Alt is its description as inlines (rendered as plain
// text where a caption is needed).
type Image struct {
	Dest  string
	Title string
	Alt   []Inline
}

// RawHTML is inline raw HTML, kept verbatim.
type RawHTML struct{ Raw string }

// SoftBreak is a line ending inside a paragraph.
type SoftBreak struct{}

// HardBreak is a forced line break.
type HardBreak struct{}

// FootnoteRef is a reference to Document.Footnotes[Index-1].
type FootnoteRef struct {
	Label string
	Index int
}

func (*Document) node()      {}
func (*Heading) node()       {}
func (*Paragraph) node()     {}
func (*ThematicBreak) node() {}
func (*BlockQuote) node()    {}
func (*Alert) node()         {}
func (*List) node()          {}
func (*ListItem) node()      {}
func (*CodeBlock) node()     {}
func (*HTMLBlock) node()     {}
func (*Table) node()         {}
func (*Text) node()          {}
func (*Code) node()          {}
func (*Emph) node()          {}
func (*Strong) node()        {}
func (*Strike) node()        {}
func (*Link) node()          {}
func (*Image) node()         {}
func (*RawHTML) node()       {}
func (*SoftBreak) node()     {}
func (*HardBreak) node()     {}
func (*FootnoteRef) node()   {}

func (*Heading) block()       {}
func (*Paragraph) block()     {}
func (*ThematicBreak) block() {}
func (*BlockQuote) block()    {}
func (*Alert) block()         {}
func (*List) block()          {}
func (*CodeBlock) block()     {}
func (*HTMLBlock) block()     {}
func (*Table) block()         {}

func (*Text) inline()        {}
func (*Code) inline()        {}
func (*Emph) inline()        {}
func (*Strong) inline()      {}
func (*Strike) inline()      {}
func (*Link) inline()        {}
func (*Image) inline()       {}
func (*RawHTML) inline()     {}
func (*SoftBreak) inline()   {}
func (*HardBreak) inline()   {}
func (*FootnoteRef) inline() {}
