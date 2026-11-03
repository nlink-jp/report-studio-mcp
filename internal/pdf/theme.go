package pdf

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

// Theme is everything a profile decides about the PDF.
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
}

func hexRGB(s string) RGB {
	c, err := chart.Hex(s)
	if err != nil {
		panic(err)
	}
	return toRGB(c)
}

func toRGB(c chart.Color) RGB {
	return RGB{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255}
}

const mm = 72 / 25.4

// StandardTheme is the built-in "standard" profile: A4, Hiragino Sans,
// 20 mm margins.
func StandardTheme() Theme {
	return Theme{
		PageW: 210 * mm, PageH: 297 * mm,
		MarginT: 20 * mm, MarginB: 20 * mm, MarginL: 20 * mm, MarginR: 20 * mm,
		Body: "HiraginoSans-W3", BodyBold: "HiraginoSans-W6", Heading: "HiraginoSans-W6", Code: "Menlo-Regular",
		BodySize: 10.5, CodeSize: 8.5, TableSize: 9, TableMinSize: 7, NoteSize: 8.5, MetaSize: 9,
		TitleSize: 20, SubtitleSize: 13,
		HeadingSizes: [3]float64{16, 13, 11.5},
		LineGap:      0.55,
		Text:         hexRGB("#0b0b0b"),
		TextMuted:    hexRGB("#52514e"),
		Link:         hexRGB("#1c5cab"),
		Rule:         hexRGB("#c3c2b7"),
		CodeBG:       hexRGB("#f3f2ef"),
		QuoteBar:     hexRGB("#c3c2b7"),
		TableHeadBG:  hexRGB("#f3f2ef"),
		Alerts: map[markdown.AlertKind]AlertLook{
			markdown.AlertNote:      {"Note", hexRGB("#2a78d6")},
			markdown.AlertTip:       {"Tip", hexRGB("#008300")},
			markdown.AlertImportant: {"Important", hexRGB("#6250d6")},
			markdown.AlertWarning:   {"Warning", hexRGB("#c98500")},
			markdown.AlertCaution:   {"Caution", hexRGB("#d03b3b")},
		},
		Chart: chart.DefaultStyle(),
	}
}

// contentW is the width between the side margins.
func (t Theme) contentW() float64 { return t.PageW - t.MarginL - t.MarginR }

// bodyH is the height between the top and bottom margins.
func (t Theme) bodyH() float64 { return t.PageH - t.MarginT - t.MarginB }
