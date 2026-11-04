// Package placement holds the reading of a manuscript that both typesetters
// share for page breaks (RFP §2, "Page breaks and placement"): which blocks
// are captions, what the directives ask for, and which numbers must not be
// broken. The PDF places with it; the .docx turns it into paragraph
// properties for Word.
package placement

import (
	"regexp"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// Item is a block with what the directives before it asked for and whether
// it is kept with what follows.
type Item struct {
	Block       markdown.Block
	BreakBefore bool // a <!-- pagebreak --> came before it
	Keep        bool // kept with the start of the next block
	Caption     bool // a caption, or the note under one
}

// Keeper reports whether an item is kept with what follows: every heading,
// caption groups, and blocks a keep-with-next directive marked.
func (it Item) Keeper() bool {
	_, h := it.Block.(*markdown.Heading)
	return h || it.Keep
}

// Items reads a container's blocks: directives become flags on the block
// after them (they are not blocks themselves), and captions are paired with
// what they caption when that follows in the same container.
func Items(bs []markdown.Block) []Item {
	var out []Item
	brk, keep := false, false
	for _, b := range bs {
		if d, ok := b.(*markdown.Directive); ok {
			switch d.Kind {
			case markdown.DirectivePageBreak:
				brk = true
			case markdown.DirectiveKeepWithNext:
				keep = true
			}
			continue
		}
		out = append(out, Item{Block: b, BreakBefore: brk, Keep: keep})
		brk, keep = false, false
	}
	for i := range out {
		p, ok := out[i].Block.(*markdown.Paragraph)
		if !ok || !IsCaption(p) {
			continue
		}
		j := i + 1
		if j < len(out) && !out[j].BreakBefore && IsNote(out[j].Block) {
			j++
		}
		if j < len(out) && !out[j].BreakBefore && Captionable(out[j].Block) {
			for k := i; k < j; k++ {
				out[k].Caption, out[k].Keep = true, true
			}
		}
	}
	return out
}

// Groups splits items into runs placed together: a run of keepers and the
// block after it (its tail, nil when a break or the end comes first). Every
// other item is a run of its own with no tail.
type Group struct {
	Items []Item
	Tail  markdown.Block
}

// Runs groups a container's items.
func Runs(its []Item) []Group {
	var out []Group
	for i := 0; i < len(its); {
		if !its[i].Keeper() {
			out = append(out, Group{Items: its[i : i+1]})
			i++
			continue
		}
		j := i + 1
		for j < len(its) && its[j].Keeper() && !its[j].BreakBefore {
			j++
		}
		g := Group{Items: its[i:j]}
		if j < len(its) && !its[j].BreakBefore {
			g.Tail = its[j].Block
		}
		out = append(out, g)
		i = j
	}
	return out
}

// Kept reports whether a group is kept with its tail.
func (g Group) Kept() bool { return g.Items[0].Keeper() }

// Chapter reports whether a group holds a heading at or above level (1 or
// 2) that breaks the page; level 0 never does.
func (g Group) Chapter(level int) bool {
	if level == 0 {
		return false
	}
	for _, it := range g.Items {
		if h, ok := it.Block.(*markdown.Heading); ok && h.Level <= level {
			return true
		}
	}
	return false
}

var captionRE = regexp.MustCompile(`^\s*(?:(?:表|図)\s*[0-9０-９]+(?:[-‐－.．][0-9０-９]+)*\s*[：:．.]|(?i:table|figure|fig\.)\s*[0-9]+(?:[-.][0-9]+)*\s*[:.])`)

// IsCaption: a paragraph made only of bold text (an italic note may follow
// on its own line), or one starting "表N：", "図N：", "Table N:" or
// "Figure N:".
func IsCaption(p *markdown.Paragraph) bool {
	if captionRE.MatchString(markdown.PlainText(p.Children)) {
		return true
	}
	strong := false
	for _, n := range p.Children {
		switch v := n.(type) {
		case *markdown.Strong:
			if strong && strings.TrimSpace(markdown.PlainText(v.Children)) != "" {
				return false // a second bold run after other text is prose
			}
			strong = true
		case *markdown.Emph:
			if !strong {
				return false
			}
		case *markdown.SoftBreak, *markdown.HardBreak:
		case *markdown.Text:
			if strings.TrimSpace(v.Value) != "" {
				return false
			}
		default:
			return false
		}
	}
	return strong
}

// IsNote: a paragraph of only italics, the source or note under a caption.
func IsNote(b markdown.Block) bool {
	p, ok := b.(*markdown.Paragraph)
	if !ok {
		return false
	}
	em := false
	for _, n := range p.Children {
		switch v := n.(type) {
		case *markdown.Emph:
			em = true
		case *markdown.SoftBreak, *markdown.HardBreak:
		case *markdown.Text:
			if strings.TrimSpace(v.Value) != "" {
				return false
			}
		default:
			return false
		}
	}
	return em
}

// Captionable: what a caption can be paired with.
func Captionable(b markdown.Block) bool {
	switch v := b.(type) {
	case *markdown.Table, *markdown.CodeBlock:
		return true
	case *markdown.Paragraph:
		return markdown.OnlyImages(v.Children) != nil
	}
	return false
}
