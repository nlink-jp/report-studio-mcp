package markdown

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type extCase struct {
	name string
	in   string
	want string // dump of the tree
}

func runExtCases(t *testing.T, cases []extCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dump(Parse([]byte(c.in))); got != c.want {
				t.Errorf("input %q\nwant %s\ngot  %s", c.in, c.want, got)
			}
		})
	}
}

// Footnotes as GitHub Docs describes them ("Basic writing and formatting
// syntax", Footnotes): numbered by first reference wherever the definition
// stands, unreferenced definitions dropped, undefined references literal.
func TestFootnotes(t *testing.T) {
	runExtCases(t, []extCase{
		{"basic with hard break and lazy line",
			"Here is a note[^1] and another[^two].\n\n[^1]: First.\n[^two]: Second line one,  \nlazy second line.\n",
			`p@1("Here is a note" fnref1(1) " and another" fnref2(two) ".") [fn1 1](p@3("First.")) [fn2 two](p@4("Second line one," br "lazy second line."))`},
		{"numbered by first reference; labels case-insensitive; unused dropped",
			"b first[^b], then a[^a], b again[^B].\n\n[^a]: A.\n[^b]: B.\n[^unused]: never.\n",
			`p@1("b first" fnref1(b) ", then a" fnref2(a) ", b again" fnref1(B) ".") [fn1 b](p@4("B.")) [fn2 a](p@3("A."))`},
		{"undefined reference stays text",
			"missing [^nope] stays.\n",
			`p@1("missing [^nope] stays.")`},
		{"indented continuation blocks",
			"x[^n]\n\n[^n]: first para\n\n    second para\n\n    - item\n\nafter\n",
			`p@1("x" fnref1(n)) p@9("after") [fn1 n](p@3("first para") p@5("second para") ul@7(li@7(p@7("item"))))`},
		{"reference inside a footnote",
			"x[^a]\n\n[^a]: see[^b]\n[^b]: inner\n",
			`p@1("x" fnref1(a)) [fn1 a](p@3("see" fnref2(b))) [fn2 b](p@4("inner"))`},
		{"definition before reference, in the middle of the text",
			"[^m]: middle\n\ntext[^m]\n",
			`p@3("text" fnref1(m)) [fn1 m](p@1("middle"))`},
		{"inline link wins over footnote syntax",
			"[^1](/u)\n\n[^1]: n\n",
			`p@1(a[/u]("^1"))`},
		{"emphasis inside the label does not leak",
			"a[^x*y] *b*\n\n[^x*y]: n\n",
			`p@1("a" fnref1(x*y) " " em("b")) [fn1 x*y](p@3("n"))`},
		{"inside an image description: literal text, definition dropped",
			"x ![a[^1]](i.png)\n\n[^1]: n\n",
			`p@1("x " img[i.png]("a[^1]"))`},
		{"inside an image description and outside it",
			"![a[^1] *e*](i.png) b[^1]\n\n[^1]: n\n",
			`p@1(img[i.png]("a[^1] " em("e")) " b" fnref1(1)) [fn1 1](p@3("n"))`},
		{"a label with whitespace matches no definition",
			"x[^ 1] y[^1 ]\n\n[^1]: n\n",
			`p@1("x[^ 1] y[^1 ]")`},
		{"an escaped bracket may stand in a label",
			"x[^a\\[b]\n\n[^a\\[b]: n\n",
			`p@1("x" fnref1(a\[b)) [fn1 a\[b](p@3("n"))`},
		{"nested openers: only the innermost is a reference",
			"[^[^1]]\n\n[^1]: n\n",
			`p@1("[^" fnref1(1) "]") [fn1 1](p@3("n"))`},
	})
}

// Footnote definitions inside list items and block quotes are hoisted into
// Document.Footnotes; what they leave empty is dropped, and the lists that
// remain keep their lines and tightness as the remaining items show them.
func TestHoistedFootnoteDefinitions(t *testing.T) {
	runExtCases(t, []extCase{
		{"in a list item and a block quote",
			"- [^1]: in list\n\n> [^2]: q\n\nx[^1][^2]\n",
			`p@5("x" fnref1(1) fnref2(2)) [fn1 1](p@1("in list")) [fn2 2](p@3("q"))`},
		{"first item dropped: the list starts at the next",
			"- [^1]: a\n- b\n- c\n\nx[^1]\n",
			`ul@2(li@2(p@2("b")) li@3(p@3("c"))) p@5("x" fnref1(1)) [fn1 1](p@1("a"))`},
		{"a blank line only after the dropped item: tight",
			"- [^1]: a\n\n- b\n- c\n\nx[^1]\n",
			`ul@3(li@3(p@3("b")) li@4(p@4("c"))) p@6("x" fnref1(1)) [fn1 1](p@1("a"))`},
		{"a blank line between remaining items across a dropped one: loose",
			"- a\n- [^1]: x\n\n- b\n\ny[^1]\n",
			`ul:loose@1(li@1(p@1("a")) li@4(p@4("b"))) p@6("y" fnref1(1)) [fn1 1](p@2("x"))`},
		{"definition after a paragraph in an item, list stays loose",
			"- a\n\n  [^1]: n\n- b\n\nx[^1]\n",
			`ul:loose@1(li@1(p@1("a")) li@4(p@4("b"))) p@6("x" fnref1(1)) [fn1 1](p@3("n"))`},
		{"definition at the end of the last item: tight",
			"- a\n- b\n\n  [^1]: n\n\nx[^1]\n",
			`ul@1(li@1(p@1("a")) li@2(p@2("b"))) p@6("x" fnref1(1)) [fn1 1](p@4("n"))`},
		{"quote in an item holding only a definition",
			"- > [^1]: q\n- b\n\nx[^1]\n",
			`ul@2(li@2(p@2("b"))) p@4("x" fnref1(1)) [fn1 1](p@1("q"))`},
		{"unreferenced definition in a quote: the quote still goes",
			"> [^u]: never\n\ntext\n",
			`p@3("text")`},
		{"empty items written as such stay",
			"- \n- [ ]\n> \n",
			`ul@1(li@1() li[ ]@2()) quote@3()`},
	})
}

// Alerts as GitHub Docs describes them ("Basic writing and formatting
// syntax", Alerts).
func TestAlerts(t *testing.T) {
	runExtCases(t, []extCase{
		{"five kinds; kind matched case-insensitively",
			"> [!NOTE]\n> Useful.\n\n> [!tip]\n> lower\n\n> [!IMPORTANT]\n> a\n\n> [!WARNING]\n> b\n\n> [!CAUTION]\n> c\n",
			`alert:NOTE@1(p@2("Useful.")) alert:TIP@4(p@5("lower")) alert:IMPORTANT@7(p@8("a")) alert:WARNING@10(p@11("b")) alert:CAUTION@13(p@14("c"))`},
		{"alerts cannot nest",
			"> > [!NOTE]\n> > nested\n\n- > [!NOTE]\n  > in list\n",
			`quote@1(quote@1(p@1("[!NOTE]" sb "nested"))) ul@4(li@4(quote@4(p@4("[!NOTE]" sb "in list"))))`},
		{"marker must stand alone, be known, and have content",
			"> [!NOTE] same line\n\n> [!FOO]\n> x\n\n> [!NOTE]\n",
			`quote@1(p@1("[!NOTE] same line")) quote@3(p@3("[!FOO]" sb "x")) quote@6(p@6("[!NOTE]"))`},
		{"several blocks",
			"> [!WARNING]\n> para one\n>\n> - item\n",
			`alert:WARNING@1(p@2("para one") ul@4(li@4(p@4("item"))))`},
		{"blank line after the marker",
			"> [!NOTE]\n>\n> after blank\n",
			`alert:NOTE@1(p@3("after blank"))`},
		{"inside a footnote it is a quote",
			"x[^a]\n\n[^a]: text\n\n    > [!NOTE]\n    > q\n",
			`p@1("x" fnref1(a)) [fn1 a](p@3("text") quote@5(p@5("[!NOTE]" sb "q")))`},
	})
}

// GFM task list items (GFM 0.29-gfm §5.3) beyond the spec's two examples.
func TestTaskListItems(t *testing.T) {
	runExtCases(t, []extCase{
		{"markers",
			"- [ ] a\n- [x] b\n- [X] c\n- [ ]\n- [ ]x\n\n1. [x] d\n",
			`ul@1(li[ ]@1(p@1("a")) li[x]@2(p@2("b")) li[x]@3(p@3("c")) li[ ]@4() li@5(p@5("[ ]x"))) ol1@7(li[x]@7(p@7("d")))`},
		{"loose list",
			"- [x] loose\n\n- [ ] item\n",
			`ul:loose@1(li[x]@1(p@1("loose")) li[ ]@3(p@3("item")))`},
		{"marker not on the item's first line",
			"-\n  [ ] later\n",
			`ul@1(li@1(p@2("[ ] later")))`},
		{"only the first block counts",
			"- > [ ] quoted\n",
			`ul@1(li@1(quote@1(p@1("[ ] quoted"))))`},
	})
}

func TestTables(t *testing.T) {
	runExtCases(t, []extCase{
		{"paragraph line above the header stays a paragraph; tables in quotes",
			"intro\n| a | b |\n| :- | -: |\n| 1 | 2 |\n\n> | q |\n> |---|\n> | r |\n",
			`p@1("intro") table@2([1 3] ["a" | "b"] ["1" | "2"]) quote@6(table@6([0] ["q"] ["r"]))`},
		{"raw HTML in a cell carries the row's line",
			"| a |\n| - |\n| <b>x</b> |\n",
			`table@1([0] ["a"] [rawhtml("<b>") "x" rawhtml("</b>")])`},
		{"table in a list item, ended by the next item",
			"- | a |\n  | - |\n  | b |\n- c\n",
			`ul@1(li@1(table@1([0] ["a"] ["b"])) li@4(p@4("c")))`},
		{"a row of only a pipe ends the table",
			"| a |\n| - |\n|\n",
			`table@1([0] ["a"]) p@3("|")`},
	})
}

func TestStrikethrough(t *testing.T) {
	runExtCases(t, []extCase{
		{"one or two tildes, matched by length",
			"~a~ and ~~b~~ and ~~c~ and ~~~d~~~\n",
			`p@1(del("a") " and " del("b") " and ~~c~ and ~~~d~~~")`},
		{"nests with emphasis",
			"~~**bold** gone~~\n",
			`p@1(del(strong("bold") " gone"))`},
		{"intraword",
			"a~~b~~c\n",
			`p@1("a" del("b") "c")`},
	})
}

func TestExtendedAutolinks(t *testing.T) {
	runExtCases(t, []extCase{
		{"not inside link text; boundary required",
			"[see www.example.com](https://x.test) www.example.com 参照www.example.com (www.example.com)\n",
			`p@1(a[https://x.test]("see www.example.com") " " auto[http://www.example.com]("www.example.com") " 参照www.example.com (" auto[http://www.example.com]("www.example.com") ")")`},
		{"path is not emphasis",
			"https://example.com/a_b_c *https://example.com/x*\n",
			`p@1(auto[https://example.com/a_b_c]("https://example.com/a_b_c") " " em(auto[https://example.com/x]("https://example.com/x")))`},
		{"underscore in the last two segments",
			"www.a_b.example.org www.example.a_b www.example_x.org www.x_y.example.com.\n",
			`p@1(auto[http://www.a_b.example.org]("www.a_b.example.org") " www.example.a_b www.example_x.org " auto[http://www.x_y.example.com]("www.x_y.example.com") ".")`},
		{"www. alone is not a link",
			"www. and www..x\n",
			`p@1("www. and www..x")`},
		{"percent-encoding of the destination",
			"https://例え.jp/パス\n",
			`p@1(auto[https://%E4%BE%8B%E3%81%88.jp/%E3%83%91%E3%82%B9]("https://例え.jp/パス"))`},
		{"email after an escaped underscore",
			"a\\_b@example.com\n",
			`p@1(auto[mailto:a_b@example.com]("a_b@example.com"))`},
		{"email not inside a link",
			"[x@example.com](/u)\n",
			`p@1(a[/u]("x@example.com"))`},
		{"two emails, a failing one between",
			"a@b.c d@e f@g.h\n",
			`p@1(auto[mailto:a@b.c]("a@b.c") " d@e " auto[mailto:f@g.h]("f@g.h"))`},
		{"defanged indicators are not links",
			"user@mail.evil[.]com https://login.evil.co[.]uk/path www.evil.example[.]com\n",
			`p@1("user@mail.evil[.]com https://login.evil.co[.]uk/path www.evil.example[.]com")`},
		{"defanged with parentheses, a bracketed at, a dot before the bracket",
			"www.evil(.)com a@b.evil(.)com https://x.evil(.)com/p a@mail.evil.[.]com http://evil[:]8080 a@evil.com[@]x\n",
			`p@1("www.evil(.)com a@b.evil(.)com https://x.evil(.)com/p a@mail.evil.[.]com http://evil[:]8080 a@evil.com[@]x")`},
		{"ordinary neighbours still link",
			"see https://example.com. (https://example.com) www.example.com/a[1] mail a@b.com, or https://example.com/(x)\n",
			`p@1("see " auto[https://example.com]("https://example.com") ". (" auto[https://example.com]("https://example.com") ") " auto[http://www.example.com/a%5B1%5D]("www.example.com/a[1]") " mail " auto[mailto:a@b.com]("a@b.com") ", or " auto[https://example.com/(x)]("https://example.com/(x)"))`},
		{"explicit links and <> autolinks are not affected",
			"[x](https://evil[.]com) <https://evil.com>[.]\n",
			`p@1(a[https://evil%5B.%5Dcom]("x") " " auto[https://evil.com]("https://evil.com") "[.]")`},
	})
}

// A marker alone on a task item's first line: the text, and what is reported
// about it, are on the next line.
func TestTaskMarkerAloneOnItsLine(t *testing.T) {
	for _, in := range []string{"- [ ]\n  <i>\n", "- [x] \t\n  <i>\n"} {
		d := Parse([]byte(in))
		want := `ul@1(li` + map[bool]string{true: "[x]", false: "[ ]"}[strings.Contains(in, "[x]")] + `@1(p@2(rawhtml("<i>"))))`
		if got := dump(d); got != want {
			t.Errorf("%q: want %s\ngot  %s", in, want, got)
		}
		if got := warningKeys(d); !reflect.DeepEqual(got, []string{"2:raw_html"}) {
			t.Errorf("%q: warnings %v", in, got)
		}
	}
	// A reference definition on the next line stays text, as before.
	if got, want := dump(Parse([]byte("- [ ]\n  [a]: /u\n"))), `ul@1(li[ ]@1(p@2("[a]: /u")))`; got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
}

// normalizeLabel's ASCII fast path agrees with the general folding on every
// short string over an alphabet of the cases that matter.
func TestNormalizeLabelASCIIFastPath(t *testing.T) {
	const alphabet = "aZ \t\n\r\f-[]"
	var gen func(prefix string, n int)
	count := 0
	gen = func(prefix string, n int) {
		count++
		if got, want := normalizeLabel(prefix), foldLabel(trimSpaceTab(prefix)); got != want {
			t.Fatalf("%q: fast %q, general %q", prefix, got, want)
		}
		if n == 0 {
			return
		}
		for i := 0; i < len(alphabet); i++ {
			gen(prefix+alphabet[i:i+1], n-1)
		}
	}
	gen("", 4)
	if count < 10000 {
		t.Fatalf("only %d strings", count)
	}
	if got := normalizeLabel("Straße  X"); got != "strasse x" {
		t.Errorf("non-ASCII label: %q", got)
	}
}

func TestFrontMatter(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		meta     map[string]string
		warnings []string // "line:code"
	}{
		{"keys, quotes, case, unknown key, invalid line, real body lines",
			"---\nTitle: \"My Report\"\nauthor: Me\nfoo: bar\nnot a pair\n\ntoc: true\n---\n# H\n\n<i>x</i>\n",
			`h1@9("H") p@11(rawhtml("<i>") "x" rawhtml("</i>"))`,
			map[string]string{"title": "My Report", "author": "Me", "toc": "true"},
			[]string{"4:unknown_front_matter_key", "5:invalid_front_matter_line", "11:raw_html", "11:raw_html"}},
		{"every accepted key",
			"---\ntitle: T\nsubtitle: S\nauthor: A\ndate: 2026-11-03\nclassification: 社外秘\nprofile: formal\ncover: true\ntoc: false\n---\n",
			``,
			map[string]string{"title": "T", "subtitle": "S", "author": "A", "date": "2026-11-03", "classification": "社外秘", "profile": "formal", "cover": "true", "toc": "false"},
			nil},
		{"no closing line: plain Markdown, reported when a known key follows",
			"---\ntitle: x\n# no closing\n",
			`hr@1() p@2("title: x") h1@3("no closing")`, map[string]string{},
			[]string{"1:unclosed_front_matter"}},
		{"no closing line and no key: plain Markdown, nothing reported",
			"---\nsome text\n",
			`hr@1() p@2("some text")`, map[string]string{}, nil},
		{"no closing line, an unknown key: not reported",
			"---\nfoo: x\n",
			`hr@1() p@2("foo: x")`, map[string]string{}, nil},
		{"fences may have trailing spaces and tabs",
			"--- \ntitle: T\n---\t \n# x\n",
			`h1@4("x")`, map[string]string{"title": "T"}, nil},
		{"a fence is three hyphens, no more",
			"----\ntitle: x\n----\n",
			`hr@1() h2@2("title: x")`, map[string]string{}, nil},
		{"duplicate key: reported, the later value wins",
			"---\ntitle: one\nTitle: two\n---\n",
			``, map[string]string{"title": "two"}, []string{"3:duplicate_front_matter_key"}},
		{"NUL in a value",
			"---\ntitle: a\x00b\n---\n",
			``, map[string]string{"title": "a�b"}, nil},
		{"only on the first line",
			"\n---\ntitle: x\n---\n",
			`hr@2() h2@3("title: x")`, map[string]string{}, nil},
		{"CRLF",
			"---\r\ntitle: crlf\r\n---\r\nbody\r\n",
			`p@4("body")`, map[string]string{"title": "crlf"}, nil},
		{"nesting and arrays are not accepted",
			"---\nauthor:\n  name: x\n- a\nk:v\n---\n",
			``, map[string]string{"author": ""},
			[]string{"3:invalid_front_matter_line", "4:invalid_front_matter_line", "5:invalid_front_matter_line"}},
		{"closing line at end of input",
			"---\ntitle: t\n---",
			``, map[string]string{"title": "t"}, nil},
		{"byte order mark before the opening line",
			"\uFEFF---\ntitle: bom\n---\nx\n",
			`p@4("x")`, map[string]string{"title": "bom"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Parse([]byte(c.in))
			if got := dump(d); got != c.want {
				t.Errorf("tree\nwant %s\ngot  %s", c.want, got)
			}
			if !reflect.DeepEqual(d.Meta, c.meta) {
				t.Errorf("meta: want %v, got %v", c.meta, d.Meta)
			}
			if got := warningKeys(d); !reflect.DeepEqual(got, c.warnings) {
				t.Errorf("warnings: want %v, got %v", c.warnings, got)
			}
		})
	}
}

func warningKeys(d *Document) []string {
	var out []string
	for _, w := range d.Warnings {
		if w.Message == "" {
			out = append(out, "missing message")
		}
		out = append(out, fmt.Sprintf("%d:%s", w.Line, w.Code))
	}
	return out
}

func TestWarnings(t *testing.T) {
	in := "line one\nline <b>two</b> here\n\n<div>\nblock\n</div>\n\n#### deep\n\n### ok\n###### six\n\n- item\n\n  ##### in list <!-- c -->\n"
	d := Parse([]byte(in))
	want := []string{"2:raw_html", "2:raw_html", "4:raw_html", "8:deep_heading", "11:deep_heading", "15:deep_heading", "15:raw_html"}
	if got := warningKeys(d); !reflect.DeepEqual(got, want) {
		t.Errorf("want %v\ngot  %v", want, got)
	}
}

// Raw HTML that does not reach the tree (an unreferenced footnote) is not
// reported.
func TestWarningsOnlyForWhatIsKept(t *testing.T) {
	d := Parse([]byte("text\n\n[^u]: <b>unused</b>\n"))
	if len(d.Warnings) != 0 {
		t.Errorf("want no warnings, got %v", d.Warnings)
	}
}

func TestLines(t *testing.T) {
	runExtCases(t, []extCase{
		{"after link reference definitions",
			"[a]: /u\n[b]: /v\nreal text\n",
			`p@3("real text")`},
		{"setext heading after a definition",
			"[a]: /u\nTitle\n===\n",
			`h1@2("Title")`},
		{"blocks of every kind",
			"# h\n\n> q\n\n1. a\n2. b\n\n```go\nx\n```\n\n    ind\n\n***\n",
			`h1@1("h") quote@3(p@3("q")) ol1@5(li@5(p@5("a")) li@6(p@6("b"))) code@8("go" "x\n") code@12("" "ind\n") hr@14()`},
	})
}

func TestRenderExtensionsHTML(t *testing.T) {
	got := renderHTML(Parse([]byte("> [!NOTE]\n> n\n\nx[^1]\n\n[^1]: f\n")))
	want := "<div class=\"markdown-alert markdown-alert-note\">\n<p class=\"markdown-alert-title\">Note</p>\n<p>n</p>\n</div>\n" +
		"<p>x<sup class=\"footnote-ref\"><a href=\"#fn-1\" id=\"fnref-1\">1</a></sup></p>\n" +
		"<section class=\"footnotes\">\n<ol>\n<li id=\"fn-1\">\n<p>f</p>\n</li>\n</ol>\n</section>\n"
	if got != want {
		t.Errorf("want %q\ngot  %q", want, got)
	}
}

func TestInputHygiene(t *testing.T) {
	d := Parse([]byte("a\x00b \xff\xfe c\n"))
	got := dump(d)
	want := `p@1("a` + "\uFFFD" + `b ` + "\uFFFD" + ` c")`
	if got != want {
		t.Errorf("want %s\ngot  %s", want, got)
	}
	if s := dump(Parse(nil)); s != "" {
		t.Errorf("empty input: %q", s)
	}
}

// Link labels match after full Unicode case folding (§4.7, example 540
// covers ẞ/SS; these cover the expansion table directly).
func TestLinkLabelCaseFolding(t *testing.T) {
	for _, c := range []struct{ ref, def string }{
		{"Straße", "STRASSE"}, {"ﬃ", "FFI"}, {"ΣΊΣΥΦΟΣ", "σίσυφος"}, {"ſ", "S"}, {"K", "k"},
	} {
		in := "[" + c.ref + "]\n\n[" + c.def + "]: /u\n"
		if got := dump(Parse([]byte(in))); !strings.HasPrefix(got, "p@1(a[/u]") {
			t.Errorf("%q vs %q: %s", c.ref, c.def, got)
		}
	}
}

func TestDirectives(t *testing.T) {
	src := "<!-- pagebreak -->\n\n<!--  Keep-With-Next  -->\n\n<!-- pagebreak --> x\n\n<!-- landscape -->\n\n> <!-- pagebreak -->\n\n<!-- pagebreak --><!-- pagebreak -->\n\n<!-- page break -->\n"
	d := Parse([]byte(src))
	var kinds []string
	for _, b := range d.Children {
		switch v := b.(type) {
		case *Directive:
			kinds = append(kinds, v.Kind)
		case *HTMLBlock:
			kinds = append(kinds, "html")
		case *BlockQuote:
			if _, ok := v.Children[0].(*HTMLBlock); ok {
				kinds = append(kinds, "quote>html")
			}
		}
	}
	want := []string{"pagebreak", "keep-with-next", "html", "html", "quote>html", "html", "html"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("blocks %v, want %v", kinds, want)
	}
	// Directives are not raw HTML; the rest still warns.
	if got := warningKeys(d); !reflect.DeepEqual(got, []string{"5:raw_html", "7:raw_html", "9:raw_html", "11:raw_html", "13:raw_html"}) {
		t.Errorf("warnings %v", got)
	}
	// Without extensions (the CommonMark spec run) a directive is plain HTML.
	if b, ok := parse([]byte("<!-- pagebreak -->\n"), false).Children[0].(*HTMLBlock); !ok || strings.TrimSpace(b.Raw) != "<!-- pagebreak -->" {
		t.Errorf("extensions off: %#v", parse([]byte("<!-- pagebreak -->\n"), false).Children[0])
	}
	// break_before is a front matter key.
	if d := Parse([]byte("---\nbreak_before: h2\n---\n\nx\n")); d.Meta["break_before"] != "h2" || len(d.Warnings) != 0 {
		t.Errorf("front matter %v %v", d.Meta, d.Warnings)
	}
}
