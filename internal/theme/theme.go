// Package theme holds what a profile decides about the documents — page,
// fonts, sizes, colours — shared by the PDF typesetter and the .docx writer,
// and the built-in profiles.
package theme

import (
	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// RGB is a colour with 0..1 components.
type RGB = [3]float64

// AlertLook is how one alert kind is shown.
type AlertLook struct {
	Label string
	Color RGB
}

// Theme is everything a profile decides about the output.
type Theme struct {
	PageW, PageH                       float64
	MarginT, MarginB, MarginL, MarginR float64

	// PostScript names.
	Body, BodyBold, Heading, Code string

	BodySize, CodeSize, TableSize, TableMinSize, NoteSize, MetaSize float64
	TitleSize, SubtitleSize                                         float64
	HeadingSizes                                                    [3]float64 // levels 1..3; deeper levels look like 3
	// LineGap is the extra space between lines of body text, in em.
	LineGap float64

	Text, TextMuted, Link, Rule, CodeBG, QuoteBar, TableHeadBG RGB
	Alerts                                                     map[markdown.AlertKind]AlertLook

	Chart chart.Style

	// Cover puts a title page first; TOC a table of contents after it. The
	// manuscript's front matter (cover, toc) overrides both.
	Cover, TOC bool
	// Logo is an image file shown on the cover, named in the operator's
	// configuration (never by the manuscript).
	Logo string
	// TOCTitle heads the table of contents.
	TOCTitle string
	// BreakBefore starts a new page before every top-level heading of this
	// level or above (1 or 2); 0 never. The front matter's break_before
	// overrides it.
	BreakBefore int
}

// BreakLevel parses a break_before value: "h1", "h2" or "none".
func BreakLevel(s string) (int, bool) {
	switch s {
	case "none":
		return 0, true
	case "h1":
		return 1, true
	case "h2":
		return 2, true
	}
	return 0, false
}

// BreakName is BreakLevel's inverse.
func BreakName(level int) string {
	switch level {
	case 1:
		return "h1"
	case 2:
		return "h2"
	}
	return "none"
}

// HexRGB parses "#rrggbb"; it panics on a malformed literal, so use it for
// constants only.
func HexRGB(s string) RGB {
	c, err := chart.Hex(s)
	if err != nil {
		panic(err)
	}
	return ToRGB(c)
}

// ToRGB converts a chart colour.
func ToRGB(c chart.Color) RGB {
	return RGB{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255}
}

const mm = 72 / 25.4

// Standard is the built-in "standard" profile: A4, Hiragino Sans, 20 mm
// margins, no cover, no table of contents.
func Standard() Theme {
	return Theme{
		PageW: 210 * mm, PageH: 297 * mm,
		MarginT: 20 * mm, MarginB: 20 * mm, MarginL: 20 * mm, MarginR: 20 * mm,
		Body: "HiraginoSans-W3", BodyBold: "HiraginoSans-W6", Heading: "HiraginoSans-W6", Code: "Menlo-Regular",
		BodySize: 10.5, CodeSize: 8.5, TableSize: 9, TableMinSize: 7, NoteSize: 8.5, MetaSize: 9,
		TitleSize: 20, SubtitleSize: 13,
		HeadingSizes: [3]float64{16, 13, 11.5},
		LineGap:      0.55,
		Text:         HexRGB("#0b0b0b"),
		TextMuted:    HexRGB("#52514e"),
		Link:         HexRGB("#1c5cab"),
		Rule:         HexRGB("#c3c2b7"),
		CodeBG:       HexRGB("#f3f2ef"),
		QuoteBar:     HexRGB("#c3c2b7"),
		TableHeadBG:  HexRGB("#f3f2ef"),
		Alerts: map[markdown.AlertKind]AlertLook{
			markdown.AlertNote:      {"Note", HexRGB("#2a78d6")},
			markdown.AlertTip:       {"Tip", HexRGB("#008300")},
			markdown.AlertImportant: {"Important", HexRGB("#6250d6")},
			markdown.AlertWarning:   {"Warning", HexRGB("#c98500")},
			markdown.AlertCaution:   {"Caution", HexRGB("#d03b3b")},
		},
		Chart:    chart.DefaultStyle(),
		TOCTitle: "Contents",
	}
}

// Formal is the built-in "formal" profile: a cover and a table of contents,
// Hiragino Mincho for the body, wider margins, chapters on a new page.
func Formal() Theme {
	t := Standard()
	t.Body, t.BodyBold = "HiraMinProN-W3", "HiraMinProN-W6"
	t.MarginT, t.MarginB, t.MarginL, t.MarginR = 25*mm, 25*mm, 25*mm, 25*mm
	t.Cover, t.TOC = true, true
	// A document with a cover and contents is opened chapter by chapter
	// (RFP Discussion Log 16).
	t.BreakBefore = 2
	return t
}

// Compact is the built-in "compact" profile: narrow margins and smaller type,
// for drafts and notes.
func Compact() Theme {
	t := Standard()
	t.MarginT, t.MarginB, t.MarginL, t.MarginR = 12*mm, 12*mm, 12*mm, 12*mm
	t.BodySize, t.CodeSize, t.TableSize, t.TableMinSize, t.NoteSize, t.MetaSize = 9, 7.5, 8, 6.5, 7.5, 8
	t.TitleSize, t.SubtitleSize = 16, 11
	t.HeadingSizes = [3]float64{13, 11, 10}
	t.LineGap = 0.4
	t.Chart.FontSize, t.Chart.TitleSize = 7, 8.5
	return t
}

// Builtin returns the built-in profile of that name.
func Builtin(name string) (Theme, bool) {
	switch name {
	case "standard":
		return Standard(), true
	case "formal":
		return Formal(), true
	case "compact":
		return Compact(), true
	}
	return Theme{}, false
}

// BuiltinNames lists the built-in profiles.
func BuiltinNames() []string { return []string{"standard", "formal", "compact"} }

// ContentW is the width between the side margins.
func (t Theme) ContentW() float64 { return t.PageW - t.MarginL - t.MarginR }

// BodyH is the height between the top and bottom margins.
func (t Theme) BodyH() float64 { return t.PageH - t.MarginT - t.MarginB }
