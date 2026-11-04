package pdf

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	md "github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// Placement rules of "Page breaks and placement" (RFP §2, v0.2.0). Each test
// sweeps how much text comes first, so the block under test meets the foot of
// the page at every position.

// opText is the text one line op draws.
func opText(op TextOp) string {
	var b strings.Builder
	for _, s := range op.Spans {
		b.WriteString(s.Text)
	}
	return strings.ReplaceAll(b.String(), "⁠", "")
}

// linePages lists every text line in order with the page it is on.
type placedLine struct {
	page int
	text string
	y    float64
}

func lines(r *Result) []placedLine {
	var out []placedLine
	for pi, p := range r.Pages {
		for _, op := range p.Ops {
			if t, ok := op.(TextOp); ok {
				out = append(out, placedLine{pi, opText(t), t.Y})
			}
		}
	}
	return out
}

// pageOf is the page of the first line containing s, or -1.
func pageOf(r *Result, s string) int {
	for _, l := range lines(r) {
		if strings.Contains(l.text, s) {
			return l.page
		}
	}
	return -1
}

func parse(_ *testing.T, src string) *md.Document {
	return md.Parse([]byte(src))
}

// filler is n one-line paragraphs joined by hard breaks into one paragraph,
// so each n moves what follows down by one line.
func filler(n int) string {
	if n == 0 {
		return ""
	}
	ls := make([]string, n)
	for i := range ls {
		ls[i] = fmt.Sprintf("ダミーの本文の行 %02d。", i)
	}
	return strings.Join(ls, "  \n") + "\n\n"
}

const wideTable = `| 区分 | 状態 | 組み合わせ | 件数A (件) | 回数 (回) | 金額A (円) | 件数B (件) | 金額B (円) | 件数C (件) | 金額C (円) | 比率 (%) | 備考 |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 種別あ (説明文あり) | 状態1: 説明あり | 項目X+項目Y+項目Z (9,999円) | 1,234 | 12,345 | 1,234,567 | 2,345 | 9,876,543 | 3,456 | 7,654,321 | 12.34% | テスト |
| 種別い (説明文あり) | 状態2: 説明あり | 項目X+項目Y (5,555円) | 2,222 | 22,222 | 2,222,222 | 3,333 | 3,333,333 | 4,444 | 4,444,444 | 56.78% | テスト |
`

const stackedChart = "```chart\n" + `{"type":"bar","stacked":true,
 "x":{"label":"日","categories":["D01","D02","D03","D04","D05","D06","D07","D08","D09","D10","D11","D12","D13","D14","D15","D16","D17","D18","D19","D20","D21"]},
 "y":{"label":"回数","min":0},
 "series":[{"name":"系列A","values":[100,120,90,150,160,210,180,250,520,310,270,140,120,50,260,180,400,720,640,400,320]},
           {"name":"系列B","values":[0,0,0,0,0,0,0,0,0,20,40,50,30,60,140,170,540,760,770,440,330]}]}` + "\n```\n"

func sweep(t *testing.T, max int, f func(n int)) {
	for n := 0; n <= max; n++ {
		f(n)
	}
}

func TestCaptionStaysWithItsTable(t *testing.T) {
	sweep(t, 60, func(n int) {
		src := filler(n) + "**表1：12 列の数値表**\n*注記: 値はすべて乱数*\n\n" + wideTable
		r, _ := layout(t, parse(t, src), opts())
		if c, b := pageOf(r, "表1："), pageOf(r, "種別あ"); c != b {
			t.Errorf("filler %d: caption on page %d, first row on page %d", n, c+1, b+1)
		}
	})
}

func TestCaptionStaysWithItsChart(t *testing.T) {
	sweep(t, 60, func(n int) {
		src := filler(n) + "**図1：積み上げ棒**\n\n" + stackedChart
		r, _ := layout(t, parse(t, src), opts())
		c := pageOf(r, "図1：")
		fig := -1
		for pi, p := range r.Pages {
			for _, op := range p.Ops {
				if _, ok := op.(FigOp); ok && fig < 0 {
					fig = pi
				}
			}
		}
		if c != fig {
			t.Errorf("filler %d: caption on page %d, chart on page %d", n, c+1, fig+1)
		}
	})
}

func TestHeadingStaysWithATableStart(t *testing.T) {
	sweep(t, 60, func(n int) {
		src := filler(n) + "## 集計\n\n" + wideTable
		r, _ := layout(t, parse(t, src), opts())
		if h, b := pageOf(r, "集計"), pageOf(r, "種別あ"); h != b {
			t.Errorf("filler %d: heading on page %d, first row on page %d", n, h+1, b+1)
		}
	})
}

func TestHeadingIsNotSplit(t *testing.T) {
	head := strings.Repeat("とても長い見出しが三行以上に折り返す場合の扱い", 5)
	sweep(t, 60, func(n int) {
		src := filler(n) + "## " + head + "\n\n本文。\n"
		r, _ := layout(t, parse(t, src), opts())
		pages := map[int]bool{}
		for _, l := range lines(r) {
			if strings.Contains(head, l.text) && l.text != "" && !strings.HasPrefix(l.text, "ダミー") && l.text != "本文。" {
				pages[l.page] = true
			}
		}
		if len(pages) != 1 {
			t.Errorf("filler %d: the heading's lines are on pages %v", n, pages)
		}
	})
}

func TestHeadingIsNotLeftBeforeAThreeLineParagraph(t *testing.T) {
	three := strings.Repeat("三行の段落。", 60) // three lines at the standard width
	sweep(t, 60, func(n int) {
		src := filler(n) + "## 見出し\n\n" + three + "\n"
		r, _ := layout(t, parse(t, src), opts())
		if h, p := pageOf(r, "見出し"), pageOf(r, "三行の段落"); h != p {
			t.Errorf("filler %d: heading on page %d, paragraph starts on page %d", n, h+1, p+1)
		}
	})
}

func TestNoWidowsOrOrphans(t *testing.T) {
	para := strings.Repeat("孤立行の試験に使う段落の文。", 40)
	sweep(t, 60, func(n int) {
		src := filler(n) + para + "\n"
		r, _ := layout(t, parse(t, src), opts())
		perPage := map[int]int{}
		total := 0
		for _, l := range lines(r) {
			if strings.Contains(l.text, "孤立行") || strings.Contains(l.text, "段落の文") || strings.Contains(l.text, "試験に使う") {
				perPage[l.page]++
				total++
			}
		}
		if total < 3 {
			t.Fatalf("the paragraph has only %d lines", total)
		}
		for p, c := range perPage {
			if c < 2 {
				t.Errorf("filler %d: page %d holds %d line(s) of the paragraph (%v)", n, p+1, c, perPage)
			}
		}
	})
}

var sourceNumberRE = regexp.MustCompile(`[0-9][0-9,.]*%?`)

func TestNumbersInCellsAreNotBroken(t *testing.T) {
	r, _ := layout(t, parse(t, wideTable), opts())
	all := strings.Join(func() []string {
		var s []string
		for _, l := range lines(r) {
			s = append(s, l.text)
		}
		return s
	}(), "\n")
	for _, num := range sourceNumberRE.FindAllString(wideTable, -1) {
		if len(num) < 2 {
			continue
		}
		found := false
		for l := range strings.SplitSeq(all, "\n") {
			if strings.Contains(l, num) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is broken across lines", num)
		}
	}
	for _, w := range r.Warnings {
		if w.Code == "cell_token_split" {
			t.Errorf("unexpected %s: %s", w.Code, w.Message)
		}
	}
}

func TestTwoLineParagraphIsNotSplit(t *testing.T) {
	two := strings.Repeat("太字で書いた長い見出しの代わり", 5) // two lines at the standard width
	sweep(t, 60, func(n int) {
		src := filler(n) + "**" + two + "**\n\n本文。\n"
		r, _ := layout(t, parse(t, src), opts())
		pages := map[int]bool{}
		for _, l := range lines(r) {
			if strings.Contains(l.text, "見出しの代わり") || strings.Contains(l.text, "太字で書いた") {
				pages[l.page] = true
			}
		}
		if len(pages) != 1 {
			t.Errorf("filler %d: the paragraph's lines are on pages %v", n, pages)
		}
	})
}

func withBreak(level int) Options {
	o := opts()
	o.Theme.BreakBefore = level
	return o
}

func TestChapterBreaks(t *testing.T) {
	src := "---\ntitle: 報告書\n---\n\n# 題名の見出し\n\n## 1. 概要\n\n概要の本文。\n\n## 2. 事実\n\n### 2.1 節\n\n事実の本文。\n\n> ## 引用の中の見出し\n\n## 3. 解釈\n\n解釈の本文。\n"
	r, _ := layout(t, parse(t, src), withBreak(2))
	for _, c := range []struct {
		text string
		page int
	}{
		{"報告書", 0}, {"題名の見出し", 0}, {"1. 概要", 0}, // the title block and a run of headings: no blank page
		{"2. 事実", 1}, {"2.1 節", 1}, // a run breaks once, before its first
		{"引用の中の見出し", 1}, // only the body's own headings break
		{"3. 解釈", 2},
	} {
		if p := pageOf(r, c.text); p != c.page {
			t.Errorf("%q on page %d, want %d", c.text, p+1, c.page+1)
		}
	}
	if len(r.Pages) != 3 {
		t.Errorf("%d pages, want 3", len(r.Pages))
	}
	// h1 breaks only before level 1; none never.
	if r, _ := layout(t, parse(t, src), withBreak(1)); len(r.Pages) != 1 {
		t.Errorf("break_before h1: %d pages, want 1", len(r.Pages))
	}
	if r, _ := layout(t, parse(t, src), withBreak(0)); len(r.Pages) != 1 {
		t.Errorf("break_before none: %d pages, want 1", len(r.Pages))
	}
	// A chapter after a table that continued onto a new page still breaks:
	// the rows make that page's body.
	rows := "| a | b |\n|---|---|\n" + strings.Repeat("| 行 | 値 |\n", 80)
	r, _ = layout(t, parse(t, "## 1. 表\n\n"+rows+"\n## 2. 次の章\n\n本文。\n"), withBreak(2))
	if last, next := pageOf(r, "2. 次の章"), len(r.Pages)-1; last != next || pageOf(r, "1. 表") == last {
		t.Errorf("chapter after a long table on page %d of %d", last+1, len(r.Pages))
	}
}

func TestPageBreakDirective(t *testing.T) {
	r, _ := layout(t, parse(t, "本文。\n\n<!-- pagebreak -->\n\n次の本文。\n"), opts())
	if p := pageOf(r, "次の本文"); p != 1 {
		t.Errorf("after the directive: page %d, want 2", p+1)
	}
	for _, l := range lines(r) {
		if strings.Contains(l.text, "pagebreak") {
			t.Error("the directive is shown")
		}
	}
	// At the top of a page, and repeated: no blank pages.
	r, _ = layout(t, parse(t, "<!-- PageBreak -->\n\n<!--   pagebreak -->\n\n本文。\n\n<!-- pagebreak -->\n<!-- pagebreak -->\n"), opts())
	if len(r.Pages) != 1 {
		t.Errorf("%d pages, want 1", len(r.Pages))
	}
	for _, w := range r.Warnings {
		if w.Code == "raw_html" {
			t.Errorf("a directive warned raw_html at line %d", w.Line)
		}
	}
	// Inside a container, or with anything else, it is ordinary HTML.
	r, _ = layout(t, parse(t, "> <!-- pagebreak -->\n\n<!-- pagebreak --> x\n\n<!-- landscape -->\n"), opts())
	shown := 0
	for _, l := range lines(r) {
		if strings.Contains(l.text, "pagebreak") || strings.Contains(l.text, "landscape") {
			shown++
		}
	}
	if shown != 3 || len(r.Pages) != 1 {
		t.Errorf("ordinary comments: %d shown on %d pages", shown, len(r.Pages))
	}
}

func TestKeepWithNextDirective(t *testing.T) {
	sweep(t, 60, func(n int) {
		src := filler(n) + "<!-- keep-with-next -->\n\n次のコードを実行する。\n\n```sh\nline one\nline two\nline three\n```\n"
		r, _ := layout(t, parse(t, src), opts())
		if a, b := pageOf(r, "次のコードを"), pageOf(r, "line one"); a != b {
			t.Errorf("filler %d: the paragraph on page %d, the code on page %d", n, a+1, b+1)
		}
	})
}

func TestFiguresShrinkToFitOrMoveOn(t *testing.T) {
	fig := func(r *Result) (int, FigOp) {
		for pi, p := range r.Pages {
			for _, op := range p.Ops {
				if f, ok := op.(FigOp); ok {
					return pi, f
				}
			}
		}
		t.Fatal("no chart")
		return 0, FigOp{}
	}
	r0, _ := layout(t, parse(t, stackedChart), opts())
	_, orig := fig(r0)
	shrunk := 0
	sweep(t, 60, func(n int) {
		r, _ := layout(t, parse(t, filler(n)+stackedChart), opts())
		checkPages(t, r, StandardTheme())
		_, f := fig(r)
		if s := f.Scale / orig.Scale; s < minShrink-0.001 {
			t.Errorf("filler %d: drawn at %.0f%%", n, s*100)
		} else if s < 0.999 {
			shrunk++
		}
	})
	if shrunk == 0 {
		t.Error("no chart was shrunk to fit; the sweep does not reach the rule")
	}
}

type wideDiagram struct {
	fakeAssets
	natW float64
}

func (a wideDiagram) Mermaid(string, int) (Picture, error) {
	pix := make([]byte, 400*40*4)
	return Picture{Pix: pix, PW: 400, PH: 40, DispW: 480, NatW: a.natW}, nil
}

func TestMermaidScaledWarns(t *testing.T) {
	for _, c := range []struct {
		natW float64
		warn bool
	}{{1200, true}, {700, false}, {0, false}} {
		o := opts()
		o.Assets = wideDiagram{natW: c.natW}
		r, _ := layout(t, parse(t, "```mermaid\nflowchart LR\n  A --> B\n```\n"), o)
		got := false
		for _, w := range r.Warnings {
			got = got || w.Code == "mermaid_scaled"
		}
		if got != c.warn {
			t.Errorf("natural width %v: warned %v", c.natW, got)
		}
	}
}

func TestBrokenNumbersAreReportedOncePerTable(t *testing.T) {
	var head, rule, row strings.Builder
	for c := range 40 {
		fmt.Fprintf(&head, "| 列%d ", c)
		rule.WriteString("|---:")
		row.WriteString("| 1,234,567,890 ")
	}
	src := head.String() + "|\n" + rule.String() + "|\n" + row.String() + "|\n" + row.String() + "|\n"
	r, _ := layout(t, parse(t, src), opts())
	n := 0
	for _, w := range r.Warnings {
		if w.Code == "cell_token_split" {
			n++
			if !strings.Contains(w.Message, `"列0"`) || w.Line != 1 {
				t.Errorf("warning at line %d: %s", w.Line, w.Message)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d cell_token_split warnings, want 1", n)
	}
}

func TestFullWidthAndSignedNumbersStayWhole(t *testing.T) {
	src := "| 区分 | 金額 | 前年比 | 備考 |\n|---|---:|---:|---|\n" +
		"| （全角） | （１，２３４，５６７） | ▲１２．３４％ | 説明の文章がここに入る長めのセル |\n" +
		"| 符号 | −1,234,567 | -12.5% | x1,234 |\n"
	for _, w := range []float64{140, 200, 300} {
		o := opts()
		o.Theme.MarginR = o.Theme.PageW - o.Theme.MarginL - w // a narrow column
		r, _ := layout(t, parse(t, src), o)
		all := map[string]bool{}
		for _, l := range lines(r) {
			all[l.text] = true
		}
		warned := false
		for _, w := range r.Warnings {
			warned = warned || w.Code == "cell_token_split"
		}
		broken := 0
		for _, num := range []string{"１，２３４，５６７", "▲１２．３４％", "−1,234,567", "-12.5%"} {
			found := false
			for txt := range all {
				found = found || strings.Contains(txt, num)
			}
			if !found {
				broken++
			}
		}
		// Whole wherever the columns fit; where they cannot, reported.
		if broken > 0 != warned {
			t.Errorf("width %v: %d numbers broken, warned %v", w, broken, warned)
		}
		if w >= 200 && broken > 0 {
			t.Errorf("width %v: %d numbers broken in a column wide enough", w, broken)
		}
	}
}

func TestKeptCodeBlockMovesWhole(t *testing.T) {
	code := "```sh\n" + strings.Repeat("echo line\n", 6) + "```\n"
	sweep(t, 60, func(n int) {
		src := filler(n) + "<!-- keep-with-next -->\n\n" + code + "\n後に続く段落。\n"
		r, _ := layout(t, parse(t, src), opts())
		pages := map[int]bool{}
		for _, l := range lines(r) {
			if l.text == "echo line" {
				pages[l.page] = true
			}
		}
		if len(pages) != 1 || !pages[pageOf(r, "後に続く段落")] {
			t.Errorf("filler %d: code on pages %v, the next paragraph on page %d", n, pages, pageOf(r, "後に続く段落")+1)
		}
	})
}

// The reporter's synthetic sample (RFP Discussion Log 14), set in the formal
// profile with every amount of text before it: the captions stay with what
// they caption, the numbers stay whole, and a chart is shrunk to no less than
// 70% or moved on with its caption. The log records what each variant did,
// the measurement behind the initial values (Discussion Log 17).
func TestReporterSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/reporter-sample.md")
	if err != nil {
		t.Fatal(err)
	}
	o := opts()
	o.Theme = theme.Formal()
	r0, _ := layout(t, parse(t, strings.Replace(string(raw), "{{filler}}", "", 1)), o)
	var orig float64
	for _, p := range r0.Pages {
		for _, op := range p.Ops {
			if f, ok := op.(FigOp); ok {
				orig = f.Scale
			}
		}
	}
	shrunk, moved := 0, 0
	for n := 0; n <= 60; n++ {
		src := strings.Replace(string(raw), "{{filler}}", filler(n), 1)
		r, _ := layout(t, parse(t, src), o)
		checkPages(t, r, o.Theme)
		if c, b := pageOf(r, "表1："), pageOf(r, "種別あ"); c != b {
			t.Errorf("filler %d: table caption on page %d, table on page %d", n, c+1, b+1)
		}
		capPage, figPage, scale, figY := pageOf(r, "図1："), -1, 0.0, 0.0
		for pi, p := range r.Pages {
			for _, op := range p.Ops {
				if f, ok := op.(FigOp); ok {
					figPage, scale, figY = pi, f.Scale/orig, f.Y
				}
			}
		}
		if capPage != figPage {
			t.Errorf("filler %d: chart caption on page %d, chart on page %d", n, capPage+1, figPage+1)
		}
		if scale < minShrink-0.001 {
			t.Errorf("filler %d: chart at %.0f%%", n, scale*100)
		}
		if scale < 0.999 {
			shrunk++
			t.Logf("filler %2d: chart shrunk to %.0f%%", n, scale*100)
		}
		if figPage > pageOf(r, fmt.Sprintf("行 %02d", max(n-1, 0))) && n > 0 && figY < o.Theme.MarginT+60 {
			moved++
		}
		for _, num := range []string{"1,234,567", "9,876,543", "7,654,321", "2,222,222", "12.34%", "56.78%"} {
			if pageOf(r, num) < 0 {
				t.Errorf("filler %d: %s is broken", n, num)
			}
		}
	}
	t.Logf("61 variants: the chart was shrunk in %d and moved on in %d", shrunk, moved)
}
