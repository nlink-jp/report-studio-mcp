package docx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// rich exercises every block and inline the writer knows.
const rich = `---
title: 調査報告
author: 調査担当
---

# 調査の概要

本文の段落です。**強調**と*斜体*と~~取り消し~~と` + "`code`" + `と[リンク](https://example.com/a)。
日本語の行は
つながる。脚注を付ける[^1]。

## 観測した事象

| 宛先 | 件数 | 割合 |
|:-----|-----:|:----:|
| 社内 | 1,204 | 62% |
| 社外 | 731 | 38% |

> 引用の段落
>
> > 入れ子の引用

> [!WARNING]
> 注意の本文

1. 一つ目
2. 二つ目
   - 入れ子
- [ ] 未完了
- [x] 完了

![宛先別の件数](img/chart.png)

` + "```go\nfunc main() {\n\tfmt.Println(\"x  y\")\n}\n```" + `

` + "```mermaid\ngraph TD; A-->B\n```" + `

` + "```chart\n{\"type\":\"bar\"}\n```" + `

---

#### 深い見出し

<div>raw block</div>

[^1]: 出典: 試験用の架空の観測値。
`

func TestPackageIsSound(t *testing.T) {
	o := fakeOptions(t, nil)
	o.Cover, o.TOC = true, true
	o.Meta.Classification = "社外秘"
	o.Logo = &Picture{PNG: pngOf(t, 10, 10, colorRed), WidthPt: 100, HeightPt: 50}
	data, _ := write(t, rich, o)
	p := checkPackage(t, data)
	want := []string{"[Content_Types].xml", "_rels/.rels", "docProps/core.xml", "word/document.xml",
		"word/styles.xml", "word/settings.xml", "word/numbering.xml", "word/footnotes.xml",
		"word/header1.xml", "word/footer1.xml", "word/header2.xml", "word/footer2.xml",
		"word/_rels/document.xml.rels"}
	for i, n := range want {
		if i >= len(p.names) || p.names[i] != n {
			t.Fatalf("zip entries %v; want them to start with %v", p.names, want)
		}
	}
	for _, n := range p.names[len(want):] {
		if !strings.HasPrefix(n, "word/media/image") || !strings.HasSuffix(n, ".png") {
			t.Errorf("unexpected part %s", n)
		}
	}
	if _, ok := p.parts["docProps/app.xml"]; ok {
		t.Error("app.xml written")
	}
	// LibreOffice refuses a package with a stored entry written with a data
	// descriptor, which is how Go writes stored entries: deflate everything.
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Method != zip.Deflate || !f.Modified.Equal(fixedTime) {
			t.Errorf("%s: method %d, time %v; want deflated at the fixed time", f.Name, f.Method, f.Modified)
		}
	}
}

// TestSpecCorpus is the robustness net: every CommonMark and GFM example
// becomes a sound package without error.
func TestSpecCorpus(t *testing.T) {
	for _, file := range []string{"commonmark-0.31.2.json", "gfm-0.29-gfm-extensions.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "markdown", "testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		var ex []struct {
			Markdown string `json:"markdown"`
			Example  int    `json:"example"`
		}
		if err := json.Unmarshal(raw, &ex); err != nil {
			t.Fatal(err)
		}
		if len(ex) < 20 {
			t.Fatalf("%s: only %d examples", file, len(ex))
		}
		for _, e := range ex {
			for _, front := range []bool{false, true} {
				o := fakeOptions(t, nil)
				o.Cover, o.TOC = front, front
				data, _, err := Write(markdown.Parse([]byte(e.Markdown)), o)
				if err != nil {
					t.Errorf("%s example %d: %v", file, e.Example, err)
					continue
				}
				t.Run("", func(t *testing.T) {
					checkPackage(t, data)
				})
				if t.Failed() {
					t.Fatalf("%s example %d (front %v): %q", file, e.Example, front, e.Markdown)
				}
			}
		}
	}
}

func TestTextutilRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("textutil"); err != nil {
		t.Skip("textutil not available")
	}
	data, _ := write(t, rich, fakeOptions(t, nil))
	f := filepath.Join(t.TempDir(), "r.docx")
	if err := os.WriteFile(f, data, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("textutil", "-convert", "txt", "-stdout", f).Output()
	if err != nil {
		t.Fatalf("textutil: %v", err)
	}
	got := string(out)
	for _, s := range []string{"調査の概要", "本文の段落です。", "強調", "リンク", "日本語の行はつながる。",
		"宛先", "件数", "割合", "社内", "1,204", "62%", "社外", "731", "38%",
		"引用の段落", "入れ子の引用", "Warning", "注意の本文", "一つ目", "入れ子", "☐ 未完了", "☑ 完了",
		"宛先別の件数", "fmt.Println", "深い見出し", "raw block"} {
		if !strings.Contains(got, s) {
			t.Errorf("textutil output lacks %q:\n%s", s, got)
		}
	}
}

func TestListsRestartAndNest(t *testing.T) {
	md := "1. a\n2. b\n\npara\n\n3. c\n4. d\n   1. x\n   2. y\n\n- p\n  - q\n"
	data, _ := write(t, md, fakeOptions(t, nil))
	p := checkPackage(t, data)
	num := parseXML(t, "numbering", p.parts["word/numbering.xml"])

	if n := len(num.all("abstractNum")); n != 2 {
		t.Errorf("%d abstractNum; want 2 (bullets, decimals)", n)
	}
	type inst struct{ abs, ilvl, start string }
	insts := map[string]inst{}
	var order []string
	for _, n := range num.all("num") {
		id := n.attr["numId"]
		x := inst{abs: firstChild(n, "abstractNumId").attr["val"]}
		if o := firstChild(n, "lvlOverride"); o != nil {
			x.ilvl = o.attr["ilvl"]
			if s := firstChild(o, "startOverride"); s != nil {
				x.start = s.attr["val"]
			}
		}
		insts[id] = x
		order = append(order, id)
	}
	ps := paragraphs(t, p.parts["word/document.xml"])
	a, b, c, d := findPara(t, ps, "a"), findPara(t, ps, "b"), findPara(t, ps, "c"), findPara(t, ps, "d")
	x, y := findPara(t, ps, "x"), findPara(t, ps, "y")
	if a.numID == "" || a.numID != b.numID {
		t.Errorf("a, b numbered %q, %q; want one instance", a.numID, b.numID)
	}
	if c.numID == a.numID || c.numID != d.numID {
		t.Errorf("second list numbered %q, %q, first %q: each ordered list needs its own instance", c.numID, d.numID, a.numID)
	}
	if got := insts[a.numID]; got.start != "1" || got.ilvl != "0" || got.abs != "1" {
		t.Errorf("first list instance %+v; want decimals restarting level 0 at 1", got)
	}
	if got := insts[c.numID]; got.start != "3" || got.ilvl != "0" || got.abs != "1" {
		t.Errorf("second list instance %+v; want decimals restarting level 0 at 3 (List.Start)", got)
	}
	if x.ilvl != "1" || y.ilvl != "1" || x.numID == c.numID {
		t.Errorf("nested ordered list at ilvl %q/%q, num %q; want level 1 with its own instance", x.ilvl, y.ilvl, x.numID)
	}
	if got := insts[x.numID]; got.start != "1" || got.ilvl != "1" {
		t.Errorf("nested instance %+v; want restart of level 1 at 1", got)
	}
	pp, q := findPara(t, ps, "p"), findPara(t, ps, "q")
	if pp.numID != q.numID || insts[pp.numID].abs != "0" || pp.ilvl != "0" || q.ilvl != "1" {
		t.Errorf("bullets: p %+v q %+v; want the bullet instance at levels 0 and 1", pp, q)
	}
	if len(order) != 4 {
		t.Errorf("%d numbering instances; want bullets + 3 ordered lists", len(order))
	}
	// Continuation paragraphs of an item are indented but not numbered.
	data, _ = write(t, "1. first\n\n   more\n", fakeOptions(t, nil))
	ps = paragraphs(t, unzipDocx(t, data).parts["word/document.xml"])
	if m := findPara(t, ps, "more"); m.numID != "" || firstChild(firstChild(m.n, "pPr"), "ind") == nil {
		t.Errorf("continuation paragraph numbered %q or not indented", m.numID)
	}
}

func TestTaskItems(t *testing.T) {
	data, _ := write(t, "- [ ] todo\n- [x] done\n- plain\n", fakeOptions(t, nil))
	ps := paragraphs(t, unzipDocx(t, data).parts["word/document.xml"])
	if p := findPara(t, ps, "☐ todo"); p.numID != "" {
		t.Errorf("task item also bulleted: %+v", p)
	}
	findPara(t, ps, "☑ done")
	if p := findPara(t, ps, "plain"); p.numID != "1" {
		t.Errorf("plain item numId %q; want the bullets", p.numID)
	}
}

func TestFootnotes(t *testing.T) {
	md := "A[^1] B[^n] again[^1].\n\n[^1]: one\n[^n]: two\n\n    code in note\n"
	doc := markdown.Parse([]byte(md))
	if len(doc.Footnotes) != 2 {
		t.Fatalf("parser gave %d footnotes", len(doc.Footnotes))
	}
	data, _, err := Write(doc, fakeOptions(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	p := checkPackage(t, data)
	body := parseXML(t, "document", p.parts["word/document.xml"])
	var refs []string
	for _, r := range body.all("footnoteReference") {
		refs = append(refs, r.attr["id"])
	}
	if strings.Join(refs, ",") != "1,2" {
		t.Errorf("footnote references %v; want each note referenced once, in order: 1,2", refs)
	}
	if !strings.Contains(body.textOf(), "again1.") {
		t.Errorf("a repeated reference should show its number as text: %q", body.textOf())
	}
	notes := parseXML(t, "footnotes", p.parts["word/footnotes.xml"])
	got := map[string]*xnode{}
	for _, n := range notes.all("footnote") {
		got[n.attr["id"]] = n
	}
	if got["-1"] == nil || got["0"] == nil || got["-1"].first("separator") == nil || got["0"].first("continuationSeparator") == nil {
		t.Error("separators missing")
	}
	if len(got) != 2+len(doc.Footnotes) {
		t.Errorf("%d footnotes in footnotes.xml; want separators + %d", len(got), len(doc.Footnotes))
	}
	for _, f := range doc.Footnotes {
		n := got[fmt.Sprint(f.Index)]
		if n == nil {
			t.Errorf("footnote %d missing", f.Index)
			continue
		}
		if n.first("footnoteRef") == nil {
			t.Errorf("footnote %d has no reference mark", f.Index)
		}
		if ps := n.all("p"); len(ps) == 0 || firstChild(firstChild(ps[0], "pPr"), "pStyle").attr["val"] != "FootnoteText" {
			t.Errorf("footnote %d does not start with a FootnoteText paragraph", f.Index)
		}
	}
	if s := got["1"].textOf(); !strings.Contains(s, "one") {
		t.Errorf("footnote 1 text %q", s)
	}
	if s := got["2"].textOf(); !strings.Contains(s, "two") || !strings.Contains(s, "code in note") {
		t.Errorf("footnote 2 text %q", s)
	}
	settings := string(p.parts["word/settings.xml"])
	if !strings.Contains(settings, `<w:footnotePr><w:footnote w:id="-1"/><w:footnote w:id="0"/></w:footnotePr>`) {
		t.Errorf("settings lack footnotePr: %s", settings)
	}

	data, _ = write(t, "no notes\n", fakeOptions(t, nil))
	p = checkPackage(t, data)
	if _, ok := p.parts["word/footnotes.xml"]; ok {
		t.Error("footnotes.xml without footnotes")
	}
	if strings.Contains(string(p.parts["word/settings.xml"]), "footnotePr") {
		t.Error("footnotePr without footnotes")
	}
}

func TestLinkSchemes(t *testing.T) {
	md := "[a](https://a.example/x) [b](HTTP://B.example) [c](mailto:c@example.com)\n" +
		"[d](javascript:alert(1)) [e](ftp://e.example) [f](relative/path) <file:///etc/passwd> [g](hxxp://bad[.]example)\n\n" +
		"defanged hxxp://bad[.]example stays text\n"
	data, warns := write(t, md, fakeOptions(t, nil))
	p := checkPackage(t, data)
	var ext []string
	for _, r := range parseXML(t, "rels", p.parts["word/_rels/document.xml.rels"]).all("Relationship") {
		if r.attr["TargetMode"] == "External" {
			ext = append(ext, r.attr["Target"])
		}
	}
	want := []string{"https://a.example/x", "HTTP://B.example", "mailto:c@example.com"}
	if strings.Join(ext, " ") != strings.Join(want, " ") {
		t.Errorf("external targets %v; want %v", ext, want)
	}
	doc := parseXML(t, "document", p.parts["word/document.xml"])
	if n := len(doc.all("hyperlink")); n != 3 {
		t.Errorf("%d hyperlinks; want 3", n)
	}
	text := doc.textOf()
	for _, s := range []string{"d", "e", "f", "file:///etc/passwd", "g", "hxxp://bad[.]example stays text"} {
		if !strings.Contains(text, s) {
			t.Errorf("link text %q not shown", s)
		}
	}
	n := 0
	for _, w := range warns {
		if w.Code == "link_scheme" {
			n++
			if w.Line != 1 || !strings.Contains(w.Message, "only http, https and mailto") {
				t.Errorf("warning %+v", w)
			}
		}
	}
	if n != 5 {
		t.Errorf("%d link_scheme warnings; want 5: %+v", n, warns)
	}
}

func TestPictures(t *testing.T) {
	c := &calls{}
	o := fakeOptions(t, c)
	md := "![宛先別の件数](img/a.png)\n\ntext ![inline](img/b.png) more\n\n" +
		"```mermaid\ngraph TD; A-->B\n```\n\n```mermaid\nbad diagram\n```\n\n```chart\n{}\n```\n\n- ```chart\n  {}\n  ```\n"
	data, warns := write(t, md, o)
	p := checkPackage(t, data)
	doc := parseXML(t, "document", p.parts["word/document.xml"])
	th := o.Theme
	ext := doc.all("extent")
	if len(ext) != 4 { // image, diagram, two charts
		t.Fatalf("%d pictures; want 4", len(ext))
	}
	maxCX := emu(th.ContentW())
	cx, cy := ext[0].attr["cx"], ext[0].attr["cy"]
	if cx != fmt.Sprint(maxCX) || cy != fmt.Sprint(emu(th.ContentW()/2)) {
		t.Errorf("800 x 400 pt image extent %s x %s; want the column width %d, aspect kept", cx, cy, maxCX)
	}
	for _, e := range ext {
		var v int64
		fmt.Sscan(e.attr["cx"], &v)
		if v > maxCX {
			t.Errorf("picture wider than the column: %d", v)
		}
	}
	prs := doc.all("docPr")
	if prs[0].attr["name"] != "Picture 1" || prs[0].attr["descr"] != "宛先別の件数" {
		t.Errorf("docPr %v; want generic name and the caption as description", prs[0].attr)
	}
	for i, pr := range prs[1:] {
		if pr.attr["name"] != fmt.Sprintf("Picture %d", i+2) {
			t.Errorf("docPr name %q", pr.attr["name"])
		}
		if _, ok := pr.attr["descr"]; ok {
			t.Errorf("uncaptioned picture has a description %q", pr.attr["descr"])
		}
	}
	ps := paragraphs(t, p.parts["word/document.xml"])
	if cp := findPara(t, ps, "宛先別の件数"); cp.style != "Caption" {
		t.Errorf("caption style %q", cp.style)
	}
	findPara(t, ps, "text [inline] more")
	if bad := findPara(t, ps, "bad diagram"); bad.style != "CodeBlock" {
		t.Errorf("mermaid fallback style %q; want CodeBlock", bad.style)
	}
	codes := map[string][]markdown.Warning{}
	for _, w := range warns {
		codes[w.Code] = append(codes[w.Code], w)
	}
	if w := codes["inline_image"]; len(w) != 1 || w[0].Line != 3 {
		t.Errorf("inline_image warnings %+v", w)
	}
	if w := codes["mermaid_fallback"]; len(w) != 1 || w[0].Message != "SyntaxError: the diagram cannot be read" || w[0].Line != 9 {
		t.Errorf("mermaid_fallback warnings %+v", w)
	}
	if w := codes["chart_labels_thinned"]; len(w) != 2 || w[0].Message != "labels thinned" {
		t.Errorf("chart warnings %+v", w)
	}
	if len(c.chartW) != 2 || c.chartW[0] != th.ContentW() || !(c.chartW[1] < c.chartW[0]) {
		t.Errorf("charts asked at widths %v; want the column, then narrower inside the list", c.chartW)
	}
	if len(c.images) != 1 || c.images[0] != "img/a.png" {
		t.Errorf("images asked %v; the inline one is only described", c.images)
	}
	// One PNG used twice is stored once.
	data, _ = write(t, "![a](x.png)\n\n![b](y.png)\n", o)
	n := 0
	for _, name := range unzipDocx(t, data).names {
		if strings.HasPrefix(name, "word/media/") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d media files for one PNG", n)
	}
}

func TestCallbackErrorsStop(t *testing.T) {
	o := fakeOptions(t, nil)
	data, warns, err := Write(markdown.Parse([]byte("ok\n\n![x](missing.png)\n")), o)
	if err != errMissing || data != nil || warns != nil {
		t.Errorf("image error: got %v (data %d bytes); want the callback's error as is", err, len(data))
	}
	boom := errors.New("too many points")
	o.Chart = func(string, int, float64) (Picture, []string, error) { return Picture{}, nil, boom }
	if _, _, err := Write(markdown.Parse([]byte("```chart\n{}\n```\n")), o); err != boom {
		t.Errorf("chart error %v", err)
	}
	o.Mermaid = func(string, int) (Picture, string, error) { return Picture{}, "", boom }
	if _, _, err := Write(markdown.Parse([]byte("```mermaid\nx\n```\n")), o); err != boom {
		t.Errorf("mermaid error %v", err)
	}
}

func TestTables(t *testing.T) {
	o := fakeOptions(t, nil)
	md := "| a | a much longer column of text that goes on | c |\n|:--|:-:|--:|\n| 1 | 2 | 3 |\n| 4 | 5 | 6 |\n" +
		"\n- item\n\n  | x | y |\n  |---|---|\n  | 1 | 2 |\n"
	data, _ := write(t, md, o)
	p := checkPackage(t, data)
	doc := parseXML(t, "document", p.parts["word/document.xml"])
	tbls := doc.all("tbl")
	if len(tbls) != 2 {
		t.Fatalf("%d tables", len(tbls))
	}
	th := o.Theme
	least := tw(th.TableSize*3 + 2*th.TableSize*0.45)
	grid := func(tb *xnode) []int {
		var ws []int
		for _, g := range tb.all("gridCol") {
			var v int
			fmt.Sscan(g.attr["w"], &v)
			ws = append(ws, v)
		}
		return ws
	}
	ws := grid(tbls[0])
	sum := 0
	for _, v := range ws {
		sum += v
		if v < least {
			t.Errorf("column %d twips narrower than %d", v, least)
		}
	}
	if sum != tw(th.ContentW()) || !(ws[1] > ws[0] && ws[1] > ws[2]) {
		t.Errorf("grid %v (sum %d); want the text width %d, the long column widest", ws, sum, tw(th.ContentW()))
	}
	rows := tbls[0].all("tr")
	if len(rows) != 3 || firstChild(rows[0], "trPr").first("tblHeader") == nil || rows[1].first("tblHeader") != nil {
		t.Error("header row must repeat (tblHeader) and only it")
	}
	if f := rows[0].first("shd"); f == nil || f.attr["fill"] != hexRGB(th.TableHeadBG) {
		t.Error("header row not shaded TableHeadBG")
	}
	var jcs []string
	for _, tc := range rows[1].all("tc") {
		jc := tc.first("jc")
		if jc == nil {
			jcs = append(jcs, "")
		} else {
			jcs = append(jcs, jc.attr["val"])
		}
	}
	if strings.Join(jcs, ",") != "left,center,right" {
		t.Errorf("cell alignment %v", jcs)
	}
	if tbls[0].first("tblLayout").attr["type"] != "fixed" {
		t.Error("table layout not fixed")
	}
	// Inside a list item: indented, narrower, still the remaining width.
	ws2 := grid(tbls[1])
	ind := tbls[1].first("tblInd")
	var in int
	if ind != nil {
		fmt.Sscan(ind.attr["w"], &in)
	}
	if in == 0 || ws2[0]+ws2[1] != tw(th.ContentW()-float64(in)/20) {
		t.Errorf("nested table indent %d grid %v", in, ws2)
	}
	// A table ends the document: a paragraph follows it.
	data, _ = write(t, "| a |\n|---|\n| 1 |\n", o)
	body := parseXML(t, "document", unzipDocx(t, data).parts["word/document.xml"]).first("body")
	if k := body.kids; k[len(k)-2].name != "p" {
		t.Error("no paragraph after a final table")
	}
}

func TestGridWidths(t *testing.T) {
	for _, tc := range []struct {
		units        []float64
		total, least int
	}{
		{[]float64{1, 1, 1}, 9000, 500},
		{[]float64{100, 1, 1}, 9000, 1000},
		{[]float64{0, 0}, 9001, 100},
		{[]float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}, 1000, 300}, // least does not fit: equal shares
	} {
		ws := gridWidths(tc.units, tc.total, tc.least)
		sum := 0
		lo := min(tc.least, tc.total/len(tc.units))
		for _, v := range ws {
			sum += v
			if v < lo {
				t.Errorf("%v: column %d below %d", tc.units, v, lo)
			}
		}
		if sum != tc.total {
			t.Errorf("%v: sum %d; want %d", tc.units, sum, tc.total)
		}
	}
}

func TestBlocksAndStyles(t *testing.T) {
	o := fakeOptions(t, nil)
	md := "# H1\n\n## H2\n\n### H3\n\n#### H4\n\n> outer\n>\n> > inner\n\n> [!NOTE]\n> body\n\n---\n\n" +
		"```\n\tindented  two\n\nlast\n```\n\n<div>\nhtml\n</div>\n\n日本語の\n文章\n\nfoo\nbar\n\nhard  \nbreak\n"
	data, _ := write(t, md, o)
	p := checkPackage(t, data)
	ps := paragraphs(t, p.parts["word/document.xml"])
	for text, style := range map[string]string{"H1": "Heading1", "H2": "Heading2", "H3": "Heading3", "H4": "Heading3",
		"outer": "Quote", "inner": "Quote", "\tindented  two": "CodeBlock", "last": "CodeBlock", "<div>": "CodeBlock"} {
		if got := findPara(t, ps, text).style; got != style {
			t.Errorf("%q in style %q; want %q", text, got, style)
		}
	}
	left := func(text string) int {
		var v int
		fmt.Sscan(firstChild(firstChild(findPara(t, ps, text).n, "pPr"), "ind").attr["left"], &v)
		return v
	}
	if !(left("inner") > left("outer")) {
		t.Error("a nested quote must indent further")
	}
	note := findPara(t, ps, "Note")
	if c := note.n.first("color"); c == nil || c.attr["val"] != hexRGB(o.Theme.Alerts[markdown.AlertNote].Color) || note.n.first("b") == nil {
		t.Error("alert label not bold in the alert colour")
	}
	if b := findPara(t, ps, "body").n.first("pBdr"); b == nil || firstChild(b, "left").attr["color"] != hexRGB(o.Theme.Alerts[markdown.AlertNote].Color) {
		t.Error("alert body lacks the alert-coloured bar")
	}
	findPara(t, ps, "日本語の文章") // no space between CJK across a soft break
	findPara(t, ps, "foo bar")
	if hb := findPara(t, ps, "hardbreak"); hb.n.first("br") == nil {
		t.Error("hard break not a w:br")
	}
	if !bytes.Contains(p.parts["word/document.xml"], []byte(`<w:t xml:space="preserve">indented  two</w:t>`)) {
		t.Error("code spaces not preserved")
	}
	hr := 0
	for _, x := range ps {
		if b := x.n.first("pBdr"); b != nil && firstChild(b, "bottom") != nil && x.text == "" {
			hr++
		}
	}
	if hr != 1 {
		t.Errorf("%d thematic breaks", hr)
	}

	st := parseXML(t, "styles", p.parts["word/styles.xml"])
	styles := map[string]*xnode{}
	for _, s := range st.all("style") {
		styles[s.attr["styleId"]] = s
	}
	for _, id := range []string{"Normal", "Heading1", "Heading2", "Heading3", "Title", "Subtitle", "TOC1", "TOC2", "TOC3",
		"Quote", "CodeBlock", "Caption", "FootnoteText", "FootnoteReference", "Hyperlink", "ReportTable"} {
		if styles[id] == nil {
			t.Errorf("style %s missing", id)
		}
	}
	for i := 1; i <= 3; i++ {
		s := styles[fmt.Sprint("Heading", i)]
		if s.first("outlineLvl").attr["val"] != fmt.Sprint(i-1) || s.first("rFonts").attr["ascii"] != "Fam-"+o.Theme.Heading {
			t.Errorf("Heading%d outline level or font wrong", i)
		}
	}
	if st.first("docDefaults").first("rFonts").attr["eastAsia"] != "Fam-"+o.Theme.Body {
		t.Error("body family not from Family")
	}
	if styles["CodeBlock"].first("rFonts").attr["ascii"] != "Fam-"+o.Theme.Code || styles["CodeBlock"].first("shd").attr["fill"] != hexRGB(o.Theme.CodeBG) {
		t.Error("code block font or shading wrong")
	}
	for i := 1; i <= 3; i++ {
		tab := styles[fmt.Sprint("TOC", i)].first("tab")
		if tab == nil || tab.attr["val"] != "right" || tab.attr["leader"] != "dot" || tab.attr["pos"] != fmt.Sprint(tw(o.Theme.ContentW())) {
			t.Errorf("TOC%d needs a right tab with a dot leader at the text width", i)
		}
	}
}

func TestCoverTOCAndFurniture(t *testing.T) {
	md := "---\ntitle: T\n---\n# One\n\n## Two\n\n> ### In quote\n\n#### Four\n\nbody\n"
	o := fakeOptions(t, nil)
	data, _ := write(t, md, o)
	p := checkPackage(t, data)
	doc := string(p.parts["word/document.xml"])
	for _, s := range []string{"titlePg", "TOC \\o", `w:type="first"`, "headerReference", "bookmarkStart"} {
		if strings.Contains(doc, s) {
			t.Errorf("without cover, TOC or classification the document has %s", s)
		}
	}
	if _, ok := p.parts["word/header1.xml"]; ok {
		t.Error("header part without a classification")
	}
	foot := string(p.parts["word/footer1.xml"])
	if !strings.Contains(foot, " PAGE ") || !strings.Contains(foot, " NUMPAGES ") || !strings.Contains(foot, `<w:jc w:val="center"/>`) {
		t.Errorf("footer lacks centred PAGE / NUMPAGES: %s", foot)
	}
	ps := paragraphs(t, p.parts["word/document.xml"])
	if ps[0].style != "Title" || ps[0].text != "Report title" {
		t.Errorf("without a cover the body opens with the title block; first paragraph %+v", ps[0])
	}

	o.Cover, o.TOC = true, true
	o.Meta.Subtitle, o.Meta.Date, o.Meta.Classification = "Sub", "2026-11-03", "社外秘"
	o.Logo = &Picture{PNG: pngOf(t, 10, 10, colorRed), WidthPt: 1000, HeightPt: 1000}
	data, _ = write(t, md, o)
	p = checkPackage(t, data)
	doc = string(p.parts["word/document.xml"])
	body := parseXML(t, "document", p.parts["word/document.xml"])
	sect := body.first("sectPr")
	if firstChild(sect, "titlePg") == nil {
		t.Error("cover section lacks titlePg")
	}
	kinds := map[string]bool{}
	for _, k := range sect.kids {
		if k.name == "headerReference" || k.name == "footerReference" {
			kinds[k.name+"/"+k.attr["type"]] = true
		}
	}
	for _, k := range []string{"headerReference/default", "footerReference/default", "headerReference/first", "footerReference/first"} {
		if !kinds[k] {
			t.Errorf("section lacks %s", k)
		}
	}
	if h := string(p.parts["word/header1.xml"]); !strings.Contains(h, "社外秘") || !strings.Contains(h, `<w:jc w:val="right"/>`) {
		t.Errorf("header: %s", h)
	}
	for _, n := range []string{"word/header2.xml", "word/footer2.xml"} {
		if x := parseXML(t, n, p.parts[n]); x.textOf() != "" || len(x.all("fldChar")) > 0 {
			t.Errorf("first-page %s not empty", n)
		}
	}
	ps = paragraphs(t, p.parts["word/document.xml"])
	brk := -1
	for i, x := range ps {
		if b := x.n.first("br"); b != nil && b.attr["type"] == "page" {
			brk = i
			break
		}
	}
	cover := ""
	for _, x := range ps[:max(brk, 0)] {
		cover += x.text + "|"
	}
	if brk < 0 || cover != "|Report title|Sub|Author name|2026-11-03|社外秘|" {
		t.Errorf("cover paragraphs %q (break at %d)", cover, brk)
	}
	if e := ps[0].n.first("extent"); e == nil || ps[0].n.first("docPr").attr["descr"] != "" {
		t.Error("logo not on the cover, or it has a description")
	} else {
		var cx int64
		fmt.Sscan(e.attr["cx"], &cx)
		if cx > emu(o.Theme.ContentW()*0.35) {
			t.Errorf("logo %d EMU wide", cx)
		}
	}
	if strings.Contains(doc, "dirty") {
		t.Error("TOC field marked dirty")
	}
	if !strings.Contains(doc, `<w:instrText xml:space="preserve"> TOC \o "1-3" \h \z \u </w:instrText>`) {
		t.Error("TOC field instruction missing")
	}
	marks := map[string]bool{}
	for _, b := range body.all("bookmarkStart") {
		marks[b.attr["name"]] = true
	}
	var entries []string
	for _, x := range ps {
		if strings.HasPrefix(x.style, "TOC") && x.style != "TOCHeading" {
			h := x.n.first("hyperlink")
			if h == nil || !marks[h.attr["anchor"]] {
				t.Errorf("TOC entry %q does not link to a heading bookmark", x.text)
			}
			entries = append(entries, x.style+":"+x.text)
		}
	}
	if got := strings.Join(entries, ","); got != "TOC1:One,TOC2:Two,TOC3:In quote,TOC3:Four" {
		t.Errorf("TOC entries %s", got)
	}
	if findPara(t, ps, o.Theme.TOCTitle).style != "TOCHeading" {
		t.Error("contents title style")
	}
	for _, x := range ps {
		if x.style == "Title" && brk >= 0 && x.n != ps[1].n {
			t.Error("a title block in the body as well as the cover")
		}
	}
	if s := string(p.parts["word/settings.xml"]); strings.Contains(s, "updateFields") {
		t.Error("updateFields set")
	}

	// No headings: no table of contents.
	data, _ = write(t, "just text\n", o)
	if strings.Contains(string(unzipDocx(t, data).parts["word/document.xml"]), "TOC \\o") {
		t.Error("TOC without headings")
	}
}

func TestMetadataHygiene(t *testing.T) {
	tmp := strings.TrimSuffix(os.TempDir(), "/")
	md := "---\ntitle: Clean title\nauthor: A. Writer\n---\n# H\n\n![the caption](/Users/someone/private/fig.png)\n\n" +
		"![second](" + tmp + "/x/fig2.png)\n\n[local](file:///private/var/secret.txt)\n"
	o := fakeOptions(t, nil)
	o.Meta = Meta{Title: "Clean title", Author: "A. Writer"}
	o.Cover, o.TOC = true, true
	data, _ := write(t, md, o)
	p := checkPackage(t, data)
	banned := []string{"/Users/", "/private/", tmp, "fig.png", "fig2.png", "secret.txt"}
	if u := os.Getenv("USER"); len(u) >= 3 && !strings.Contains(md, u) {
		banned = append(banned, u)
	}
	for _, name := range p.names {
		for _, b := range banned {
			if bytes.Contains(p.parts[name], []byte(b)) {
				t.Errorf("%s contains %q", name, b)
			}
		}
	}
	core := parseXML(t, "core", p.parts["docProps/core.xml"])
	var els []string
	for _, k := range core.kids {
		els = append(els, k.name+"="+k.text)
	}
	if strings.Join(els, ";") != "title=Clean title;creator=A. Writer" {
		t.Errorf("core.xml holds %v; want only title and creator", els)
	}
}

func TestControlCharacters(t *testing.T) {
	doc := &markdown.Document{Children: []markdown.Block{
		&markdown.Paragraph{Children: []markdown.Inline{&markdown.Text{Value: "clean"}}, Line: 1},
		&markdown.Paragraph{Children: []markdown.Inline{&markdown.Text{Value: "a\x01b\x0bc\x0cd\x1fe\uFFFEf\uFFFFg\xed\xa0\x80h"}}, Line: 3},
		&markdown.CodeBlock{Text: "x\x00y\n", Line: 5},
		&markdown.Paragraph{Children: []markdown.Inline{&markdown.Text{Value: "tab\tand\x07bell"}}, Line: 8},
	}}
	o := fakeOptions(t, nil)
	o.Meta.Title = "T\x02itle"
	data, warns, err := Write(doc, o)
	if err != nil {
		t.Fatal(err)
	}
	p := checkPackage(t, data)
	for _, name := range p.names {
		if strings.HasSuffix(name, ".png") {
			continue
		}
		for _, r := range string(p.parts[name]) {
			if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0xFFFE || r == 0xFFFF || r == 0xFFFD {
				t.Errorf("%s still holds %U", name, r)
			}
		}
	}
	ps := paragraphs(t, p.parts["word/document.xml"])
	findPara(t, ps, "abcdefgh")
	findPara(t, ps, "xy")
	findPara(t, ps, "tab\tandbell")
	if !strings.Contains(string(p.parts["docProps/core.xml"]), "<dc:title>Title</dc:title>") {
		t.Error("title not cleaned")
	}
	var cw []markdown.Warning
	for _, w := range warns {
		if w.Code == "control_characters" {
			cw = append(cw, w)
		}
	}
	if len(cw) != 1 || cw[0].Line != 3 {
		t.Errorf("control_characters warnings %+v; want one, at line 3", cw)
	}
	if _, warns, _ := Write(markdown.Parse([]byte("fine\n")), fakeOptions(t, nil)); len(warns) != 0 {
		t.Errorf("warnings on clean input: %+v", warns)
	}
}

func TestDeterministic(t *testing.T) {
	o := fakeOptions(t, nil)
	o.Cover, o.TOC = true, true
	o.Meta.Classification = "社外秘"
	a, _ := write(t, rich, o)
	b, _ := write(t, rich, o)
	if !bytes.Equal(a, b) {
		t.Error("two writes of the same input differ")
	}
}

func TestPageSetup(t *testing.T) {
	o := fakeOptions(t, nil)
	o.Theme = theme.Compact()
	data, _ := write(t, "x\n", o)
	sect := parseXML(t, "document", unzipDocx(t, data).parts["word/document.xml"]).first("sectPr")
	sz, mar := sect.first("pgSz"), sect.first("pgMar")
	th := o.Theme
	if sz.attr["w"] != fmt.Sprint(tw(th.PageW)) || sz.attr["h"] != fmt.Sprint(tw(th.PageH)) ||
		mar.attr["left"] != fmt.Sprint(tw(th.MarginL)) || mar.attr["top"] != fmt.Sprint(tw(th.MarginT)) {
		t.Errorf("page %v margins %v", sz.attr, mar.attr)
	}
	if !regexp.MustCompile(`^1\d{4}$`).MatchString(sz.attr["w"]) {
		t.Errorf("A4 width in twips %s", sz.attr["w"])
	}
}

// TestWriteSample writes the rich sample, with cover and table of contents,
// to $DOCX_SAMPLE for opening by hand in Word, Pages or LibreOffice (the
// reader checks no test can make: RFP §4 Phase 3). Skipped unless set.
func TestWriteSample(t *testing.T) {
	out := os.Getenv("DOCX_SAMPLE")
	if out == "" {
		t.Skip("set DOCX_SAMPLE=/path/sample.docx to write the sample")
	}
	o := fakeOptions(t, nil)
	o.Family = func(ps string) string {
		switch {
		case strings.HasPrefix(ps, "HiraginoSans"):
			return "Hiragino Sans"
		case strings.HasPrefix(ps, "HiraMinProN"):
			return "Hiragino Mincho ProN"
		case strings.HasPrefix(ps, "Menlo"):
			return "Menlo"
		}
		return ps
	}
	o.Cover, o.TOC = true, true
	o.Meta = Meta{Title: "調査報告", Subtitle: "試験用の文書", Author: "調査担当", Date: "2026-11-03", Classification: "社外秘"}
	o.Logo = &Picture{PNG: pngOf(t, 40, 20, colorRed), WidthPt: 120, HeightPt: 60}
	data, _ := write(t, rich, o)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
