package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	md "github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

type fakeAssets struct{ mermaidErr error }

func (fakeAssets) Image(dest string, line int) (Picture, error) {
	if dest == "missing.png" {
		return Picture{}, &Error{Code: "file_not_found", Message: "missing.png does not exist"}
	}
	pix := bytes.Repeat([]byte{40, 90, 200, 255}, 400*300)
	return Picture{Pix: pix, PW: 400, PH: 300, DispW: 300}, nil
}

func (a fakeAssets) Mermaid(src string, line int) (Picture, error) {
	if a.mermaidErr != nil {
		return Picture{}, a.mermaidErr
	}
	pix := bytes.Repeat([]byte{255, 255, 255, 255}, 200*100)
	return Picture{Pix: pix, PW: 200, PH: 100, DispW: 200}, nil
}

func opts() Options {
	return Options{Theme: StandardTheme(), Assets: fakeAssets{}, MaxPages: 500,
		Chart: chart.Limits{MaxPoints: 10000, MaxSeries: 8}, MaxChartPoints: 50000}
}

func txt(s string) []md.Inline { return []md.Inline{&md.Text{Value: s}} }
func mdPara(s string) *md.Paragraph { return &md.Paragraph{Children: txt(s), Line: 1} }

var longJa = strings.Repeat("調査の結果、社内から社外への通信のうち一部が既知の不審な宛先に向かっていたことが分かった。", 5)

// sample is a document using every block kind.
func sample(n int) *md.Document {
	yes, no := true, false
	d := &md.Document{Meta: map[string]string{"title": "調査報告書", "subtitle": "通信の分析", "author": "調査班", "date": "2026-11-03"}}
	for i := 0; i < n; i++ {
		d.Children = append(d.Children,
			&md.Heading{Level: 1, Children: txt(fmt.Sprintf("%d. 概要", i+1)), Line: 1},
			&md.Paragraph{Line: 2, Children: []md.Inline{
				&md.Text{Value: longJa + " 詳しくは "},
				&md.Link{Dest: "https://example.com/report", Children: txt("報告書")},
				&md.Text{Value: " を参照。"}, &md.Strong{Children: txt("重要")}, &md.Emph{Children: txt("強調")},
				&md.Code{Value: "rm -rf"}, &md.Strike{Children: txt("取り消し")},
				&md.FootnoteRef{Label: "1", Index: 1},
			}},
			&md.Heading{Level: 2, Children: txt("観測した事象"), Line: 3},
			&md.List{Ordered: true, Start: 1, Tight: true, Line: 4, Items: []*md.ListItem{
				{Children: []md.Block{mdPara("最初の項目")}},
				{Children: []md.Block{mdPara(longJa), &md.List{Tight: true, Items: []*md.ListItem{
					{Task: &yes, Children: []md.Block{mdPara("済み")}},
					{Task: &no, Children: []md.Block{mdPara("未了")}},
				}}}},
			}},
			&md.BlockQuote{Line: 5, Children: []md.Block{mdPara("引用された文。" + longJa)}},
			&md.Alert{Kind: md.AlertWarning, Line: 6, Children: []md.Block{mdPara("注意すべき点。")}},
			&md.CodeBlock{Fenced: true, Info: "sh", Text: "grep -r token . | wc -l\n" + strings.Repeat("x", 200) + "\n", Line: 7},
			&md.Table{Line: 8, Align: []md.Align{md.AlignLeft, md.AlignRight, md.AlignCenter},
				Header: [][]md.Inline{txt("宛先"), txt("件数"), txt("割合")},
				Rows: [][][]md.Inline{
					{txt("社内"), txt("1,204"), txt("62%")},
					{txt("社外の非常に長い名前の宛先で折り返しが必要になる"), txt("731"), txt("38%")},
				}},
			&md.CodeBlock{Fenced: true, Info: "chart", Line: 9, Text: `{"type":"bar","title":"宛先別","x":{"categories":["社内","社外"]},"series":[{"values":[1204,731]}]}`},
			&md.CodeBlock{Fenced: true, Info: "mermaid", Line: 10, Text: "flowchart LR\n  A --> B\n"},
			&md.Paragraph{Line: 11, Children: []md.Inline{&md.Image{Dest: "chart.png", Alt: txt("図 1 宛先の内訳")}}},
			&md.ThematicBreak{Line: 12},
		)
	}
	d.Footnotes = []*md.Footnote{{Label: "1", Index: 1, Children: []md.Block{mdPara("出典: 試験用の架空の観測値。")}}}
	return d
}

func layout(t *testing.T, d *md.Document, o Options) (*Result, *Doc) {
	t.Helper()
	doc, err := NewDoc(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(doc.Close)
	r, err := Layout(d, doc, o)
	if err != nil {
		t.Fatal(err)
	}
	return r, doc
}

// Every op stays inside the margins, and no two text lines overlap.
func checkPages(t *testing.T, r *Result, th Theme) {
	t.Helper()
	const eps = 0.01
	x0, y0, x1, y1 := th.MarginL, th.MarginT, th.PageW-th.MarginR, th.PageH-th.MarginB
	for pi, p := range r.Pages {
		var texts [][4]float64
		for _, op := range p.Ops {
			a, b, c, d := op.Bounds()
			if a < x0-eps || b < y0-eps || c > x1+eps || d > y1+eps {
				t.Errorf("page %d: %T at (%.1f,%.1f)-(%.1f,%.1f) outside the margins", pi+1, op, a, b, c, d)
			}
			if _, ok := op.(TextOp); ok {
				for _, o := range texts {
					if a < o[2]-eps && o[0] < c-eps && b < o[3]-eps && o[1] < d-eps {
						t.Errorf("page %d: text lines overlap: %v and %v", pi+1, [4]float64{a, b, c, d}, o)
					}
				}
				texts = append(texts, [4]float64{a, b, c, d})
			}
		}
	}
}

func TestLayoutKeepsEverythingInsideAndApart(t *testing.T) {
	r, _ := layout(t, sample(6), opts())
	if len(r.Pages) < 3 {
		t.Fatalf("only %d pages", len(r.Pages))
	}
	checkPages(t, r, StandardTheme())
}

func TestRenderedPDFHoldsTheText(t *testing.T) {
	d := sample(2)
	r, doc := layout(t, d, opts())
	pdf, err := Render(r, doc, StandardTheme(), Meta{Title: "調査報告書", Author: "調査班", Classification: "社外秘"})
	if err != nil {
		t.Fatal(err)
	}
	text, pages, err := ExtractText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if pages != len(r.Pages) {
		t.Errorf("PDFKit sees %d pages, layout made %d", pages, len(r.Pages))
	}
	got := strings.Join(strings.Fields(text), "")
	for _, want := range []string{"調査報告書", "通信の分析", longJa, "報告書を参照。", "重要", "強調", "rm-rf", "取り消し",
		"最初の項目", "済み", "未了", "引用された文。", "Warning", "注意すべき点。", "grep -r token", strings.Repeat("x", 200),
		"宛先", "1,204", "社外の非常に長い名前の宛先で折り返しが必要になる", "図1宛先の内訳", "出典:試験用の架空の観測値。",
		"社外秘", fmt.Sprintf("1/%d", len(r.Pages))} {
		if !strings.Contains(got, strings.Join(strings.Fields(want), "")) {
			t.Errorf("PDF text lacks %q", want)
		}
	}
	if out := os.Getenv("REPORT_STUDIO_SAMPLE_PDF"); out != "" {
		if err := os.WriteFile(out, pdf, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(pdf, []byte("example.com/report")) {
		t.Error("link annotation missing")
	}
}

func TestTableHeaderRepeatsOnEveryPage(t *testing.T) {
	tb := &md.Table{Line: 1, Align: []md.Align{md.AlignNone, md.AlignNone},
		Header: [][]md.Inline{txt("列見出しA"), txt("列見出しB")}}
	for i := 0; i < 200; i++ {
		tb.Rows = append(tb.Rows, [][]md.Inline{txt(fmt.Sprint("行", i)), txt(longJa[:60])})
	}
	r, _ := layout(t, &md.Document{Children: []md.Block{tb}}, opts())
	if len(r.Pages) < 3 {
		t.Fatalf("only %d pages", len(r.Pages))
	}
	for pi, p := range r.Pages {
		found := false
		for _, op := range p.Ops {
			if to, ok := op.(TextOp); ok && to.Spans[0].Text == "列見出しA" {
				found = true
			}
		}
		if !found {
			t.Errorf("page %d has no header row", pi+1)
		}
	}
	checkPages(t, r, StandardTheme())
}

// A cell taller than a page continues on the next.
func TestTallRowContinues(t *testing.T) {
	tb := &md.Table{Line: 1, Align: []md.Align{md.AlignNone}, Header: [][]md.Inline{txt("見出し")},
		Rows: [][][]md.Inline{{txt(strings.Repeat(longJa, 40))}}}
	r, _ := layout(t, &md.Document{Children: []md.Block{tb}}, opts())
	if len(r.Pages) < 2 {
		t.Fatalf("%d pages", len(r.Pages))
	}
	checkPages(t, r, StandardTheme())
}

func TestWideTableShrinksThenWraps(t *testing.T) {
	var header []([]md.Inline)
	var row [][]md.Inline
	var align []md.Align
	for c := 0; c < 30; c++ {
		header = append(header, txt(fmt.Sprint("見出し", c)))
		row = append(row, txt("値ABCDEFG"))
		align = append(align, md.AlignNone)
	}
	tb := &md.Table{Line: 1, Align: align, Header: header, Rows: [][][]md.Inline{row}}
	r, _ := layout(t, &md.Document{Children: []md.Block{tb}}, opts())
	checkPages(t, r, StandardTheme())
	found := false
	for _, w := range r.Warnings {
		found = found || w.Code == "table_wrapped"
	}
	if !found {
		t.Error("no table_wrapped warning")
	}
}

func TestLinksOnlyForAllowedSchemes(t *testing.T) {
	d := &md.Document{Children: []md.Block{&md.Paragraph{Line: 4, Children: []md.Inline{
		&md.Link{Dest: "javascript:alert(1)", Children: txt("bad")},
		&md.Link{Dest: "file:///etc/passwd", Children: txt("file")},
		&md.Link{Dest: "mailto:a@example.com", Children: txt("mail")},
	}}}}
	r, _ := layout(t, d, opts())
	var urls []string
	for _, op := range r.Pages[0].Ops {
		if l, ok := op.(LinkOp); ok {
			urls = append(urls, l.URL)
		}
	}
	if len(urls) != 1 || urls[0] != "mailto:a@example.com" {
		t.Errorf("links = %v", urls)
	}
	n := 0
	for _, w := range r.Warnings {
		if w.Code == "link_scheme" && w.Line == 4 {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d link_scheme warnings with line 4, want 2: %+v", n, r.Warnings)
	}
}

func TestStops(t *testing.T) {
	doc, _ := NewDoc(nil)
	defer doc.Close()
	cases := map[string]struct {
		d    *md.Document
		o    Options
		code string
		line int
	}{
		"chart": {&md.Document{Children: []md.Block{&md.CodeBlock{Info: "chart", Line: 10, Text: "{\n\"type\": \"bar\",,\n}"}}}, opts(), "chart_invalid", 12},
		"image": {&md.Document{Children: []md.Block{&md.Paragraph{Line: 3, Children: []md.Inline{&md.Image{Dest: "missing.png"}}}}}, opts(), "file_not_found", 3},
		"pages": {sample(20), func() Options { o := opts(); o.MaxPages = 3; return o }(), "limit_exceeded", 0},
		"points": {&md.Document{Children: []md.Block{&md.CodeBlock{Info: "chart", Line: 1,
			Text: `{"type":"line","x":{"categories":["a","b","c"]},"series":[{"values":[1,2,3]}]}`}}},
			func() Options { o := opts(); o.MaxChartPoints = 2; return o }(), "limit_exceeded", 1},
	}
	for name, c := range cases {
		_, err := Layout(c.d, doc, c.o)
		var e *Error
		if !errors.As(err, &e) || e.Code != c.code || (c.line > 0 && e.Line != c.line) {
			t.Errorf("%s: err = %#v, want code %s line %d", name, err, c.code, c.line)
		}
	}
}

func TestMermaidFallsBackToItsSource(t *testing.T) {
	o := opts()
	o.Assets = fakeAssets{mermaidErr: errors.New("unsupported diagram type")}
	d := &md.Document{Children: []md.Block{&md.CodeBlock{Info: "mermaid", Line: 7, Text: "journey\n  title x\n"}}}
	r, _ := layout(t, d, o)
	found := false
	for _, w := range r.Warnings {
		found = found || (w.Code == "mermaid_fallback" && w.Line == 7)
	}
	if !found {
		t.Errorf("warnings = %+v", r.Warnings)
	}
	var shown bool
	for _, op := range r.Pages[0].Ops {
		if to, ok := op.(TextOp); ok && strings.Contains(to.Spans[0].Text, "journey") {
			shown = true
		}
	}
	if !shown {
		t.Error("source not shown")
	}
}

func TestJapaneseSoftBreaksVanishOthersBecomeSpaces(t *testing.T) {
	rs := flatten(StandardTheme(), textStyle{font: "HiraginoSans-W3", bold: "HiraginoSans-W6", size: 10},
		[]md.Inline{&md.Text{Value: "日本語の"}, &md.SoftBreak{}, &md.Text{Value: "文章 and"}, &md.SoftBreak{}, &md.Text{Value: "English"}},
		func(string, string) {})
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.text)
	}
	if b.String() != "日本語の文章 and English" {
		t.Errorf("got %q", b.String())
	}
}
