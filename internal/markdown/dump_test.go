package markdown

import (
	"fmt"
	"strings"
)

// dump writes a tree compactly so tests can state its whole shape:
// blocks as kind@line(children), inlines as kind(children), text quoted.
func dump(d *Document) string {
	var b strings.Builder
	dumpBlocks(&b, d.Children)
	for _, f := range d.Footnotes {
		fmt.Fprintf(&b, " [fn%d %s](", f.Index, f.Label)
		dumpBlocks(&b, f.Children)
		b.WriteString(")")
	}
	return b.String()
}

func dumpBlocks(b *strings.Builder, bs []Block) {
	for i, bl := range bs {
		if i > 0 {
			b.WriteByte(' ')
		}
		dumpBlock(b, bl)
	}
}

func dumpBlock(b *strings.Builder, bl Block) {
	switch n := bl.(type) {
	case *Paragraph:
		fmt.Fprintf(b, "p@%d(", n.Line)
		dumpInlines(b, n.Children)
	case *Heading:
		fmt.Fprintf(b, "h%d@%d(", n.Level, n.Line)
		dumpInlines(b, n.Children)
	case *ThematicBreak:
		fmt.Fprintf(b, "hr@%d(", n.Line)
	case *BlockQuote:
		fmt.Fprintf(b, "quote@%d(", n.Line)
		dumpBlocks(b, n.Children)
	case *Alert:
		fmt.Fprintf(b, "alert:%s@%d(", n.Kind, n.Line)
		dumpBlocks(b, n.Children)
	case *List:
		kind := "ul"
		if n.Ordered {
			kind = fmt.Sprintf("ol%d", n.Start)
		}
		if !n.Tight {
			kind += ":loose"
		}
		fmt.Fprintf(b, "%s@%d(", kind, n.Line)
		for i, it := range n.Items {
			if i > 0 {
				b.WriteByte(' ')
			}
			task := ""
			if it.Task != nil {
				task = "[ ]"
				if *it.Task {
					task = "[x]"
				}
			}
			fmt.Fprintf(b, "li%s@%d(", task, it.Line)
			dumpBlocks(b, it.Children)
			b.WriteByte(')')
		}
	case *CodeBlock:
		fmt.Fprintf(b, "code@%d(%q %q", n.Line, n.Info, n.Text)
	case *HTMLBlock:
		fmt.Fprintf(b, "html@%d(%q", n.Line, n.Raw)
	case *Table:
		fmt.Fprintf(b, "table@%d(%v", n.Line, n.Align)
		dumpRow(b, n.Header)
		for _, r := range n.Rows {
			dumpRow(b, r)
		}
	default:
		fmt.Fprintf(b, "?%T(", bl)
	}
	b.WriteByte(')')
}

func dumpRow(b *strings.Builder, cells [][]Inline) {
	b.WriteString(" [")
	for i, c := range cells {
		if i > 0 {
			b.WriteString(" | ")
		}
		dumpInlines(b, c)
	}
	b.WriteString("]")
}

func dumpInlines(b *strings.Builder, ns []Inline) {
	for i, n := range ns {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch n := n.(type) {
		case *Text:
			fmt.Fprintf(b, "%q", n.Value)
		case *Code:
			fmt.Fprintf(b, "code(%q)", n.Value)
		case *SoftBreak:
			b.WriteString("sb")
		case *HardBreak:
			b.WriteString("br")
		case *Emph:
			b.WriteString("em(")
			dumpInlines(b, n.Children)
			b.WriteByte(')')
		case *Strong:
			b.WriteString("strong(")
			dumpInlines(b, n.Children)
			b.WriteByte(')')
		case *Strike:
			b.WriteString("del(")
			dumpInlines(b, n.Children)
			b.WriteByte(')')
		case *Link:
			kind := "a"
			if n.Autolink {
				kind = "auto"
			}
			fmt.Fprintf(b, "%s[%s](", kind, n.Dest)
			dumpInlines(b, n.Children)
			b.WriteByte(')')
		case *Image:
			fmt.Fprintf(b, "img[%s](", n.Dest)
			dumpInlines(b, n.Alt)
			b.WriteByte(')')
		case *RawHTML:
			fmt.Fprintf(b, "rawhtml(%q)", n.Raw)
		case *FootnoteRef:
			fmt.Fprintf(b, "fnref%d(%s)", n.Index, n.Label)
		default:
			fmt.Fprintf(b, "?%T", n)
		}
	}
}
