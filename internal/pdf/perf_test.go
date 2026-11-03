package pdf

import (
	"math"
	"strings"
	"testing"
	"time"
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
