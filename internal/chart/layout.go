package chart

import (
	"fmt"
	"math"
)

// Layout lays a validated spec out at the given width in points. Height
// follows from the content: a fixed aspect for the plot, plus what the title,
// labels and legend need.
func Layout(s *Spec, width float64, m Measurer, st Style) *Figure {
	// A column narrower than this cannot hold a plot; the typesetters never
	// ask for less, and the floor keeps the arithmetic below positive.
	width = math.Max(width, 120)
	l := &layout{s: s, m: m, st: st, f: &Figure{W: width}}
	if s.Title != "" {
		lines := wrap(m, s.Title, st.TitleSize, true, width)
		for i, ln := range lines {
			l.add(Text{X: 0, Y: st.TitleSize*float64(i+1)*1.25 - st.TitleSize*0.25, S: ln, Size: st.TitleSize, Bold: true, Color: st.Text})
		}
		l.y = st.TitleSize*1.25*float64(len(lines)) + st.TitleSize*0.6
	}
	if s.Type == "bar" && s.Orientation == "horizontal" {
		l.horizontal()
	} else {
		l.xy()
	}
	l.legend()
	l.f.H = l.y + pad
	return l.f
}

type layout struct {
	s  *Spec
	m  Measurer
	st Style
	f  *Figure
	y  float64 // next free y
}

func (l *layout) add(it Item) { l.f.Items = append(l.f.Items, it) }

func (l *layout) warnf(format string, a ...any) {
	l.f.Warnings = append(l.f.Warnings, fmt.Sprintf(format, a...))
}

func (l *layout) color(i int) Color { return l.st.Palette[i%len(l.st.Palette)] }

// valueRange returns the value axis's ticks, decimals and domain.
func (l *layout) valueRange(ax Axis, lo, hi float64, includeZero bool) ([]float64, int, float64, float64) {
	if includeZero {
		lo, hi = math.Min(lo, 0), math.Max(hi, 0)
	}
	if ax.Min != nil {
		lo = *ax.Min
	}
	if ax.Max != nil {
		hi = *ax.Max
	}
	// One fixed end can pass the data's other end (all values null, say).
	if !(hi > lo) {
		switch {
		case ax.Max == nil:
			hi = lo + math.Max(math.Abs(lo), 1)
		case ax.Min == nil:
			lo = hi - math.Max(math.Abs(hi), 1)
		}
	}
	ts, dec := ticks(lo, hi, 5)
	dlo, dhi := ts[0], ts[len(ts)-1]
	if ax.Min != nil {
		dlo = *ax.Min
	}
	if ax.Max != nil {
		dhi = *ax.Max
	}
	var in []float64
	for _, t := range ts {
		if t >= dlo-1e-9*(dhi-dlo) && t <= dhi+1e-9*(dhi-dlo) {
			in = append(in, t)
		}
	}
	return in, dec, dlo, dhi
}

// dataRange is the extent of the values a bar or line chart plots (stack
// totals for a stacked bar chart).
func (l *layout) dataRange() (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	see := func(v float64) { lo, hi = math.Min(lo, v), math.Max(hi, v) }
	if l.s.Stacked {
		for c := range l.s.X.Categories {
			sum := 0.0
			for _, se := range l.s.Series {
				if v := se.Values[c]; v != nil {
					sum += *v
				}
			}
			see(sum)
		}
	} else {
		for _, se := range l.s.Series {
			for _, v := range se.Values {
				if v != nil {
					see(*v)
				}
			}
		}
	}
	if math.IsInf(lo, 1) { // every value null
		return 0, 0
	}
	return lo, hi
}

// xy lays out vertical bars, lines and scatters: value axis on the left.
func (l *layout) xy() {
	s, st, m := l.s, l.st, l.m
	fs := st.FontSize
	var lo, hi float64
	if s.Type == "scatter" {
		lo, hi = math.Inf(1), math.Inf(-1)
		for _, se := range s.Series {
			for _, p := range se.Points {
				lo, hi = math.Min(lo, p[1]), math.Max(hi, p[1])
			}
		}
	} else {
		lo, hi = l.dataRange()
	}
	yt, ydec, ylo, yhi := l.valueRange(s.Y, lo, hi, s.Type == "bar")
	ylabels := make([]string, len(yt))
	labW := 0.0
	for i, t := range yt {
		ylabels[i] = compact(t, ydec)
		labW = math.Max(labW, m.Width(ylabels[i], fs, false))
	}
	// The y label reads up the axis when it fits the plot's height; a longer
	// one sits above the plot, wrapped, rather than running off the figure.
	yVertical := s.Y.Label != "" && m.Width(s.Y.Label, fs, false) <= l.f.W*0.45
	if s.Y.Label != "" && !yVertical {
		l.textLines(wrap(m, s.Y.Label, fs, false, l.f.W), 0, Start, fs, st.TextMuted)
		l.y += fs * 0.3
	}
	left := labW + pad
	if yVertical {
		left += fs * 1.6
	}
	right := pad
	top := l.y + fs*0.6
	if s.Type == "scatter" {
		// A point on the domain's edge keeps its whole marker, and the last x
		// tick label is centred on the plot's right edge.
		edge := markerR + markerRing
		right = math.Max(edge, l.m.Width("-0.000M", fs, false)/2)
		top = l.y + math.Max(fs*0.6, edge)
		left = math.Max(left, edge)
	}
	pw := l.f.W - left - right
	ph := l.f.W * 0.45
	px := left
	yOf := func(v float64) float64 { return top + ph*(1-(v-ylo)/(yhi-ylo)) }

	// Grid and value ticks.
	for i, t := range yt {
		y := yOf(t)
		l.add(Line{X1: px, Y1: y, X2: px + pw, Y2: y, Width: hairline, Color: st.Grid})
		l.add(Text{X: px - pad, Y: y + fs*0.35, S: ylabels[i], Size: fs, Anchor: End, Color: st.TextMuted})
	}
	if yVertical {
		l.add(Text{X: fs, Y: top + ph/2, S: s.Y.Label, Size: fs, Anchor: Middle, Color: st.TextMuted, Vertical: true})
	}

	bottom := top + ph
	var xlabH float64
	if s.Type == "scatter" {
		xlo, xhi := math.Inf(1), math.Inf(-1)
		for _, se := range s.Series {
			for _, p := range se.Points {
				xlo, xhi = math.Min(xlo, p[0]), math.Max(xhi, p[0])
			}
		}
		xt, xdec, dlo, dhi := l.valueRange(s.X, xlo, xhi, false)
		xOf := func(v float64) float64 { return px + pw*(v-dlo)/(dhi-dlo) }
		l.numericAxis(xt, xdec, xOf, bottom, false)
		xlabH = fs * 1.9
		for si, se := range s.Series {
			for _, p := range se.Points {
				l.add(Circle{X: xOf(p[0]), Y: yOf(p[1]), R: markerR, Fill: l.color(si), Ring: markerRing, RingColor: st.Surface})
			}
		}
	} else {
		n := len(s.X.Categories)
		band := pw / float64(n)
		xlabH = l.categoryAxis(px, band, bottom)
		if s.Type == "bar" {
			l.verticalBars(px, band, yOf)
		} else {
			l.lines(px, band, yOf)
		}
	}
	// The baseline (zero for bars) over the grid.
	base := ylo
	if ylo <= 0 && yhi >= 0 {
		base = 0
	}
	l.add(Line{X1: px, Y1: yOf(base), X2: px + pw, Y2: yOf(base), Width: hairline * 1.5, Color: st.Axis})

	l.y = bottom + xlabH
	if s.X.Label != "" {
		l.y += fs * 0.4
		l.textLines(wrap(m, s.X.Label, fs, false, pw), px+pw/2, Middle, fs, st.TextMuted)
	}
}

// categoryAxis draws category labels under the plot: horizontal when they
// fit their band, else vertical; thinned only when even vertical labels
// would collide. It returns the height used.
func (l *layout) categoryAxis(px, band, bottom float64) float64 {
	st, fs := l.st, l.st.FontSize
	cats := l.s.X.Categories
	maxW := 0.0
	for _, c := range cats {
		maxW = math.Max(maxW, l.m.Width(c, fs, false))
	}
	if maxW <= band-2 {
		for i, c := range cats {
			l.add(Text{X: px + band*(float64(i)+0.5), Y: bottom + fs*1.5, S: c, Size: fs, Anchor: Middle, Color: st.TextMuted})
		}
		return fs * 1.9
	}
	step := 1
	if band <= 0 {
		return 0
	}
	if line := fs * 1.2; line > band {
		step = int(math.Ceil(line / band))
		l.warnf("x-axis labels: %d categories do not fit; every %d%s label is shown", len(cats), step, ordinal(step))
	}
	for i := 0; i < len(cats); i += step {
		l.add(Text{X: px + band*(float64(i)+0.5) + fs*0.35, Y: bottom + pad, S: cats[i], Size: fs, Anchor: End, Color: st.TextMuted, Vertical: true})
	}
	return maxW + pad*2
}

func ordinal(n int) string {
	switch n % 10 {
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// numericAxis draws value ticks along a horizontal axis at y, with vertical
// grid lines when grid is set. Labels that would collide are thinned.
func (l *layout) numericAxis(ts []float64, dec int, xOf func(float64) float64, y float64, grid bool) {
	st, fs := l.st, l.st.FontSize
	labels := make([]string, len(ts))
	maxW := 0.0
	for i, t := range ts {
		labels[i] = compact(t, dec)
		maxW = math.Max(maxW, l.m.Width(labels[i], fs, false))
	}
	step := 1
	if len(ts) > 1 {
		spacing := math.Abs(xOf(ts[1]) - xOf(ts[0]))
		for float64(step)*spacing < maxW+pad {
			step++
		}
	}
	for i, t := range ts {
		x := xOf(t)
		if grid {
			l.add(Line{X1: x, Y1: l.y, X2: x, Y2: y, Width: hairline, Color: st.Grid})
		}
		if i%step == 0 {
			// Below the lowest value tick's label, which sits on the axis line.
			l.add(Text{X: x, Y: y + fs*1.5, S: labels[i], Size: fs, Anchor: Middle, Color: st.TextMuted})
		}
	}
}

func (l *layout) verticalBars(px, band float64, yOf func(float64) float64) {
	s := l.s
	y0 := yOf(0)
	for c := range s.X.Categories {
		x0 := px + band*float64(c)
		if s.Stacked {
			w := math.Min(band*0.6, barMax)
			x := x0 + (band-w)/2
			l.stack(c, func(from, to float64, top bool, color Color) {
				ya, yb := yOf(from), yOf(to)
				h := ya - yb
				if !top {
					h -= gap
				}
				if h <= 0 {
					return
				}
				r := Rect{X: x, Y: yb, W: w, H: h, Fill: color}
				if top {
					r.Round, r.RoundSide = math.Min(barRound, math.Min(h, w/2)), "top"
				} else {
					r.Y += gap
				}
				l.add(r)
			})
			continue
		}
		ns := float64(len(s.Series))
		group := math.Min(band*0.75, ns*barMax+(ns-1)*gap)
		// In a narrow band the gaps shrink before the bars do: at most a fifth
		// of the group is gap, so a bar's width stays positive.
		g := math.Min(gap, group*0.2/ns)
		w := (group - (ns-1)*g) / ns
		for si, se := range s.Series {
			v := se.Values[c]
			if v == nil || *v == 0 {
				continue
			}
			x := x0 + (band-group)/2 + float64(si)*(w+g)
			yv := yOf(*v)
			r := Rect{X: x, W: w, Fill: l.color(si)}
			if *v > 0 {
				r.Y, r.H, r.RoundSide = yv, y0-yv, "top"
			} else {
				r.Y, r.H, r.RoundSide = y0, yv-y0, "bottom"
			}
			r.Round = math.Min(barRound, math.Min(r.H, w/2))
			l.add(r)
		}
	}
}

// stack walks category c's stacked segments from the baseline, calling seg
// with each segment's value range; top marks the outermost segment.
func (l *layout) stack(c int, seg func(from, to float64, top bool, color Color)) {
	last := -1
	for si, se := range l.s.Series {
		if v := se.Values[c]; v != nil && *v > 0 {
			last = si
		}
	}
	sum := 0.0
	for si, se := range l.s.Series {
		v := se.Values[c]
		if v == nil || *v <= 0 {
			continue
		}
		seg(sum, sum+*v, si == last, l.color(si))
		sum += *v
	}
}

func (l *layout) lines(px, band float64, yOf func(float64) float64) {
	s, st := l.s, l.st
	markers := len(s.X.Categories) <= 40
	for si, se := range s.Series {
		var run [][2]float64
		flush := func() {
			if len(run) > 1 {
				l.add(Polyline{Points: run, Width: lineWidth, Color: l.color(si)})
			}
			run = nil
		}
		for c, v := range se.Values {
			if v == nil {
				flush()
				continue
			}
			run = append(run, [2]float64{px + band*(float64(c)+0.5), yOf(*v)})
		}
		flush()
		for c, v := range se.Values {
			// A lone point between gaps has no line, so it always gets a marker.
			alone := v != nil && (c == 0 || se.Values[c-1] == nil) && (c == len(se.Values)-1 || se.Values[c+1] == nil)
			if v != nil && (markers || alone) {
				l.add(Circle{X: px + band*(float64(c)+0.5), Y: yOf(*v), R: markerR, Fill: l.color(si), Ring: markerRing, RingColor: st.Surface})
			}
		}
	}
}

// horizontal lays out a horizontal bar chart: categories down the left,
// values along the bottom.
func (l *layout) horizontal() {
	s, st, m := l.s, l.st, l.m
	fs := st.FontSize
	labCol := 0.0
	for _, c := range s.X.Categories {
		labCol = math.Max(labCol, m.Width(c, fs, false))
	}
	labCol = math.Min(labCol, l.f.W*0.35)
	wrapped := make([][]string, len(s.X.Categories))
	maxLines := 1
	for i, c := range s.X.Categories {
		wrapped[i] = wrap(m, c, fs, false, labCol)
		maxLines = max(maxLines, len(wrapped[i]))
	}
	ns := float64(len(s.Series))
	thick := barMax
	if !s.Stacked {
		thick = ns*barMax + (ns-1)*gap
	}
	band := math.Max(float64(maxLines)*fs*1.25, thick) + fs
	lo, hi := l.dataRange()
	xt, xdec, dlo, dhi := l.valueRange(s.Y, lo, hi, true)
	px := labCol + pad*2
	pw := l.f.W - px - pad*3 // room for the last tick label's half
	if s.X.Label != "" {
		l.textLines(wrap(m, s.X.Label, fs, false, l.f.W), 0, Start, fs, st.TextMuted)
		l.y += fs * 0.6
	}
	top := l.y
	ph := band * float64(len(s.X.Categories))
	xOf := func(v float64) float64 { return px + pw*(v-dlo)/(dhi-dlo) }
	l.y = top
	l.numericAxis(xt, xdec, xOf, top+ph, true)
	x0 := xOf(0)
	for c := range s.X.Categories {
		yc := top + band*float64(c)
		lines := wrapped[c]
		ty := yc + band/2 - float64(len(lines))*fs*1.25/2 + fs
		for i, ln := range lines {
			l.add(Text{X: px - pad*2, Y: ty + float64(i)*fs*1.25, S: ln, Size: fs, Anchor: End, Color: st.TextMuted})
		}
		if s.Stacked {
			y := yc + (band-barMax)/2
			l.stack(c, func(from, to float64, last bool, color Color) {
				xa, xb := xOf(from), xOf(to)
				w := xb - xa
				if !last {
					w -= gap
				}
				if w <= 0 {
					return
				}
				r := Rect{X: xa, Y: y, W: w, H: barMax, Fill: color}
				if last {
					r.Round, r.RoundSide = math.Min(barRound, math.Min(w, barMax/2)), "right"
				}
				l.add(r)
			})
			continue
		}
		for si, se := range s.Series {
			v := se.Values[c]
			if v == nil || *v == 0 {
				continue
			}
			y := yc + (band-thick)/2 + float64(si)*(barMax+gap)
			xv := xOf(*v)
			r := Rect{Y: y, H: barMax, Fill: l.color(si)}
			if *v > 0 {
				r.X, r.W, r.RoundSide = x0, xv-x0, "right"
			} else {
				r.X, r.W, r.RoundSide = xv, x0-xv, "left"
			}
			r.Round = math.Min(barRound, math.Min(r.W, barMax/2))
			l.add(r)
		}
	}
	l.add(Line{X1: x0, Y1: top, X2: x0, Y2: top + ph, Width: hairline * 1.5, Color: st.Axis})
	l.y = top + ph + fs*1.9
	if s.Y.Label != "" {
		l.y += fs * 0.4
		l.textLines(wrap(m, s.Y.Label, fs, false, pw), px+pw/2, Middle, fs, st.TextMuted)
	}
}

// legend draws one entry per series under the chart, wrapping to the width;
// a single series has none (the title names it).
func (l *layout) legend() {
	s, st := l.s, l.st
	if len(s.Series) < 2 {
		return
	}
	fs := st.FontSize
	l.y += fs * 0.5
	x, row := 0.0, l.y
	rowH := fs * 1.6
	for si, se := range s.Series {
		// A name wider than the figure wraps and takes a row of its own.
		names := wrap(l.m, se.Name, fs, false, l.f.W-fs-pad)
		w := fs + pad + l.m.Width(names[0], fs, false)
		if x > 0 && (x+w > l.f.W || len(names) > 1) {
			x, row, rowH = 0, row+rowH, fs*1.6
		}
		cy := row + fs*0.65
		if s.Type == "line" {
			l.add(Line{X1: x, Y1: cy, X2: x + fs, Y2: cy, Width: lineWidth, Color: l.color(si)})
		} else {
			l.add(Rect{X: x, Y: cy - fs*0.4, W: fs * 0.8, H: fs * 0.8, Fill: l.color(si)})
		}
		for i, n := range names {
			l.add(Text{X: x + fs + pad, Y: row + fs + float64(i)*fs*1.25, S: n, Size: fs, Color: st.Text})
		}
		if len(names) > 1 {
			rowH = fs*1.25*float64(len(names)) + fs*0.35
			x = l.f.W // the next entry starts a new row
			continue
		}
		x += w + fs*1.5
	}
	l.y = row + rowH
}

// textLines places wrapped lines of text from l.y down, anchored at x.
func (l *layout) textLines(ls []string, x float64, a Anchor, size float64, c Color) {
	for _, s := range ls {
		l.add(Text{X: x, Y: l.y + size, S: s, Size: size, Anchor: a, Color: c})
		l.y += size * 1.25
	}
}

// wrap breaks s into lines no wider than w, between characters (Japanese has
// no spaces to break at). A single character wider than w gets a line of its
// own rather than being lost.
func wrap(m Measurer, s string, size float64, bold bool, w float64) []string {
	if m.Width(s, size, bold) <= w {
		return []string{s}
	}
	var lines []string
	rs := []rune(s)
	start := 0
	for i := 1; i <= len(rs); i++ {
		if m.Width(string(rs[start:i]), size, bold) > w && i-1 > start {
			lines = append(lines, string(rs[start:i-1]))
			start = i - 1
		}
	}
	return append(lines, string(rs[start:]))
}
