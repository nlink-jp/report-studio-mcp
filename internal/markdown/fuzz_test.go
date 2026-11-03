package markdown

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"
)

// FuzzParse: no input may panic, and the tree keeps its promises whatever
// the input. Crashers found are kept in testdata/fuzz/FuzzParse and run with
// every `go test`.
//
//	go test -run=^$ -fuzz=FuzzParse -fuzztime=60s ./internal/markdown/
func FuzzParse(f *testing.F) {
	for _, path := range []string{"testdata/commonmark-0.31.2.json", "testdata/gfm-0.29-gfm-extensions.json"} {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		var ex []specExample
		if err := json.Unmarshal(data, &ex); err != nil {
			f.Fatal(err)
		}
		for _, e := range ex {
			f.Add([]byte(e.Markdown))
		}
	}
	for _, s := range []string{
		"---\ntitle: t\nx\n---\n# h\n",
		"x[^1]\n\n[^1]: a\n    > [!NOTE]\n    > b\n",
		"> [!WARNING]\n> - [ ] task\n> | a |\n> | - |\n> | b |\n",
		"www.example.com/(a)) a@b.cd xmpp:a@b.c/d ~~s~~",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		d := Parse(src)
		_ = renderHTML(d)
		lines := bytes.Count(src, []byte("\n")) + bytes.Count(src, []byte("\r")) + 1
		for _, w := range d.Warnings {
			if w.Line < 1 || w.Line > lines || w.Code == "" {
				t.Fatalf("bad warning %+v (input has %d lines)", w, lines)
			}
		}
		for i, fn := range d.Footnotes {
			if fn.Index != i+1 {
				t.Fatalf("footnote %d has index %d", i, fn.Index)
			}
		}
		if bd := blockDepth(d.Children); bd > maxBlockDepth+1 {
			t.Fatalf("block depth %d", bd)
		}
		if id := docInlineDepth(d); id > maxInlineDepth {
			t.Fatalf("inline depth %d", id)
		}
		checkTree(t, d, len(d.Footnotes))
	})
}

func checkTree(t *testing.T, d *Document, nfn int) {
	var blocks func([]Block)
	var inlines func([]Inline)
	inlines = func(ns []Inline) {
		for _, n := range ns {
			switch n := n.(type) {
			case *Text:
				if !utf8.ValidString(n.Value) {
					t.Fatalf("invalid UTF-8 in text %q", n.Value)
				}
			case *Emph:
				inlines(n.Children)
			case *Strong:
				inlines(n.Children)
			case *Strike:
				inlines(n.Children)
			case *Link:
				inlines(n.Children)
			case *Image:
				inlines(n.Alt)
			case *FootnoteRef:
				if n.Index < 1 || n.Index > nfn {
					t.Fatalf("footnote reference %d of %d", n.Index, nfn)
				}
			}
		}
	}
	blocks = func(bs []Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *Paragraph:
				inlines(b.Children)
			case *Heading:
				if b.Level < 1 || b.Level > 6 {
					t.Fatalf("heading level %d", b.Level)
				}
				inlines(b.Children)
			case *BlockQuote:
				blocks(b.Children)
			case *Alert:
				if len(b.Children) == 0 {
					t.Fatal("empty alert")
				}
				blocks(b.Children)
			case *List:
				for _, it := range b.Items {
					blocks(it.Children)
				}
			case *Table:
				if len(b.Header) != len(b.Align) {
					t.Fatalf("header has %d cells, %d columns", len(b.Header), len(b.Align))
				}
				for _, c := range b.Header {
					inlines(c)
				}
				for _, r := range b.Rows {
					if len(r) != len(b.Align) {
						t.Fatalf("row has %d cells, %d columns", len(r), len(b.Align))
					}
					for _, c := range r {
						inlines(c)
					}
				}
			}
		}
	}
	blocks(d.Children)
	for _, f := range d.Footnotes {
		blocks(f.Children)
	}
}
