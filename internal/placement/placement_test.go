package placement

import (
	"fmt"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

func TestCaptionsAreRecognisedOnlyBeforeWhatTheyCaption(t *testing.T) {
	for _, c := range []struct {
		src  string
		want []bool
	}{
		{"**表1：数値**\n\n| a |\n|---|\n| 1 |\n", []bool{true, false}},
		{"表 2: 数値の表\n\n| a |\n|---|\n| 1 |\n", []bool{true, false}},
		{"Figure 3: chart\n\n```chart\n{}\n```\n", []bool{true, false}},
		{"**図1：構成**\n\n*出典: 社内資料*\n\n![x](a.png)\n", []bool{true, true, false}},
		{"**表1：数値**\n*注記: 値は乱数*\n\n| a |\n|---|\n| 1 |\n", []bool{true, false}},
		{"**太字だけの段落**\n\n普通の段落。\n", []bool{false, false}},
		{"**太字** と普通の文。\n\n| a |\n|---|\n| 1 |\n", []bool{false, false}},
		{"*斜体だけ*\n\n| a |\n|---|\n| 1 |\n", []bool{false, false}},
		{"**表1：数値**\n\n<!-- pagebreak -->\n\n| a |\n|---|\n| 1 |\n", []bool{false, false}},
		{"- **表1：一覧**\n\n| a |\n|---|\n| 1 |\n", []bool{false, false}}, // not in the same container
	} {
		var got []bool
		for _, it := range Items(markdown.Parse([]byte(c.src)).Children) {
			got = append(got, it.Caption)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: captions %v, want %v", c.src, got, c.want)
		}
	}
}

func TestDirectivesAreFlagsOnTheNextBlock(t *testing.T) {
	src := "a\n\n<!-- pagebreak -->\n\nb\n\n<!-- keep-with-next -->\n\nc\n\nd\n"
	its := Items(markdown.Parse([]byte(src)).Children)
	if len(its) != 4 {
		t.Fatalf("%d items", len(its))
	}
	if its[0].BreakBefore || !its[1].BreakBefore || its[1].Keep || !its[2].Keep || its[3].Keep {
		t.Errorf("flags %+v", its)
	}
	runs := Runs(its)
	// keep-with-next is ignored on tables, lists and quotes.
	for _, src := range []string{"<!-- keep-with-next -->\n\n| a |\n|---|\n| 1 |\n", "<!-- keep-with-next -->\n\n- x\n", "<!-- keep-with-next -->\n\n> q\n"} {
		if it := Items(markdown.Parse([]byte(src)).Children); it[0].Keep {
			t.Errorf("%q: kept", src)
		}
	}
	// a | b | c+d (kept, d its tail) | d
	if len(runs) != 4 || runs[2].Tail == nil || !runs[2].Kept() || runs[3].Kept() {
		t.Errorf("runs %+v", runs)
	}
}

func TestHeadingRunsAndChapters(t *testing.T) {
	src := "# 題名\n## 1. 概要\n\n本文\n\n### 1.1 節\n\n<!-- pagebreak -->\n\n## 2. 次\n"
	runs := Runs(Items(markdown.Parse([]byte(src)).Children))
	if len(runs) != 4 {
		t.Fatalf("%d runs: %+v", len(runs), runs)
	}
	if len(runs[0].Items) != 2 || runs[0].Tail == nil || !runs[0].Chapter(2) || runs[0].Chapter(0) {
		t.Errorf("first run %+v", runs[0])
	}
	if runs[2].Chapter(2) || !runs[2].Chapter(3) {
		t.Errorf("a level-3 run is not a chapter at h2: %+v", runs[2])
	}
	// A break before the next run ends the tail: nothing to keep with.
	if runs[2].Tail != nil || !runs[3].Items[0].BreakBefore {
		t.Errorf("break: %+v / %+v", runs[2], runs[3])
	}
}

func TestNumberTokensAndTheirUnits(t *testing.T) {
	for _, c := range []struct{ s, tok, unit string }{
		{"合計 1,234,567 件", "1,234,567", "1,234,567"},
		{"（1,234,567）", "1,234,567", "（1,234,567）"},
		{"(12.34%)。", "12.34%", "(12.34%)。"},
		{"差 -1,234", "-1,234", " -1,234"},
		{"あ−1,234", "−1,234", "−1,234"},
		{"▲１２．３４％", "▲１２．３４％", "▲１２．３４％"},
		{"¥1,234,567円", "¥1,234,567", "¥1,234,567"},
	} {
		ms := NumberTokens(c.s)
		if len(ms) != 1 || c.s[ms[0][0]:ms[0][1]] != c.tok {
			t.Errorf("%q: tokens %v", c.s, ms)
			continue
		}
		if u := BoundUnit(c.s, ms[0]); u != c.unit {
			t.Errorf("%q: unit %q, want %q", c.s, u, c.unit)
		}
	}
	if ms := NumberTokens("1 と 2"); len(ms) != 0 {
		t.Errorf("single digits: %v", ms)
	}
	// Identifiers are not numbers: they never widen a column.
	for _, id := range []string{"ID1,234,567", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"550e8400-e29b-41d4-a716-446655440000", "CVE-2024-12345", "x1,234", "v1.2.3"} {
		if ms := NumberTokens("値 " + id + " 件"); len(ms) != 0 {
			t.Errorf("%q: tokens %v", id, ms)
		}
	}
	// An IP address is a number: kept whole.
	if ms := NumberTokens("送信元 192.168.10.25"); len(ms) != 1 {
		t.Errorf("IP address: %v", ms)
	}
}
