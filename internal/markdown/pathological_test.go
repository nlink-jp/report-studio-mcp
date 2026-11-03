package markdown

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// Manuscripts come from a language model and may be hostile. These inputs
// follow cmark's test/pathological_tests.py (and add the GFM, footnote and
// depth cases); each is about 1-2 MiB and must parse in far less than a
// second. The bound asserted is generous so the test is not flaky; the
// measured times are printed with -v.

const pathologicalBound = 3 * time.Second

func rep(s string, n int) string { return strings.Repeat(s, n) }

func pathologicalInputs() []struct{ name, in string } {
	var lists strings.Builder
	for i := 0; i < 1000; i++ {
		lists.WriteString(rep("  ", i) + "* a\n")
	}
	var refs strings.Builder
	for i := 0; i < 40000; i++ {
		refs.WriteString("[r" + itoa(i) + "]: /u" + itoa(i) + "\n")
	}
	for i := 0; i < 40000; i++ {
		refs.WriteString("[r" + itoa(i) + "][r" + itoa(i+1) + "]")
	}
	var ticks strings.Builder
	for i := 1; i < 1400; i++ {
		ticks.WriteString("e" + rep("`", i))
	}
	return []struct{ name, in string }{
		{"nested strong emph", rep("*a **a ", 65000) + "b" + rep(" a** a*", 65000)},
		{"many emph closers with no openers", rep("a_ ", 400000)},
		{"many emph openers with no closers", rep("_a ", 400000)},
		{"many 3-emph openers with no closers", rep("a***", 300000)},
		{"mismatched openers and closers", rep("*a_ ", 300000)},
		{"openers and closers multiple of 3", "a**b" + rep("c* ", 400000)},
		{"link openers and emph closers", rep("[ a_", 300000)},
		{"many link closers with no openers", rep("a]", 600000)},
		{"many link openers with no closers", rep("[a", 600000)},
		{"pattern [ (]( repeated", rep("[ (](", 250000)},
		{"pattern ![[]() repeated", rep("![[]()", 200000)},
		{"nested brackets", rep("[", 500000) + "a" + rep("]", 500000)},
		{"nested images", rep("![", 300000) + "a" + rep("](u)", 300000)},
		{"link openers then many links", rep("[", 300000) + rep("[](u)", 200000)},
		{"image openers then many links", rep("![", 300000) + rep("[](u)", 200000)},
		{"unclosed links A", rep("[a](<b", 250000)},
		{"unclosed links B", rep("[a](b", 300000)},
		{"unclosed link titles", rep("[a](b \"", 200000)},
		{"deep parens in destinations", rep("[a]("+rep("(", 40), 30000)},
		{"backticks", ticks.String()},
		{"many backticks unmatched", rep("`a``", 400000)},
		{"unclosed <!--", "</" + rep("<!--", 400000)},
		{"unclosed <?", rep("<?", 800000)},
		{"unclosed <!X", rep("<!A ", 400000)},
		{"unclosed <![CDATA[", rep("<![CDATA[", 160000)},
		{"many <", rep("<", 1500000)},
		{"open tags without >", rep("<a href=\"x\" ", 120000)},
		{"many entities", rep("&amp;&#x1F600;&bogus;&", 80000)},
		{"nested block quotes", rep("> ", 500000) + "a"},
		{"nested block quotes without spaces", rep(">", 1000000) + "a\n" + rep(">", 500000) + "b\n"},
		{"many lines of deep quotes", rep(rep(">", 50000)+"a\n", 30)},
		{"deeply nested lists", lists.String()},
		{"nested list markers on one line", rep("- ", 500000) + "a"},
		{"nested ordered markers on one line", rep("1. ", 400000) + "a"},
		{"mixed containers on one line", rep("> - 1. ", 150000) + "a"},
		{"many list items", rep("- a\n", 300000)},
		{"many link references", refs.String()},
		{"long labels", rep("["+rep("a", 990)+"]", 1500)},
		{"huge table: many rows", "| a | b | c | d |\n|---|---|---|---|\n" + rep("| 1 | 2 | 3 | 4 |\n", 70000)},
		{"huge table: wide header, short rows", rep("|a", 30000) + "\n" + rep("|-", 30000) + "\n" + rep("|x\n", 300000)},
		{"huge table: many columns", rep("|a", 200000) + "\n" + rep("|-", 200000) + "\n" + rep("|b", 200000) + "\n"},
		{"many pipes in a paragraph", rep("a | b\n", 200000)},
		{"near-miss delimiter rows", "a\n" + rep("-|-\n-|-|-\n", 100000)},
		{"many tildes", rep("~a ~~b ", 200000)},
		{"many tilde closers", rep("a~ ", 400000)},
		{"many www", rep("(www.", 300000)},
		{"autolink with many closing parens", "www.example.com/" + rep(")", 1500000)},
		{"autolink with trailing entities", "www.example.com/" + rep("&a;", 400000)},
		{"many emails", rep("a@b.cd ", 200000)},
		{"many at signs", rep("@", 1500000)},
		{"long local part then many at", rep("a", 750000) + rep("@", 750000)},
		{"many failing emails", rep("a.b@c-", 250000)},
		{"many xmpp", rep("xmpp:a@b.c/", 120000)},
		{"many footnote references", rep("x[^1]", 300000) + "\n\n[^1]: n\n"},
		{"many footnote definitions", rep("[^a]: b\n", 200000)},
		{"undefined footnote references", rep("[^nope]", 200000)},
		{"nested footnote openers", rep(rep("[^", 1300)+rep("]", 1300)+" ", (2<<20)/3901)},
		{"long footnote labels", rep("[^"+rep("a", 3990)+"]", 500)},
		{"many setext lines", rep("a\n=\n", 400000)},
		{"many fences", rep("```\n", 400000)},
		{"many html blocks", rep("<div>\n\n", 250000)},
		{"many headings", rep("####### a\n###### b\n", 100000)},
		{"many hard breaks", rep("a  \n", 400000)},
		{"many tabs", rep("\t", 500000) + "a\n" + rep(">\t", 200000)},
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func TestPathological(t *testing.T) {
	for _, c := range pathologicalInputs() {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			d := Parse([]byte(c.in))
			elapsed := time.Since(start)
			t.Logf("%-40s %8.2f MiB  %v", c.name, float64(len(c.in))/(1<<20), elapsed.Round(time.Millisecond))
			if elapsed > pathologicalBound {
				t.Errorf("took %v (bound %v)", elapsed, pathologicalBound)
			}
			if bd := blockDepth(d.Children); bd > maxBlockDepth+1 {
				t.Errorf("block depth %d exceeds %d", bd, maxBlockDepth+1)
			}
			if id := docInlineDepth(d); id > maxInlineDepth {
				t.Errorf("inline depth %d exceeds %d", id, maxInlineDepth)
			}
			_ = renderHTML(d) // the renderer recurses; the bounds keep it shallow
		})
	}
}

// TestDeepNesting checks the depth bound itself: 100,000 nested block
// quotes and list levels parse, and the tree stops at the bound with the
// rest kept as text.
func TestDeepNesting(t *testing.T) {
	for _, c := range []struct{ name, in string }{
		{"block quotes", rep(">", 100000) + " a"},
		{"block quotes with spaces", rep("> ", 100000) + "a"},
		{"bullet lists", rep("- ", 100000) + "a"},
		{"ordered lists", rep("1. ", 100000) + "a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := Parse([]byte(c.in))
			bd := blockDepth(d.Children)
			if bd < maxBlockDepth-1 || bd > maxBlockDepth+1 {
				t.Errorf("block depth %d, want about %d", bd, maxBlockDepth)
			}
			if !strings.Contains(renderHTML(d), "a") {
				t.Error("content lost")
			}
		})
	}
	d := Parse([]byte(rep("*", 200000) + "a" + rep("*", 200000)))
	if id := docInlineDepth(d); id != maxInlineDepth {
		t.Errorf("inline depth %d, want %d", id, maxInlineDepth)
	}
	if got := plainText(d.Children[0].(*Paragraph).Children); got != "a" {
		t.Errorf("text past the depth bound: got %d bytes, want %q", len(got), "a")
	}
}

// TestTableAutocompleteBound: padding short rows stops at cmark-gfm's bound;
// the rest of the lines become a paragraph.
func TestTableAutocompleteBound(t *testing.T) {
	in := rep("|a", 1000) + "\n" + rep("|-", 1000) + "\n" + rep("|x\n", 2000)
	d := Parse([]byte(in))
	tb, ok := d.Children[0].(*Table)
	if !ok {
		t.Fatalf("first block is %T", d.Children[0])
	}
	if n := len(tb.Rows); n < 500 || n > 530 {
		t.Errorf("rows %d, want about %d", n, maxAutocompleted/999)
	}
	for _, r := range tb.Rows {
		if len(r) != 1000 {
			t.Fatalf("row has %d cells", len(r))
		}
	}
	if len(d.Children) != 2 {
		t.Errorf("want table and paragraph, got %d blocks", len(d.Children))
	}
}

// TestTableAutocompleteDocumentWide: the padding bound is shared by every
// table of the document, so many tables with a wide header and short rows
// cannot add up (410 tables of 627 columns once allocated about 5 GB). The
// tree still keeps its promise of len(Align) cells per row; rows that would
// need padding past the bound end their table.
func TestTableAutocompleteDocumentWide(t *testing.T) {
	in := rep(rep("|a", 627)+"\n"+rep("|-", 627)+"\n"+rep("|x\n", 836)+"\n", 410)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	d := Parse([]byte(in))
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	alloc := after.TotalAlloc - before.TotalAlloc
	t.Logf("%.2f MiB input: %v, %.1f MiB allocated", float64(len(in))/(1<<20), elapsed.Round(time.Millisecond), float64(alloc)/(1<<20))
	if alloc > 500<<20 {
		t.Errorf("allocated %d MiB (bound 500 MiB)", alloc>>20)
	}
	if elapsed > pathologicalBound {
		t.Errorf("took %v (bound %v)", elapsed, pathologicalBound)
	}
	tables, padded := 0, 0
	for _, b := range d.Children {
		tb, ok := b.(*Table)
		if !ok {
			continue
		}
		tables++
		if len(tb.Header) != len(tb.Align) {
			t.Fatalf("header has %d cells, %d columns", len(tb.Header), len(tb.Align))
		}
		for _, r := range tb.Rows {
			if len(r) != len(tb.Align) {
				t.Fatalf("row has %d cells, %d columns", len(r), len(tb.Align))
			}
			for _, c := range r {
				if c == nil {
					padded++
				}
			}
		}
	}
	if tables != 410 {
		t.Errorf("%d tables, want 410 (each header still makes one)", tables)
	}
	if padded > maxAutocompleted+627 {
		t.Errorf("%d padded cells, bound %d", padded, maxAutocompleted)
	}
}

func blockDepth(bs []Block) int {
	m := 0
	for _, b := range bs {
		d := 0
		switch b := b.(type) {
		case *BlockQuote:
			d = 1 + blockDepth(b.Children)
		case *Alert:
			d = 1 + blockDepth(b.Children)
		case *List:
			for _, it := range b.Items {
				if x := 2 + blockDepth(it.Children); x > d {
					d = x
				}
			}
		}
		if d > m {
			m = d
		}
	}
	return m
}

func docInlineDepth(d *Document) int {
	m := 0
	var walk func(bs []Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *Paragraph:
				m = max(m, inlineDepth(b.Children))
			case *Heading:
				m = max(m, inlineDepth(b.Children))
			case *BlockQuote:
				walk(b.Children)
			case *Alert:
				walk(b.Children)
			case *List:
				for _, it := range b.Items {
					walk(it.Children)
				}
			case *Table:
				for _, c := range b.Header {
					m = max(m, inlineDepth(c))
				}
				for _, r := range b.Rows {
					for _, c := range r {
						m = max(m, inlineDepth(c))
					}
				}
			}
		}
	}
	walk(d.Children)
	for _, f := range d.Footnotes {
		walk(f.Children)
	}
	return m
}

func inlineDepth(ns []Inline) int {
	m := 0
	for _, n := range ns {
		d := 0
		switch n := n.(type) {
		case *Emph:
			d = 1 + inlineDepth(n.Children)
		case *Strong:
			d = 1 + inlineDepth(n.Children)
		case *Strike:
			d = 1 + inlineDepth(n.Children)
		case *Link:
			d = 1 + inlineDepth(n.Children)
		case *Image:
			d = 1 + inlineDepth(n.Alt)
		}
		m = max(m, d)
	}
	return m
}

func plainText(ns []Inline) string {
	var b strings.Builder
	var walk func([]Inline)
	walk = func(ns []Inline) {
		for _, n := range ns {
			switch n := n.(type) {
			case *Text:
				b.WriteString(n.Value)
			case *Emph:
				walk(n.Children)
			case *Strong:
				walk(n.Children)
			}
		}
	}
	walk(ns)
	return b.String()
}
