package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNoFileMeansBuiltinsOnly(t *testing.T) {
	path, f, err := Load([]string{filepath.Join(t.TempDir(), "absent.toml")})
	if err != nil || path != "" || len(f.Profiles) != 0 {
		t.Fatalf("%q %+v %v", path, f, err)
	}
	if th, err := f.Theme(""); err != nil || th.Body != theme.Standard().Body {
		t.Errorf("default theme: %v", err)
	}
	if _, err := f.Theme("formal"); err != nil {
		t.Errorf("formal: %v", err)
	}
}

func TestFirstExistingPathWins(t *testing.T) {
	a := write(t, "[profiles.a]\ncover = true\n")
	b := write(t, "[profiles.b]\ncover = true\n")
	path, f, err := Load([]string{filepath.Join(t.TempDir(), "none.toml"), a, b})
	if err != nil || path != a {
		t.Fatalf("read %q, %v", path, err)
	}
	if _, ok := f.Profiles["b"]; ok {
		t.Error("the second file was read too")
	}
}

func TestExtendsAndOverrides(t *testing.T) {
	_, f, err := Load([]string{write(t, `
[profiles.customer]
extends = "formal"
margin_mm = 30
margin_left_mm = 35
body_size = 11
heading_sizes = [18, 14, 12]
link_color = "#112233"
chart_palette = ["#2a78d6", "#eb6834"]
toc = false
logo = "/Library/logo.png"
break_before = "none"
`)})
	if err != nil {
		t.Fatal(err)
	}
	th, err := f.Theme("customer")
	if err != nil {
		t.Fatal(err)
	}
	formal := theme.Formal()
	const mm = 72 / 25.4
	if th.Body != formal.Body || !th.Cover || th.TOC || th.BodySize != 11 || th.HeadingSizes != [3]float64{18, 14, 12} ||
		math.Abs(th.MarginT-30*mm) > 1e-9 || math.Abs(th.MarginL-35*mm) > 1e-9 || th.Link != (theme.RGB{0x11 / 255.0, 0x22 / 255.0, 0x33 / 255.0}) ||
		len(th.Chart.Palette) != 2 || th.Logo != "/Library/logo.png" || th.BreakBefore != 0 {
		t.Errorf("theme %+v", th)
	}
	// The built-in itself is untouched.
	if g, _ := f.Theme("formal"); g.BodySize != formal.BodySize || !g.TOC || g.BreakBefore != 2 {
		t.Error("formal changed")
	}
	if !strings.Contains(strings.Join(f.Names(), ","), "standard,formal,compact,customer") {
		t.Errorf("names %v", f.Names())
	}
}

func TestRefuses(t *testing.T) {
	for body, want := range map[string]string{
		"[profiles.x]\ncolour = \"red\"\n":                   "colour",
		"[profiles.formal]\ncover = false\n":                 "built-in",
		"[profiles.x]\nextends = \"x\"\n":                    "not a built-in",
		"[profiles.x]\nbody_size = 200\n":                    "4 to 72",
		"[profiles.x]\nmargin_mm = 120\n":                    "0 to 60",
		"[profiles.x]\npaper = \"B5\"\n":                     "A4",
		"[profiles.x]\ntext_color = \"red\"\n":               "#rrggbb",
		"[profiles.x]\nheading_sizes = [10, 9]\n":            "three",
		"[profiles.x]\nlogo = \"logo.png\"\n":                "absolute",
		"[profiles.x]\ntable_size = 8\ntable_min_size = 9\n": "larger",
		"[profiles.x]\nchart_palette = []\n":                 "empty",
		"[profiles.x]\nbreak_before = \"h3\"\n":              "h1, h2 or none",
		"[server]\nlog_level = \"debug\"\n":                  "server",
		"[profiles.x\n":                                      "x",
	} {
		_, _, err := Load([]string{write(t, body)})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", body, err, want)
		}
	}
}

func TestUnknownProfileNamesTheAvailable(t *testing.T) {
	_, err := File{}.Theme("fancy")
	if err == nil || !strings.Contains(err.Error(), "standard, formal, compact") {
		t.Fatalf("err = %v", err)
	}
}

func TestPathsOrder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x")
	t.Setenv("HOME", "/h")
	got := strings.Join(Paths(), "|")
	want := "/x/report-studio-mcp/config.toml|/h/.config/report-studio-mcp/config.toml|/h/Library/Application Support/report-studio-mcp/config.toml"
	if got != want {
		t.Errorf("paths %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative")
	if strings.HasPrefix(Paths()[0], "relative") {
		t.Error("a relative XDG_CONFIG_HOME was used")
	}
}
