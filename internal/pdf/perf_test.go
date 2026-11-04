package pdf

import (
	"math"
	"strings"
	"testing"
	"time"

	md "github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// Breaking a paragraph into lines costs time linear in its length (the
// independent review measured 487 s for one 2 MiB paragraph).
func TestTypesetIsLinear(t *testing.T) {
	d := testDoc(t)
	measure := func(n int) time.Duration {
		s := strings.Repeat("abcd efgh ", n/10)
		start := time.Now()
		lines := d.Typeset([]Span{{Text: s, Style: body}}, 480)
		took := time.Since(start)
		t.Logf("%7d chars: %5d lines in %v", n, len(lines), took)
		return took
	}
	small, large := measure(20000), measure(160000)
	if large > small*20+200*time.Millisecond {
		t.Errorf("8x the text took %v against %v", large, small)
	}
}

// Setting a paragraph a window at a time breaks it exactly where setting it
// whole does — across mixed scripts, styles, emoji and hard breaks — and each
// line's width is the width it is drawn at (lines are drawn re-created from
// their spans; the whole paragraph's lines measured up to 14 pt wider).
func TestWindowedTypesetMatchesWhole(t *testing.T) {
	d := testDoc(t)
	var spans []Span
	for i := 0; i < 400; i++ {
		spans = append(spans, Span{Text: "日本語の文章と English words、句読点。😀👍🏽 ", Style: body},
			Span{Text: "太字の部分", Style: bold}, Span{Text: "\u2028", Style: body})
	}
	windowed := d.Typeset(spans, 300)
	old := typesetWindow
	typesetWindow = 1 << 30
	whole := d.Typeset(spans, 300)
	typesetWindow = old
	if len(windowed) != len(whole) {
		t.Fatalf("%d lines windowed, %d whole", len(windowed), len(whole))
	}
	for i := range whole {
		a, b := windowed[i], whole[i]
		if a.Start != b.Start || a.End != b.End {
			t.Fatalf("line %d: windowed %d-%d, whole %d-%d", i, a.Start, a.End, b.Start, b.End)
		}
		if drawn := d.OffsetAt(a.Spans, a.End-a.Start); math.Abs(a.Width-drawn) > 0.01 {
			t.Fatalf("line %d: width %.2f, drawn %.2f", i, a.Width, drawn)
		}
	}
}

// Decorations on a line are measured in one layout per line, not one per
// decoration (200,000 code spans took 80 s: independent re-review).
func TestDecoratedParagraphIsLinear(t *testing.T) {
	d, err := NewDoc(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	measure := func(n int) time.Duration {
		var kids []md.Inline
		for i := 0; i < n; i++ {
			kids = append(kids, &md.Code{Value: "a"}, &md.Text{Value: " "})
		}
		o := opts()
		start := time.Now()
		if _, err := Layout(&md.Document{Children: []md.Block{&md.Paragraph{Line: 1, Children: kids}}}, d, o); err != nil {
			t.Fatal(err)
		}
		took := time.Since(start)
		t.Logf("%6d code spans: %v", n, took)
		return took
	}
	small, large := measure(2000), measure(16000)
	if large > small*20+300*time.Millisecond {
		t.Errorf("8x the spans took %v against %v", large, small)
	}
}

// Keeping blocks together measures each block once and shares the result
// with its placement: runs of headings and keep-with-next directives, and the
// tables they precede, cost time linear in their number (RFP Phase 4).
func TestKeepingIsLinear(t *testing.T) {
	table := func(rows int) string {
		var b strings.Builder
		b.WriteString("| 区分 | 件数 | 金額 | 備考 |\n|---|---:|---:|---|\n")
		for i := 0; i < rows; i++ {
			b.WriteString("| 種別 | 1,234 | 9,876,543 | 説明の文章 |\n")
		}
		return b.String()
	}
	measure := func(n int) time.Duration {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString("## 見出し\n\n### 小見出し\n\n<!-- keep-with-next -->\n\n**表1：数値**\n\n")
			b.WriteString(table(20))
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat("## 見出し\n\n<!-- keep-with-next -->\n\n", n*5))
		b.WriteString(table(2000))
		doc := md.Parse([]byte(b.String()))
		start := time.Now()
		layout(t, doc, opts())
		took := time.Since(start)
		t.Logf("%4d groups: %v", n, took)
		return took
	}
	small, large := measure(10), measure(80)
	if large > small*20+500*time.Millisecond {
		t.Errorf("8x the groups took %v against %v", large, small)
	}
}
