package markdown

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func loadExamples(t *testing.T, path string) []specExample {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ex []specExample
	if err := json.Unmarshal(data, &ex); err != nil {
		t.Fatal(err)
	}
	return ex
}

// TestCommonMarkSpec runs every example of the CommonMark 0.31.2 spec and
// compares the HTML byte for byte. Nothing is normalized: the reference
// harness's normalize.py (whitespace, attribute order, entity forms) is not
// needed, because html.go writes exactly what cmark writes. Extensions are
// off, as cmark-gfm runs this suite.
func TestCommonMarkSpec(t *testing.T) {
	ex := loadExamples(t, "testdata/commonmark-0.31.2.json")
	if len(ex) != 652 {
		t.Fatalf("expected 652 examples, got %d", len(ex))
	}
	failed := 0
	for _, e := range ex {
		got := renderHTML(parse([]byte(e.Markdown), false))
		if got != e.HTML {
			failed++
			t.Errorf("example %d (%s)\ninput: %q\nwant:  %q\ngot:   %q", e.Example, e.Section, e.Markdown, e.HTML, got)
		}
	}
	t.Logf("CommonMark 0.31.2: %d/%d examples pass", len(ex)-failed, len(ex))
}

// TestCommonMarkSpecWithExtensions pins which spec examples change when the
// extensions are on, and why: a leading "---" block is front matter (96, 98)
// and bare URLs and email addresses are GFM extended autolinks (606, 608,
// 611, 612). Any other difference is a regression.
func TestCommonMarkSpecWithExtensions(t *testing.T) {
	ex := loadExamples(t, "testdata/commonmark-0.31.2.json")
	var differ []int
	for _, e := range ex {
		if renderHTML(Parse([]byte(e.Markdown))) != e.HTML {
			differ = append(differ, e.Example)
		}
	}
	want := []int{96, 98, 606, 608, 611, 612}
	if !reflect.DeepEqual(differ, want) {
		t.Errorf("with extensions on, examples %v differ; want %v", differ, want)
	}
}

// TestGFMExtensionSpec runs the examples of the GFM 0.29-gfm extension
// sections (tables, task list items, strikethrough, autolinks), transcribed
// from the spec's HTML. The disallowed-raw-HTML extension is not
// implemented (nothing is rendered to a browser), so example 657 is absent.
func TestGFMExtensionSpec(t *testing.T) {
	ex := loadExamples(t, "testdata/gfm-0.29-gfm-extensions.json")
	if len(ex) != 27 {
		t.Fatalf("expected 27 examples, got %d", len(ex))
	}
	for _, e := range ex {
		got := renderHTML(Parse([]byte(e.Markdown)))
		if got != e.HTML {
			t.Errorf("GFM example %d (%s)\ninput: %q\nwant:  %q\ngot:   %q", e.Example, e.Section, e.Markdown, e.HTML, got)
		}
	}
}
