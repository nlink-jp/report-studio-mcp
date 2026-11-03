// Package config reads the operator's config.toml: output profiles that take
// a built-in profile as their base and override what they name.
//
// The file is optional and read on every render, so an edit takes effect on
// the next call. It is decoded strictly (nlink-jp/toml): an unknown key is an
// error naming it, never a silently ignored setting. A broken file does not
// stop the server; render reports it.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nlink-jp/toml"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// File is config.toml.
type File struct {
	Profiles map[string]Profile `toml:"profiles"`
}

// Profile overrides a built-in profile. A nil field keeps the base's value.
type Profile struct {
	Extends string `toml:"extends"` // a built-in profile; default "standard"

	Paper    *string  `toml:"paper"`     // "A4" or "Letter"
	MarginMM *float64 `toml:"margin_mm"` // all four sides
	TopMM    *float64 `toml:"margin_top_mm"`
	BottomMM *float64 `toml:"margin_bottom_mm"`
	LeftMM   *float64 `toml:"margin_left_mm"`
	RightMM  *float64 `toml:"margin_right_mm"`

	BodyFont     *string `toml:"body_font"` // PostScript names
	BodyBoldFont *string `toml:"body_bold_font"`
	HeadingFont  *string `toml:"heading_font"`
	CodeFont     *string `toml:"code_font"`

	BodySize     *float64  `toml:"body_size"` // points
	CodeSize     *float64  `toml:"code_size"`
	TableSize    *float64  `toml:"table_size"`
	TableMinSize *float64  `toml:"table_min_size"`
	NoteSize     *float64  `toml:"note_size"`
	TitleSize    *float64  `toml:"title_size"`
	HeadingSizes []float64 `toml:"heading_sizes"` // levels 1, 2, 3
	LineGap      *float64  `toml:"line_gap"`      // extra space between lines, in em

	TextColor      *string  `toml:"text_color"` // "#rrggbb"
	MutedColor     *string  `toml:"muted_color"`
	LinkColor      *string  `toml:"link_color"`
	RuleColor      *string  `toml:"rule_color"`
	CodeBackground *string  `toml:"code_background"`
	ChartPalette   []string `toml:"chart_palette"`

	Cover    *bool   `toml:"cover"`
	TOC      *bool   `toml:"toc"`
	TOCTitle *string `toml:"toc_title"`
	Logo     *string `toml:"logo"` // absolute path (or ~/...) of a PNG or JPEG for the cover
}

// Paths is where config.toml is looked for, in order: $XDG_CONFIG_HOME,
// ~/.config, ~/Library/Application Support.
func Paths() []string {
	var out []string
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		out = append(out, filepath.Join(x, "report-studio-mcp", "config.toml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".config", "report-studio-mcp", "config.toml"),
			filepath.Join(home, "Library", "Application Support", "report-studio-mcp", "config.toml"))
	}
	return out
}

// Load reads the first config.toml that exists among paths. It returns the
// path read ("" when there is none) and the file, empty when absent.
func Load(paths []string) (string, File, error) {
	for _, p := range paths {
		var f File
		_, err := toml.DecodeFile(p, &f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return p, File{}, fmt.Errorf("%s: %w", p, err)
		}
		if err := f.validate(); err != nil {
			return p, File{}, fmt.Errorf("%s: %w", p, err)
		}
		return p, f, nil
	}
	return "", File{}, nil
}

// Names lists the profiles a render may name: the built-ins, then the file's.
func (f File) Names() []string {
	names := theme.BuiltinNames()
	var own []string
	for n := range f.Profiles {
		own = append(own, n)
	}
	slices.Sort(own)
	return append(names, own...)
}

func (f File) validate() error {
	for name, p := range f.Profiles {
		if _, builtin := theme.Builtin(name); builtin {
			return fmt.Errorf("profile %q has a built-in's name: built-in profiles cannot be changed; name yours differently and set extends = %q", name, name)
		}
		if name == "" || strings.ContainsAny(name, " \t") {
			return fmt.Errorf("profile name %q is not usable", name)
		}
		if _, err := p.apply(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
	}
	return nil
}

// Theme returns the named profile: a built-in as it is, or the file's profile
// on top of the built-in it extends.
func (f File) Theme(name string) (theme.Theme, error) {
	if name == "" {
		name = "standard"
	}
	if t, ok := theme.Builtin(name); ok {
		return t, nil
	}
	p, ok := f.Profiles[name]
	if !ok {
		return theme.Theme{}, fmt.Errorf("unknown profile %q: available are %s", name, strings.Join(f.Names(), ", "))
	}
	return p.apply()
}

const mm = 72 / 25.4

// apply returns the built-in the profile extends with its overrides.
func (p Profile) apply() (theme.Theme, error) {
	ext := p.Extends
	if ext == "" {
		ext = "standard"
	}
	t, ok := theme.Builtin(ext)
	if !ok {
		return t, fmt.Errorf("extends %q, which is not a built-in profile (%s)", ext, strings.Join(theme.BuiltinNames(), ", "))
	}
	if p.Paper != nil {
		switch strings.ToLower(*p.Paper) {
		case "a4":
			t.PageW, t.PageH = 210*mm, 297*mm
		case "letter":
			t.PageW, t.PageH = 8.5*72, 11*72
		default:
			return t, fmt.Errorf("paper %q: use \"A4\" or \"Letter\"", *p.Paper)
		}
	}
	margin := func(name string, v *float64, dst ...*float64) error {
		if v == nil {
			return nil
		}
		if !(*v >= 0 && *v <= 60) {
			return fmt.Errorf("%s %v: use 0 to 60 (mm)", name, *v)
		}
		for _, d := range dst {
			*d = *v * mm
		}
		return nil
	}
	for _, m := range []struct {
		name string
		v    *float64
		dst  []*float64
	}{
		{"margin_mm", p.MarginMM, []*float64{&t.MarginT, &t.MarginB, &t.MarginL, &t.MarginR}},
		{"margin_top_mm", p.TopMM, []*float64{&t.MarginT}},
		{"margin_bottom_mm", p.BottomMM, []*float64{&t.MarginB}},
		{"margin_left_mm", p.LeftMM, []*float64{&t.MarginL}},
		{"margin_right_mm", p.RightMM, []*float64{&t.MarginR}},
	} {
		if err := margin(m.name, m.v, m.dst...); err != nil {
			return t, err
		}
	}
	if t.ContentW() < 72 || t.BodyH() < 144 {
		return t, errors.New("the margins leave too little room for text")
	}
	for _, f := range []struct {
		name string
		v    *string
		dst  *string
	}{
		{"body_font", p.BodyFont, &t.Body}, {"body_bold_font", p.BodyBoldFont, &t.BodyBold},
		{"heading_font", p.HeadingFont, &t.Heading}, {"code_font", p.CodeFont, &t.Code},
	} {
		if f.v != nil {
			if strings.TrimSpace(*f.v) == "" {
				return t, fmt.Errorf("%s is empty", f.name)
			}
			*f.dst = *f.v
		}
	}
	size := func(name string, v *float64, dst *float64) error {
		if v == nil {
			return nil
		}
		if !(*v >= 4 && *v <= 72) {
			return fmt.Errorf("%s %v: use 4 to 72 (points)", name, *v)
		}
		*dst = *v
		return nil
	}
	for _, s := range []struct {
		name string
		v    *float64
		dst  *float64
	}{
		{"body_size", p.BodySize, &t.BodySize}, {"code_size", p.CodeSize, &t.CodeSize},
		{"table_size", p.TableSize, &t.TableSize}, {"table_min_size", p.TableMinSize, &t.TableMinSize},
		{"note_size", p.NoteSize, &t.NoteSize}, {"title_size", p.TitleSize, &t.TitleSize},
	} {
		if err := size(s.name, s.v, s.dst); err != nil {
			return t, err
		}
	}
	if t.TableMinSize > t.TableSize {
		return t, fmt.Errorf("table_min_size %v is larger than table_size %v", t.TableMinSize, t.TableSize)
	}
	if p.HeadingSizes != nil {
		if len(p.HeadingSizes) != 3 {
			return t, errors.New("heading_sizes needs three sizes: levels 1, 2 and 3")
		}
		for i, v := range p.HeadingSizes {
			if err := size("heading_sizes", &v, &t.HeadingSizes[i]); err != nil {
				return t, err
			}
		}
	}
	if p.LineGap != nil {
		if !(*p.LineGap >= 0 && *p.LineGap <= 2) || math.IsNaN(*p.LineGap) {
			return t, fmt.Errorf("line_gap %v: use 0 to 2 (em)", *p.LineGap)
		}
		t.LineGap = *p.LineGap
	}
	for _, c := range []struct {
		name string
		v    *string
		dst  *theme.RGB
	}{
		{"text_color", p.TextColor, &t.Text}, {"muted_color", p.MutedColor, &t.TextMuted},
		{"link_color", p.LinkColor, &t.Link}, {"rule_color", p.RuleColor, &t.Rule},
		{"code_background", p.CodeBackground, &t.CodeBG},
	} {
		if c.v != nil {
			col, err := chart.Hex(*c.v)
			if err != nil {
				return t, fmt.Errorf("%s: %w", c.name, err)
			}
			*c.dst = theme.ToRGB(col)
		}
	}
	if p.ChartPalette != nil {
		if len(p.ChartPalette) == 0 {
			return t, errors.New("chart_palette is empty")
		}
		pal := make([]chart.Color, len(p.ChartPalette))
		for i, h := range p.ChartPalette {
			col, err := chart.Hex(h)
			if err != nil {
				return t, fmt.Errorf("chart_palette: %w", err)
			}
			pal[i] = col
		}
		t.Chart.Palette = pal
	}
	if p.Cover != nil {
		t.Cover = *p.Cover
	}
	if p.TOC != nil {
		t.TOC = *p.TOC
	}
	if p.TOCTitle != nil {
		if strings.TrimSpace(*p.TOCTitle) == "" {
			return t, errors.New("toc_title is empty")
		}
		t.TOCTitle = *p.TOCTitle
	}
	if p.Logo != nil {
		logo := *p.Logo
		if strings.HasPrefix(logo, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				logo = filepath.Join(home, logo[2:])
			}
		}
		if !filepath.IsAbs(logo) {
			return t, fmt.Errorf("logo %q: give an absolute path (or ~/...)", *p.Logo)
		}
		t.Logo = logo
	}
	return t, nil
}
