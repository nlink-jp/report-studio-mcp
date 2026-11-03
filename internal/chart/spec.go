// Package chart turns a ```chart block — JSON naming a chart type and its
// data — into a figure: a list of shapes and text in points, independent of
// the output. The PDF typesetter draws the list as vectors; the .docx writer
// rasterises the same list, so a chart is drawn once and emitted twice.
//
// The model writes data, never drawing code. A spec that cannot be drawn
// faithfully — unknown fields, mismatched lengths, a value outside a fixed axis,
// more series than colours tell apart — is an error, because the figure would
// be wrong; anything merely crowded is drawn and warned about.
//
// The marks follow the organisation's data-visualisation method (thin bars
// with a rounded data end and a surface gap, 1.5 pt lines, ringed markers,
// hairline grids, one y axis, a legend for two or more series, text in text
// colours) and its validated categorical palette, scaled for print.
package chart

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Spec is the JSON a ```chart block holds.
type Spec struct {
	// Type is "bar", "line" or "scatter".
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	// Orientation is "vertical" (default) or "horizontal"; bar charts only.
	Orientation string `json:"orientation,omitempty"`
	// Stacked stacks a bar chart's series instead of grouping them.
	Stacked bool     `json:"stacked,omitempty"`
	X       Axis     `json:"x"`
	Y       Axis     `json:"y"`
	Series  []Series `json:"series"`
}

// Axis describes one axis. Categories label a bar or line chart's x axis;
// Min and Max fix a numeric axis's range.
type Axis struct {
	Label      string   `json:"label,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
}

// Series is one data series. Bar and line charts use Values, one per
// category (null leaves a gap); scatter charts use Points, [x, y] pairs.
type Series struct {
	Name   string       `json:"name,omitempty"`
	Values []*float64   `json:"values,omitempty"`
	Points [][2]float64 `json:"points,omitempty"`
}

// Error is a spec that cannot be drawn. Line is 1-based within the block's
// content when the problem has a position (a JSON syntax error), else 0.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("chart, line %d of the block: %s", e.Line, e.Msg)
	}
	return "chart: " + e.Msg
}

func errf(format string, a ...any) error { return &Error{Msg: fmt.Sprintf(format, a...)} }

// Limits bound one chart.
type Limits struct {
	// MaxPoints bounds the data points of one chart (values plus points).
	MaxPoints int
	// MaxSeries is the number of categorical colours available; scatter
	// charts are further capped at three (see validate).
	MaxSeries int
}

// Parse decodes and validates a block's content strictly: unknown fields,
// trailing data and every inconsistency are errors. It returns the number of
// data points, which the caller counts against the document's limit.
func Parse(src []byte, lim Limits) (*Spec, int, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, 0, jsonErr(src, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, 0, &Error{Msg: "unexpected data after the JSON object"}
	}
	n, err := validate(&s, lim)
	if err != nil {
		return nil, 0, err
	}
	return &s, n, nil
}

// jsonErr turns a decoding error into one with the line it happened on.
func jsonErr(src []byte, err error) error {
	var off int64 = -1
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		off = se.Offset
	case errors.As(err, &te):
		off = te.Offset
	}
	msg := strings.TrimPrefix(err.Error(), "json: ")
	if off < 0 || off > int64(len(src)) {
		return &Error{Msg: msg}
	}
	return &Error{Line: 1 + bytes.Count(src[:off], []byte("\n")), Msg: msg}
}

// Text and magnitude bounds. Labels longer than these are mistakes rather
// than labels, and values beyond maxMagnitude cannot be stepped through by
// floating-point ticks (an epoch in nanoseconds belongs in seconds).
const (
	maxLabelRunes = 200
	maxTitleRunes = 300
	maxMagnitude  = 1e15
)

// scatterSeriesCap is how many categorical colours stay distinguishable when
// every pair of series can sit side by side, as points in a scatter do: the
// palette validates all-pairs for its first three slots only.
const scatterSeriesCap = 3

func validate(s *Spec, lim Limits) (int, error) {
	switch s.Type {
	case "bar", "line", "scatter":
	case "":
		return 0, errf(`"type" is required: "bar", "line" or "scatter"`)
	default:
		return 0, errf(`unknown type %q: use "bar", "line" or "scatter"`, s.Type)
	}
	if s.Type != "bar" {
		if s.Orientation != "" {
			return 0, errf(`"orientation" applies to bar charts only`)
		}
		if s.Stacked {
			return 0, errf(`"stacked" applies to bar charts only`)
		}
	}
	switch s.Orientation {
	case "", "vertical", "horizontal":
	default:
		return 0, errf(`unknown orientation %q: use "vertical" or "horizontal"`, s.Orientation)
	}
	if len(s.Series) == 0 {
		return 0, errf(`"series" needs at least one series`)
	}
	long := func(what, v string, max int) error {
		if n := len([]rune(v)); n > max {
			return errf("%s is %d characters; at most %d fit a chart", what, n, max)
		}
		return nil
	}
	checks := []error{long("the title", s.Title, maxTitleRunes), long("x.label", s.X.Label, maxLabelRunes), long("y.label", s.Y.Label, maxLabelRunes)}
	for i, c := range s.X.Categories {
		checks = append(checks, long(fmt.Sprintf("category %d", i+1), c, maxLabelRunes))
	}
	for i, se := range s.Series {
		checks = append(checks, long(fmt.Sprintf("the name of series %d", i+1), se.Name, maxLabelRunes))
	}
	for _, err := range checks {
		if err != nil {
			return 0, err
		}
	}
	maxSeries := lim.MaxSeries
	if s.Type == "scatter" && maxSeries > scatterSeriesCap {
		maxSeries = scatterSeriesCap
	}
	if len(s.Series) > maxSeries {
		return 0, errf("%d series is more than the %d whose colours stay distinguishable in a %s chart: split it into several charts",
			len(s.Series), maxSeries, s.Type)
	}
	names := map[string]bool{}
	for i, se := range s.Series {
		if len(s.Series) > 1 {
			if strings.TrimSpace(se.Name) == "" {
				return 0, errf("series %d has no name: with two or more series, each needs one for the legend", i+1)
			}
			if names[se.Name] {
				return 0, errf("series name %q is used twice", se.Name)
			}
			names[se.Name] = true
		}
	}
	for _, a := range []struct {
		name string
		ax   Axis
	}{{"x", s.X}, {"y", s.Y}} {
		if a.ax.Min != nil && a.ax.Max != nil && !(*a.ax.Min < *a.ax.Max) {
			return 0, errf("%s.min must be less than %s.max", a.name, a.name)
		}
		for _, v := range []*float64{a.ax.Min, a.ax.Max} {
			if v != nil && math.Abs(*v) > maxMagnitude {
				return 0, errf("%s.min/max %v is beyond ±%g: scale the data (e.g. thousands, seconds)", a.name, *v, maxMagnitude)
			}
		}
	}

	points := 0
	if s.Type == "scatter" {
		if len(s.X.Categories) > 0 {
			return 0, errf(`a scatter chart's x axis is numeric: remove "x.categories"`)
		}
		for i, se := range s.Series {
			if len(se.Values) > 0 {
				return 0, errf(`series %d: a scatter chart takes "points" ([x, y] pairs), not "values"`, i+1)
			}
			if len(se.Points) == 0 {
				return 0, errf(`series %d has no "points"`, i+1)
			}
			for _, p := range se.Points {
				if err := inRange("x", p[0], s.X); err != nil {
					return 0, err
				}
				if err := inRange("y", p[1], s.Y); err != nil {
					return 0, err
				}
			}
			points += len(se.Points)
		}
	} else {
		if len(s.X.Categories) == 0 {
			return 0, errf(`"x.categories" is required for a %s chart`, s.Type)
		}
		if s.X.Min != nil || s.X.Max != nil {
			return 0, errf(`a %s chart's x axis is categories: remove "x.min" and "x.max"`, s.Type)
		}
		for i, se := range s.Series {
			if len(se.Points) > 0 {
				return 0, errf(`series %d: a %s chart takes "values", not "points"`, i+1, s.Type)
			}
			if len(se.Values) != len(s.X.Categories) {
				return 0, errf("series %d has %d values for %d categories", i+1, len(se.Values), len(s.X.Categories))
			}
			for _, v := range se.Values {
				if v == nil {
					continue
				}
				if s.Stacked && *v < 0 {
					return 0, errf("a stacked bar chart cannot show the negative value %v: use grouped bars", *v)
				}
				if err := inRange("y", *v, s.Y); err != nil {
					return 0, err
				}
			}
			points += len(se.Values)
		}
		if s.Stacked {
			// Stack totals must fit a fixed axis too.
			for c := range s.X.Categories {
				sum := 0.0
				for _, se := range s.Series {
					if se.Values[c] != nil {
						sum += *se.Values[c]
					}
				}
				if err := inRange("y", sum, s.Y); err != nil {
					return 0, err
				}
			}
		}
		if s.Type == "bar" && s.Y.Min != nil && *s.Y.Min > 0 {
			return 0, errf("a bar chart's axis must include 0 (bars grow from it): remove y.min or set it to 0 or below")
		}
		if s.Type == "bar" && s.Y.Max != nil && *s.Y.Max < 0 {
			return 0, errf("a bar chart's axis must include 0 (bars grow from it): set y.max to 0 or above")
		}
	}
	if points > lim.MaxPoints {
		return 0, errf("%d data points is over the limit of %d for one chart", points, lim.MaxPoints)
	}
	return points, nil
}

func inRange(axis string, v float64, a Axis) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return errf("%s value %v is not a finite number", axis, v)
	}
	if math.Abs(v) > maxMagnitude {
		return errf("%s value %v is beyond ±%g: scale the data (e.g. thousands, seconds)", axis, v, maxMagnitude)
	}
	if a.Min != nil && v < *a.Min {
		return errf("%s value %v is below %s.min %v: it would be drawn off the chart", axis, v, axis, *a.Min)
	}
	if a.Max != nil && v > *a.Max {
		return errf("%s value %v is above %s.max %v: it would be drawn off the chart", axis, v, axis, *a.Max)
	}
	return nil
}
