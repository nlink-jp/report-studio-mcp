package docx

import (
	"strings"
	"testing"
)

// Page breaks and placement (RFP §2, v0.2.0) as paragraph properties.

func has(p para, prop string) bool {
	ppr := firstChild(p.n, "pPr")
	return ppr != nil && firstChild(ppr, prop) != nil
}

func TestCaptionGroupsKeepWithTheirBody(t *testing.T) {
	o := fakeOptions(t, nil)
	data, _ := write(t, "**表1：数値**\n\n*出典: 社内*\n\n| a |\n|---|\n| 1 |\n\n**ただの太字**\n\n本文。\n", o)
	ps := paragraphs(t, unzipDocx(t, data).parts["word/document.xml"])
	for _, c := range []struct {
		text string
		keep bool
	}{{"表1：数値", true}, {"出典: 社内", true}, {"ただの太字", false}, {"本文。", false}} {
		if p := findPara(t, ps, c.text); has(p, "keepNext") != c.keep {
			t.Errorf("%q: keepNext %v, want %v", c.text, !c.keep, c.keep)
		}
	}
}

func TestDirectivesBecomeProperties(t *testing.T) {
	o := fakeOptions(t, nil)
	src := "前。\n\n<!-- pagebreak -->\n\n<!-- pagebreak -->\n\n後。\n\n<!-- keep-with-next -->\n\n前書き。\n\n```sh\ncode\n```\n\n<!-- pagebreak -->\n\n| a |\n|---|\n| 1 |\n"
	data, warns := write(t, src, o)
	xml := unzipDocx(t, data).parts["word/document.xml"]
	ps := paragraphs(t, xml)
	if p := findPara(t, ps, "後。"); !has(p, "pageBreakBefore") {
		t.Error("no pageBreakBefore after the directive")
	}
	if p := findPara(t, ps, "前。"); has(p, "pageBreakBefore") {
		t.Error("pageBreakBefore before the directive")
	}
	if p := findPara(t, ps, "前書き。"); !has(p, "keepNext") {
		t.Error("keep-with-next did not keep")
	}
	if n := strings.Count(string(xml), "pageBreakBefore"); n != 1 {
		t.Errorf("%d pageBreakBefore, want 1 (two directives in a row are one break)", n)
	}
	// A table carries no paragraph properties: the break is a paragraph before it.
	if !strings.Contains(string(xml), `<w:br w:type="page"/></w:r></w:p><w:tbl>`) {
		t.Error("no break before the table")
	}
	for _, w := range warns {
		if w.Code == "raw_html" {
			t.Errorf("directive warned raw_html: %+v", w)
		}
	}
	if strings.Contains(string(xml), "pagebreak") || strings.Contains(string(xml), "keep-with-next") {
		t.Error("a directive is written as text")
	}
}

func TestChapterBreaksInWord(t *testing.T) {
	o := fakeOptions(t, nil)
	o.Theme.BreakBefore = 2
	src := "# 題名\n\n## 1. 概要\n\n本文。\n\n## 2. 事実\n\n### 2.1 節\n\n本文。\n\n> ## 引用の中\n\n## 3. 解釈\n\n本文。\n"
	data, _ := write(t, src, o)
	ps := paragraphs(t, unzipDocx(t, data).parts["word/document.xml"])
	for _, c := range []struct {
		text  string
		brk   bool
		cause string
	}{
		{"題名", false, "the first heading"},
		{"1. 概要", false, "a run of headings breaks once, and only after body text"},
		{"2. 事実", true, "a chapter after body text"},
		{"2.1 節", false, "level 3"},
		{"引用の中", false, "inside a quote"},
		{"3. 解釈", true, "a chapter after body text"},
	} {
		if p := findPara(t, ps, c.text); has(p, "pageBreakBefore") != c.brk {
			t.Errorf("%q: pageBreakBefore %v, want %v (%s)", c.text, !c.brk, c.brk, c.cause)
		}
	}
	// Set per paragraph, not in the heading styles (a quote's heading would
	// break too).
	if strings.Contains(string(unzipDocx(t, data).parts["word/styles.xml"]), "pageBreakBefore") {
		t.Error("pageBreakBefore in the styles")
	}
	// none: no breaks.
	o.Theme.BreakBefore = 0
	data, _ = write(t, src, o)
	if strings.Contains(string(unzipDocx(t, data).parts["word/document.xml"]), "pageBreakBefore") {
		t.Error("break_before none still breaks")
	}
}

func TestWidowControlAndNumberColumns(t *testing.T) {
	o := fakeOptions(t, nil)
	src := "| 区分 | 説明 | 金額 |\n|---|---|---:|\n| あ | " + strings.Repeat("長い説明の文章", 30) + " | 1,234,567,890 |\n"
	data, _ := write(t, src, o)
	p := unzipDocx(t, data)
	styles := parseXML(t, "styles", p.parts["word/styles.xml"])
	normal := false
	for _, s := range styles.all("style") {
		if s.attr["styleId"] == "Normal" {
			normal = firstChild(firstChild(s, "pPr"), "widowControl") != nil
		}
	}
	if !normal {
		t.Error("Normal does not set widowControl")
	}
	var grid []int
	for _, g := range parseXML(t, "document", p.parts["word/document.xml"]).all("gridCol") {
		var v int
		for _, ch := range g.attr["w"] {
			v = v*10 + int(ch-'0')
		}
		grid = append(grid, v)
	}
	// "1,234,567,890" is 13 half-width characters at 0.55 em of 9 pt, plus
	// padding: 13*9*0.55 + 2*4.05 = 72.45 pt = 1449 twips.
	if len(grid) != 3 || grid[2] < 1449 {
		t.Errorf("grid %v: the number column is narrower than its number", grid)
	}
}

func TestKeepWithNextIsIgnoredOnTablesAndLists(t *testing.T) {
	o := fakeOptions(t, nil)
	data, _ := write(t, "<!-- keep-with-next -->\n\n| a |\n|---|\n| 1 |\n\n後。\n\n<!-- keep-with-next -->\n\n- 項目\n\n後。\n", o)
	if strings.Contains(string(unzipDocx(t, data).parts["word/document.xml"]), "keepNext") {
		t.Error("keep-with-next applied to a table or list")
	}
}
