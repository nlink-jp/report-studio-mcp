package pdf

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const (
	body = iota
	bold
	sup
	link
)

func testDoc(t *testing.T) *Doc {
	t.Helper()
	d, err := NewDoc([]Style{
		{Font: "HiraginoSans-W3", Size: 10.5},
		{Font: "HiraginoSans-W6", Size: 10.5},
		{Font: "HiraginoSans-W3", Size: 10.5, Superscript: true},
		{Font: "HiraginoSans-W3", Size: 10.5, B: 0.6, Underline: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

const kinsoku = "、。，．）」』】〕〉》！？ー・"

var para = strings.Repeat("本書は、組版の試験文書です（括弧と句読点「、」「。」を含みます）。英数字 ABC 123 と😀も混ぜます。", 6)

func TestTypesetBreaksWithinWidthAndKeepsEveryCharacter(t *testing.T) {
	d := testDoc(t)
	spans := []Span{{"見出し語：", bold}, {para, body}, {"1", sup}}
	lines := d.Typeset(spans, 200)
	if len(lines) < 5 {
		t.Fatalf("only %d lines", len(lines))
	}
	var got strings.Builder
	for i, l := range lines {
		if l.Width > 200+0.01 {
			t.Errorf("line %d is %.2f wide", i, l.Width)
		}
		if l.Ascent <= 0 || l.Descent <= 0 {
			t.Errorf("line %d metrics %+v", i, l)
		}
		first := []rune(l.Spans[0].Text)[0]
		if i > 0 && strings.ContainsRune(kinsoku, first) {
			t.Errorf("line %d starts with %q", i, first)
		}
		for _, s := range l.Spans {
			got.WriteString(s.Text)
		}
	}
	if want := "見出し語：" + para + "1"; got.String() != want {
		t.Errorf("text changed by breaking:\n got %q\nwant %q", got.String(), want)
	}
	// Styles survive the split.
	last := lines[len(lines)-1].Spans
	if last[len(last)-1].Style != sup || lines[0].Spans[0].Style != bold {
		t.Errorf("styles lost: first %+v last %+v", lines[0].Spans, last)
	}
}

func TestTypesetBreaksAnUnbreakableToken(t *testing.T) {
	d := testDoc(t)
	tok := strings.Repeat("x", 300)
	lines := d.Typeset([]Span{{tok, body}}, 100)
	n := 0
	for _, l := range lines {
		if l.Width > 100.01 {
			t.Errorf("line %.2f wide", l.Width)
		}
		n += len(l.Spans[0].Text)
	}
	if n != 300 {
		t.Errorf("%d characters kept of 300", n)
	}
}

func TestMissingFontIsAnError(t *testing.T) {
	_, err := NewDoc([]Style{{Font: "HiraginoSans-W3", Size: 10}, {Font: "NoSuchFont-Regular", Size: 10}})
	if !errors.Is(err, ErrFontMissing) || !strings.Contains(err.Error(), "NoSuchFont-Regular") {
		t.Fatalf("err = %v", err)
	}
}

func TestMeasureAndOffset(t *testing.T) {
	d := testDoc(t)
	w := d.Measure("ABC", body)
	if w <= 0 || d.Measure("ABCABC", body) <= w {
		t.Fatalf("widths %v", w)
	}
	spans := []Span{{"前", body}, {"link", link}, {"後", body}}
	a, b := d.Offset(spans, 1), d.Offset(spans, 2)
	if !(a > 0 && b > a) || d.Offset(spans, 3) <= b {
		t.Errorf("offsets %v %v", a, b)
	}
}

func TestPDFRoundTrip(t *testing.T) {
	d := testDoc(t)
	w := NewPDF(d, 595.28, 841.89, "試験の題名", "作成者")
	w.BeginPage()
	y := 80.0
	for _, l := range d.Typeset([]Span{{para, body}}, 480) {
		y += l.Ascent
		w.DrawText(l.Spans, 56, y, false)
		y += l.Descent + l.Leading + 4
	}
	w.FillRect(56, 600, 40, 100, [3]float64{0.2, 0.4, 0.8}, 3, RoundTop)
	w.Link(56, 700, 100, 12, "https://example.com/a")
	w.EndPage()
	w.BeginPage()
	w.DrawText([]Span{{"2 ページ目", body}}, 56, 80, false)
	w.EndPage()
	pdf := w.Finish()

	text, pages, err := ExtractText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Errorf("pages = %d", pages)
	}
	strip := func(s string) string { return strings.Join(strings.Fields(s), "") }
	if !strings.Contains(strip(text), strip(para)) || !strings.Contains(strip(text), "2ページ目") {
		t.Errorf("text not recovered:\n%s", text)
	}
	if !bytes.Contains(pdf, []byte("/URI")) || !bytes.Contains(pdf, []byte("example.com/a")) {
		t.Error("link annotation missing")
	}
	if !bytes.Contains(pdf, []byte("+HiraginoSans-W3")) {
		t.Error("font not embedded as a subset")
	}
}

// The document information carries the title and author given, and nothing
// that names the machine or its user.
func TestPDFMetadataCarriesNothingElse(t *testing.T) {
	d := testDoc(t)
	w := NewPDF(d, 100, 100, "", "")
	w.BeginPage()
	w.EndPage()
	pdf := w.Finish()
	for _, leak := range []string{"/Users/", "/private/", "/var/folders"} {
		if bytes.Contains(pdf, []byte(leak)) {
			t.Errorf("PDF contains %q", leak)
		}
	}
	if bytes.Contains(pdf, []byte("/Author")) || bytes.Contains(pdf, []byte("/Title")) {
		t.Error("empty title or author written")
	}
}

func TestBitmapDrawsWithTheSamePrimitives(t *testing.T) {
	d := testDoc(t)
	b := NewBitmap(d, 200, 100, 2) // 100 x 50 points at 2 px/pt
	b.FillRect(0, 0, 100, 50, [3]float64{1, 1, 1}, 0, RoundNone)
	b.FillRect(10, 10, 20, 20, [3]float64{1, 0, 0}, 0, RoundNone)
	pix := b.Pixels()
	at := func(x, y int) [4]byte { i := (y*200 + x) * 4; return [4]byte{pix[i], pix[i+1], pix[i+2], pix[i+3]} }
	// Point (20, 20) is pixel (40, 40), top-down.
	if got := at(40, 40); got != [4]byte{255, 0, 0, 255} {
		t.Errorf("inside the red square: %v", got)
	}
	if got := at(150, 80); got != [4]byte{255, 255, 255, 255} {
		t.Errorf("outside it: %v", got)
	}
}

func TestAddStyleDedupesAndRefusesMissingFonts(t *testing.T) {
	d := testDoc(t)
	a, err := d.Add(Style{Font: "HiraginoSans-W3", Size: 9, Oblique: true})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := d.Add(Style{Font: "HiraginoSans-W3", Size: 9, Oblique: true})
	if a != b || a != 4 {
		t.Errorf("indices %d %d, want 4 4", a, b)
	}
	if _, err := d.Add(Style{Font: "NoSuchFont-Bold", Size: 9}); !errors.Is(err, ErrFontMissing) {
		t.Errorf("err = %v", err)
	}
	// The style registered after the refusal still gets the next index.
	if c, err := d.Add(Style{Font: "HiraginoSans-W6", Size: 9}); err != nil || c != 5 {
		t.Errorf("next index %d, %v", c, err)
	}
}

func TestLinesKnowTheirRuneRanges(t *testing.T) {
	d := testDoc(t)
	spans := []Span{{"😀" + para, body}}
	lines := d.Typeset(spans, 150)
	total := len([]rune("😀" + para))
	at := 0
	for i, l := range lines {
		if l.Start != at {
			t.Fatalf("line %d starts at %d, want %d", i, l.Start, at)
		}
		n := 0
		for _, s := range l.Spans {
			n += len([]rune(s.Text))
		}
		if l.End-l.Start != n {
			t.Fatalf("line %d range %d-%d holds %d runes", i, l.Start, l.End, n)
		}
		at = l.End
	}
	if at != total {
		t.Errorf("lines cover %d of %d runes", at, total)
	}
	one := lines[0].Spans
	if x := d.OffsetAt(one, 1); x <= 0 || x >= lines[0].Width {
		t.Errorf("offset of rune 1 = %v", x)
	}
}
