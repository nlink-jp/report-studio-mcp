package chart

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Color is an sRGB colour.
type Color struct{ R, G, B uint8 }

// Hex parses "#rrggbb".
func Hex(s string) (Color, error) {
	if len(s) != 7 || s[0] != '#' {
		return Color{}, fmt.Errorf("colour %q is not #rrggbb", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("colour %q is not #rrggbb", s)
	}
	return Color{uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}

func mustHex(s string) Color {
	c, err := Hex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// Style is what a profile decides about charts.
type Style struct {
	Palette   []Color // categorical, fixed order
	Surface   Color
	Text      Color // primary
	TextMuted Color // axis labels and ticks
	Grid      Color
	Axis      Color
	FontSize  float64 // tick and legend text, points
	TitleSize float64
}

// DefaultStyle is the reference palette of the organisation's
// data-visualisation method (light mode; the categorical order is the
// colour-blind-safety mechanism, so it is kept as is).
func DefaultStyle() Style {
	pal := []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#6250d6", "#e34948"}
	s := Style{
		Surface:   mustHex("#ffffff"),
		Text:      mustHex("#0b0b0b"),
		TextMuted: mustHex("#52514e"),
		Grid:      mustHex("#e1e0d9"),
		Axis:      mustHex("#898781"),
		FontSize:  8,
		TitleSize: 9.5,
	}
	for _, h := range pal {
		s.Palette = append(s.Palette, mustHex(h))
	}
	return s
}

// Measurer measures text in points. The PDF side measures with Core Text;
// tests use a fixed advance.
type Measurer interface {
	Width(s string, size float64, bold bool) float64
}

// Anchor is a text's horizontal anchor.
type Anchor int

// Text anchors.
const (
	Start Anchor = iota
	Middle
	End
)

// Item is one thing to draw. Coordinates are points, origin at the figure's
// top-left, y growing downward.
type Item interface{ item() }

// Rect is a filled rectangle. Round, when positive, rounds the two corners of
// the side named by RoundSide ("top", "bottom", "left", "right") — a bar's
// data end; the baseline side stays square.
type Rect struct {
	X, Y, W, H float64
	Fill       Color
	Round      float64
	RoundSide  string
}

// Line is a straight stroke.
type Line struct {
	X1, Y1, X2, Y2 float64
	Width          float64
	Color          Color
}

// Polyline is a joined stroke with round joins and caps.
type Polyline struct {
	Points [][2]float64
	Width  float64
	Color  Color
}

// Circle is a filled circle with an optional ring in another colour.
type Circle struct {
	X, Y, R   float64
	Fill      Color
	Ring      float64
	RingColor Color
}

// Text is one line of text; Y is the baseline. Vertical text reads bottom to
// top, rotated about (X, Y).
type Text struct {
	X, Y     float64
	S        string
	Size     float64
	Bold     bool
	Anchor   Anchor
	Color    Color
	Vertical bool
}

func (Rect) item()     {}
func (Line) item()     {}
func (Polyline) item() {}
func (Circle) item()   {}
func (Text) item()     {}

// Figure is a laid-out chart.
type Figure struct {
	W, H     float64
	Items    []Item
	Warnings []string
}

// Marks, in points (the method's pixels scaled for print).
const (
	barMax     = 18.0 // a bar never fills its slot
	barRound   = 3.0
	gap        = 1.5 // surface gap between touching fills
	lineWidth  = 1.5
	markerR    = 3.0
	markerRing = 1.5
	hairline   = 0.5
	pad        = 4.0
)

// ticks returns about n "nice" values (1, 2, 5 × 10^k steps) covering
// [lo, hi], and the decimals they need.
func ticks(lo, hi float64, n int) ([]float64, int) {
	if hi == lo {
		if hi == 0 {
			hi = 1
		} else {
			lo, hi = lo-math.Abs(lo)*0.5, hi+math.Abs(hi)*0.5
		}
	}
	raw := (hi - lo) / float64(n)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	step := mag
	for _, m := range []float64{1, 2, 5, 10} {
		if m*mag >= raw {
			step = m * mag
			break
		}
	}
	start := math.Floor(lo/step) * step
	end := math.Ceil(hi/step) * step
	var out []float64
	for v := start; v <= end+step*1e-9; v += step {
		out = append(out, math.Round(v/step)*step)
	}
	dec := 0
	if step < 1 {
		dec = int(math.Ceil(-math.Log10(step)))
	}
	return out, dec
}

// compact formats a tick: 1,000 and up as K, M, B.
func compact(v float64, dec int) string {
	a := math.Abs(v)
	for _, u := range []struct {
		div float64
		suf string
	}{{1e9, "B"}, {1e6, "M"}, {1e3, "K"}} {
		if a >= u.div {
			s := strconv.FormatFloat(v/u.div, 'f', 2, 64)
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
			return s + u.suf
		}
	}
	return strconv.FormatFloat(v, 'f', dec, 64)
}
