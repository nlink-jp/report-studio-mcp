package pdf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// Picture is a raster ready to place: premultiplied RGBA, top row first, and
// the width in points it should be shown at (before fitting the column).
type Picture struct {
	Pix    []byte
	PW, PH int
	DispW  float64
	// Orig is the source PNG when the picture came from one, so the .docx
	// can embed it as it is instead of encoding the pixels again.
	Orig []byte
}

// Assets resolves what a manuscript refers to. An image that cannot be had
// stops the document; a mermaid diagram that cannot be drawn falls back to
// its source, so Mermaid's error is only ever a reason to warn with.
type Assets interface {
	Image(dest string, line int) (Picture, error)
	Mermaid(src string, line int) (Picture, error)
}

// Error stops a document: its output would be wrong.
type Error struct {
	Line    int
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// Options configure a layout.
type Options struct {
	Theme          Theme
	Assets         Assets
	MaxPages       int
	Chart          chart.Limits
	MaxChartPoints int // for the whole document
	// Cover puts a title page first; TOC a table of contents after it.
	Cover, TOC bool
	// Logo goes on the cover when non-nil.
	Logo *Picture
	// Deadline, when set, stops a layout still running at that time: a
	// manuscript within every limit must not hold the server past the
	// client's timeout.
	Deadline time.Time
}

// Result is a laid-out document.
type Result struct {
	Pages       []*Page
	Warnings    []markdown.Warning
	ChartPoints int
	// Headings are the document's headings of levels 1-3 with the page
	// (0-based, in Pages) each starts on.
	Headings []HeadingRef
}

// HeadingRef is one heading as the table of contents lists it.
type HeadingRef struct {
	Level int
	Text  string
	Page  int
	Dest  string // the named destination placed on it
}

// Page holds what to draw on one page, in drawing order. A cover has no page
// furniture (number, classification).
type Page struct {
	Ops   []Op
	Cover bool
}

// Op is one drawing operation; Bounds is the box it can mark.
type Op interface {
	Bounds() (x0, y0, x1, y1 float64)
}

// TextOp draws one line; Y is the baseline.
type TextOp struct {
	Spans              []Span
	X, Y, W, Asc, Desc float64
}

// RectOp fills a rectangle.
type RectOp struct {
	X, Y, W, H float64
	Color      RGB
	Radius     float64
	Side       RoundSide
}

// LineOp strokes a line.
type LineOp struct {
	X1, Y1, X2, Y2, Width float64
	Color                 RGB
}

// PicOp draws a picture into a rectangle.
type PicOp struct {
	Pic        Picture
	X, Y, W, H float64
}

// FigOp draws a chart figure at (X, Y) scaled by Scale.
type FigOp struct {
	Fig         *chart.Figure
	X, Y, Scale float64
}

// LinkOp makes a rectangle a link: to URL, or to the named destination Dest.
type LinkOp struct {
	X, Y, W, H float64
	URL        string
	Dest       string
}

// DestOp names a point as a link destination.
type DestOp struct {
	Name string
	X, Y float64
}

func (o TextOp) Bounds() (float64, float64, float64, float64) {
	return o.X, o.Y - o.Asc, o.X + o.W, o.Y + o.Desc
}
func (o RectOp) Bounds() (float64, float64, float64, float64) { return o.X, o.Y, o.X + o.W, o.Y + o.H }
func (o LineOp) Bounds() (float64, float64, float64, float64) {
	return math.Min(o.X1, o.X2), math.Min(o.Y1, o.Y2), math.Max(o.X1, o.X2), math.Max(o.Y1, o.Y2)
}
func (o PicOp) Bounds() (float64, float64, float64, float64) { return o.X, o.Y, o.X + o.W, o.Y + o.H }
func (o FigOp) Bounds() (float64, float64, float64, float64) {
	return o.X, o.Y, o.X + o.Fig.W*o.Scale, o.Y + o.Fig.H*o.Scale
}
func (o LinkOp) Bounds() (float64, float64, float64, float64) { return o.X, o.Y, o.X + o.W, o.Y + o.H }
func (o DestOp) Bounds() (float64, float64, float64, float64) { return o.X, o.Y, o.X, o.Y }

// frame is a bar or a background that runs beside or behind blocks and may
// span pages; each page gets its own segment.
type frame struct {
	bg    bool // background (inserted behind) rather than a left bar
	x, w  float64
	color RGB
	pad   float64
	y0    float64 // segment start on the current page
	at    int     // op index the background is inserted at
}

type marker struct {
	spans []Span
	x     float64
	size  float64
}

// stop carries an Error out of the layout through panic, so the many places
// that can stop need no error plumbing; Layout recovers it.
type stop struct{ err *Error }

type layouter struct {
	doc    *Doc
	o      Options
	th     Theme
	pages  []*Page
	y      float64
	x, w   float64
	frames []*frame
	mark   *marker
	hooks  []func() // run at the top of each new page (table headers)
	warns  []markdown.Warning
	line   int
	points int
	depth  int // list nesting, for the bullet shape
	heads  []HeadingRef
	base   float64 // baseline of the last line text placed
}

// Layout lays a parsed manuscript out into pages.
func Layout(md *markdown.Document, doc *Doc, o Options) (res *Result, err error) {
	l := &layouter{doc: doc, o: o, th: o.Theme}
	l.x, l.w = l.th.MarginL, l.th.ContentW()
	defer func() {
		if r := recover(); r != nil {
			s, ok := r.(stop)
			if !ok {
				panic(r)
			}
			res, err = nil, s.err
		}
	}()
	l.warns = append(l.warns, md.Warnings...)
	l.newPage()
	if !o.Cover {
		l.titleBlock(md.Meta)
	}
	body := textStyle{font: l.th.Body, bold: l.th.BodyBold, size: l.th.BodySize, color: l.th.Text}
	l.blocks(md.Children, body, false)
	l.footnotes(md.Footnotes)
	l.closeFrames()
	res = &Result{Pages: l.pages, Warnings: l.warns, ChartPoints: l.points, Headings: l.heads}
	var front []*Page
	if o.Cover {
		front = append(front, l.cover(md))
	}
	if o.TOC && len(l.heads) > 0 {
		// The table of contents lists page numbers that depend on its own
		// length: lay it out with placeholders to learn that, then for real.
		n := len(l.toc(l.heads, len(front), nil))
		toc := l.toc(l.heads, len(front), &n)
		for len(toc) != n {
			n = len(toc)
			toc = l.toc(l.heads, len(front), &n)
		}
		front = append(front, toc...)
	}
	if total := len(front) + len(res.Pages); total > o.MaxPages {
		return nil, &Error{Code: "limit_exceeded", Message: fmt.Sprintf("the document would be %d pages; the limit is %d", total, o.MaxPages),
			Details: map[string]any{"limit": "pages", "max": o.MaxPages}}
	}
	if len(front) > 0 {
		res.Pages = append(front, res.Pages...)
		for i := range res.Headings {
			res.Headings[i].Page += len(front)
		}
	}
	return res, nil
}

func (l *layouter) fail(line int, code, msg string, details map[string]any) {
	panic(stop{&Error{Line: line, Code: code, Message: msg, Details: details}})
}

func (l *layouter) warn(code, msg string) {
	l.warns = append(l.warns, markdown.Warning{Line: l.line, Code: code, Message: msg})
}

func (l *layouter) page() *Page { return l.pages[len(l.pages)-1] }

func (l *layouter) add(op Op) { l.page().Ops = append(l.page().Ops, op) }

func (l *layouter) bottom() float64 { return l.th.PageH - l.th.MarginB }

// top is where content starts on a fresh page, inside the open frames.
func (l *layouter) top() float64 {
	y := l.th.MarginT
	for _, f := range l.frames {
		y += f.pad
	}
	return y
}

func (l *layouter) atTop() bool { return l.y <= l.top()+0.01 }

func (l *layouter) newPage() {
	if len(l.pages) > 0 {
		l.closeSegments(l.bottom())
	}
	if len(l.pages) >= l.o.MaxPages {
		l.fail(l.line, "limit_exceeded", fmt.Sprintf("the document would be longer than %d pages", l.o.MaxPages),
			map[string]any{"limit": "pages", "max": l.o.MaxPages})
	}
	l.pages = append(l.pages, &Page{})
	l.y = l.th.MarginT
	for _, f := range l.frames {
		f.y0, f.at = l.y, len(l.page().Ops)
		l.y += f.pad
	}
	for _, h := range l.hooks {
		h()
	}
}

// ensure starts a new page unless h more points fit on this one. At the top
// of a page nothing more can be gained, so it never breaks there.
func (l *layouter) ensure(h float64) {
	if l.y+h > l.bottom()+0.01 && !l.atTop() {
		l.newPage()
	}
}

func (l *layouter) space(h float64) {
	if !l.atTop() {
		l.y = math.Min(l.y+h, l.bottom())
	}
}

// closeSegments ends every open frame's segment on the current page at y.
func (l *layouter) closeSegments(y float64) {
	for i := len(l.frames) - 1; i >= 0; i-- {
		l.segment(l.frames[i], y)
	}
}

func (l *layouter) segment(f *frame, y float64) {
	if y <= f.y0 {
		return
	}
	if f.bg {
		ops := l.page().Ops
		r := RectOp{X: f.x, Y: f.y0, W: f.w, H: y - f.y0, Color: f.color}
		ops = append(ops[:f.at], append([]Op{r}, ops[f.at:]...)...)
		l.page().Ops = ops
		for _, g := range l.frames { // later backgrounds now sit one op further on
			if g != f && g.bg && g.at >= f.at {
				g.at++
			}
		}
	} else {
		l.add(RectOp{X: f.x, Y: f.y0, W: f.w, H: y - f.y0, Color: f.color})
	}
}

func (l *layouter) openFrame(f *frame) {
	f.y0, f.at = l.y, len(l.page().Ops)
	l.frames = append(l.frames, f)
	l.y += f.pad
}

func (l *layouter) closeFrame() {
	f := l.frames[len(l.frames)-1]
	l.y = math.Min(l.y+f.pad, l.bottom())
	l.segment(f, l.y)
	l.frames = l.frames[:len(l.frames)-1]
}

// closeFrames ends whatever is still open at the end of the document.
func (l *layouter) closeFrames() {
	for len(l.frames) > 0 {
		l.closeFrame()
	}
}

// spans registers the styles of rich text.
func (l *layouter) spans(rs []rich) []Span {
	out := make([]Span, len(rs))
	for i, r := range rs {
		id, err := l.doc.Add(r.style)
		if err != nil {
			l.fail(0, "font_missing", err.Error(), map[string]any{"font": r.style.Font})
		}
		out[i] = Span{Text: r.text, Style: id}
	}
	return out
}

func (l *layouter) style(s Style) int {
	id, err := l.doc.Add(s)
	if err != nil {
		l.fail(0, "font_missing", err.Error(), map[string]any{"font": s.Font})
	}
	return id
}

func tsStyle(ts textStyle, bold bool) Style {
	f := ts.font
	if bold {
		f = ts.bold
	}
	return Style{Font: f, Size: ts.size, R: ts.color[0], G: ts.color[1], B: ts.color[2]}
}

// text places rich text as lines in the current column, breaking pages
// between lines. align is 0 left, 1 centre, 2 right.
func (l *layouter) text(rs []rich, size float64, gapEm float64, align int) {
	if len(rs) == 0 {
		return
	}
	spans := l.spans(rs)
	lines := l.doc.Typeset(spans, l.w)
	gap := size * gapEm
	decos := decorations(rs)
	for _, ln := range lines {
		h := ln.Ascent + ln.Descent
		l.ensure(h)
		base := l.y + ln.Ascent
		x := l.x
		switch align {
		case 1:
			x += (l.w - ln.Width) / 2
		case 2:
			x += l.w - ln.Width
		}
		decos = l.decorate(decos, ln, x, base)
		if len(ln.Spans) > 0 {
			l.add(TextOp{Spans: ln.Spans, X: x, Y: base, W: ln.Width, Asc: ln.Ascent, Desc: ln.Descent})
		}
		l.drawMarker(base)
		l.base = base
		l.y += h + gap
	}
	l.y -= gap
}

// deco is a piece of rich text that needs drawing beyond its style, with
// the runes [start, end) it covers.
type deco struct {
	r          rich
	start, end int
}

// decorations lists, in order, the pieces of rs that decorate.
func decorations(rs []rich) []deco {
	var out []deco
	at := 0
	for _, r := range rs {
		n := utf8.RuneCountInString(r.text)
		if r.code || r.strike || r.url != "" {
			out = append(out, deco{r, at, at + n})
		}
		at += n
	}
	return out
}

// decorate adds what styles cannot carry for one line: code backgrounds
// (before the text, so behind it), strike lines and link rectangles. Lines
// come in order, so the pieces wholly before this line are dropped and the
// rest returned for the next: linear in the paragraph, not quadratic.
func (l *layouter) decorate(ds []deco, ln Line, x, base float64) []deco {
	for len(ds) > 0 && ds[0].end <= ln.Start {
		ds = ds[1:]
	}
	for _, d := range ds {
		if d.start >= ln.End {
			break
		}
		r := d.r
		from, to := max(d.start, ln.Start), min(d.end, ln.End)
		if from >= to {
			continue
		}
		x0 := x + l.doc.OffsetAt(ln.Spans, from-ln.Start)
		x1 := x + l.doc.OffsetAt(ln.Spans, to-ln.Start)
		x1 = math.Min(x1, x+ln.Width)
		if x1 <= x0 {
			continue
		}
		size := r.style.Size
		if r.code {
			l.add(RectOp{X: math.Max(x0-1, l.x), Y: base - size*0.95, W: math.Min(x1-x0+2, l.x+l.w-math.Max(x0-1, l.x)), H: size * 1.25, Color: l.th.CodeBG})
		}
		if r.strike {
			l.add(LineOp{X1: x0, Y1: base - size*0.32, X2: x1, Y2: base - size*0.32, Width: size * 0.07, Color: RGB{r.style.R, r.style.G, r.style.B}})
		}
		if r.url != "" {
			l.add(LinkOp{X: x0, Y: base - ln.Ascent, W: x1 - x0, H: ln.Ascent + ln.Descent, URL: r.url})
		}
	}
	return ds
}

// drawMarker places a pending list marker beside the line whose baseline is
// base.
func (l *layouter) drawMarker(base float64) {
	if l.mark == nil {
		return
	}
	m := l.mark
	l.mark = nil
	w := 0.0
	for _, s := range m.spans {
		w += l.doc.Measure(s.Text, s.Style)
	}
	l.add(TextOp{Spans: m.spans, X: m.x - w, Y: base, W: w, Asc: m.size * 0.88, Desc: m.size * 0.12})
}

// flushMarker places a pending marker before a block that is not text.
func (l *layouter) flushMarker() {
	if l.mark != nil {
		l.ensure(l.mark.size * 1.2)
		l.drawMarker(l.y + l.mark.size*0.88)
	}
}

func (l *layouter) lineHeight(size float64) float64 { return size * (1.2 + l.th.LineGap) }

func (l *layouter) blocks(bs []markdown.Block, ts textStyle, tight bool) {
	for _, b := range bs {
		l.block(b, ts, tight)
	}
}

// checkTime stops the layout past its deadline.
func (l *layouter) checkTime() {
	if !l.o.Deadline.IsZero() && time.Now().After(l.o.Deadline) {
		l.fail(l.line, "limit_exceeded", "the document takes longer to typeset than a call may last; split it or shorten it",
			map[string]any{"limit": "time"})
	}
}

func (l *layouter) block(b markdown.Block, ts textStyle, tight bool) {
	l.checkTime()
	after := ts.size * 0.75
	if tight {
		after = ts.size * 0.2
	}
	switch v := b.(type) {
	case *markdown.Paragraph:
		l.line = int(v.Line)
		if pics := onlyImages(v.Children); pics != nil {
			for _, img := range pics {
				l.image(img, ts)
			}
			return
		}
		l.text(flatten(l.th, ts, v.Children, l.warn), ts.size, l.th.LineGap, 0)
		l.space(after)
	case *markdown.Heading:
		l.line = int(v.Line)
		lvl := min(v.Level, 3)
		size := l.th.HeadingSizes[lvl-1]
		l.space(size * 0.9)
		l.ensure(size*1.3 + 2*l.lineHeight(l.th.BodySize)) // keep with what follows
		hs := textStyle{font: l.th.Heading, bold: l.th.Heading, size: size, color: l.th.Text}
		dest := "h" + strconv.Itoa(len(l.heads)+1)
		l.add(DestOp{Name: dest, X: l.x, Y: l.y})
		l.heads = append(l.heads, HeadingRef{Level: lvl, Text: plain(v.Children), Page: len(l.pages) - 1, Dest: dest})
		l.text(flatten(l.th, hs, v.Children, l.warn), size, 0.3, 0)
		l.space(size * 0.5)
	case *markdown.ThematicBreak:
		l.line = int(v.Line)
		l.ensure(ts.size)
		y := l.y + ts.size/2
		l.add(LineOp{X1: l.x, Y1: y, X2: l.x + l.w, Y2: y, Width: 0.5, Color: l.th.Rule})
		l.y += ts.size
		l.space(after)
	case *markdown.BlockQuote:
		l.line = int(v.Line)
		l.indented(&frame{x: l.x, w: 2, color: l.th.QuoteBar}, ts.size, func() {
			q := ts
			q.color = l.th.TextMuted
			l.blocks(v.Children, q, false)
		})
		l.space(after)
	case *markdown.Alert:
		l.line = int(v.Line)
		look := l.th.Alerts[v.Kind]
		l.indented(&frame{x: l.x, w: 2.5, color: look.Color}, ts.size, func() {
			label := ts
			label.color = look.Color
			l.text([]rich{{text: look.Label, style: tsStyle(label, true)}}, ts.size, 0.3, 0)
			l.space(ts.size * 0.3)
			l.blocks(v.Children, ts, false)
		})
		l.space(after)
	case *markdown.List:
		l.line = int(v.Line)
		l.list(v, ts)
		l.space(after)
	case *markdown.CodeBlock:
		l.line = int(v.Line)
		lang, _, _ := strings.Cut(strings.TrimSpace(v.Info), " ")
		switch strings.ToLower(lang) {
		case "chart":
			l.chart(v)
		case "mermaid":
			l.mermaid(v, ts)
		default:
			l.code(v.Text)
		}
		l.space(after)
	case *markdown.HTMLBlock:
		l.line = int(v.Line)
		l.code(v.Raw)
		l.space(after)
	case *markdown.Table:
		l.line = int(v.Line)
		l.table(v)
		l.space(after)
	}
	// A block that drew no text line still owes its list marker a place.
	l.flushMarker()
}

// indented lays content out to the right of a bar.
func (l *layouter) indented(bar *frame, size float64, content func()) {
	l.flushMarker()
	l.openFrame(bar)
	x, w := l.x, l.w
	l.x, l.w = x+size, w-size
	content()
	l.x, l.w = x, w
	l.closeFrame()
}

func (l *layouter) list(v *markdown.List, ts textStyle) {
	bullets := []string{"•", "◦", "▪"}
	depth := l.depth
	l.depth++
	defer func() { l.depth-- }()
	markers := make([]string, len(v.Items))
	widest := 0.0
	ms := l.style(tsStyle(ts, false))
	for i, it := range v.Items {
		switch {
		case it.Task != nil && *it.Task:
			markers[i] = "☑"
		case it.Task != nil:
			markers[i] = "☐"
		case v.Ordered:
			markers[i] = strconv.Itoa(v.Start+i) + "."
		default:
			markers[i] = bullets[depth%len(bullets)]
		}
		widest = math.Max(widest, l.doc.Measure(markers[i], ms))
	}
	indent := math.Max(ts.size*1.6, widest+ts.size*0.6)
	for i, it := range v.Items {
		l.line = int(it.Line)
		x, w := l.x, l.w
		l.mark = &marker{spans: []Span{{Text: markers[i], Style: ms}}, x: x + indent - ts.size*0.4, size: ts.size}
		l.x, l.w = x+indent, w-indent
		l.blocks(it.Children, ts, v.Tight)
		l.flushMarker()
		l.x, l.w = x, w
		if i < len(v.Items)-1 && !v.Tight {
			l.space(ts.size * 0.3)
		}
	}
}

func (l *layouter) code(text string) {
	l.flushMarker()
	size := l.th.CodeSize
	cs := Style{Font: l.th.Code, Size: size, R: l.th.Text[0], G: l.th.Text[1], B: l.th.Text[2]}
	l.openFrame(&frame{bg: true, x: l.x, w: l.w, color: l.th.CodeBG, pad: size * 0.8})
	x, w := l.x, l.w
	l.x, l.w = x+size*0.8, w-size*1.6
	src := strings.TrimSuffix(text, "\n")
	for _, ln := range strings.Split(src, "\n") {
		if ln == "" {
			ln = " "
		}
		ln = strings.ReplaceAll(ln, "\t", "    ")
		l.text([]rich{{text: ln, style: cs}}, size, 0.25, 0)
		l.y += size * 0.25
	}
	l.x, l.w = x, w
	l.closeFrame()
}

func (l *layouter) chart(v *markdown.CodeBlock) {
	l.flushMarker()
	spec, n, err := chart.Parse([]byte(v.Text), l.o.Chart)
	if err != nil {
		line := int(v.Line)
		if ce, ok := err.(*chart.Error); ok && ce.Line > 0 {
			line += ce.Line // the block's content starts on the line after the fence
		}
		l.fail(line, "chart_invalid", err.Error(), nil)
	}
	l.points += n
	if l.points > l.o.MaxChartPoints {
		l.fail(int(v.Line), "limit_exceeded",
			fmt.Sprintf("the charts hold more than %d data points in all", l.o.MaxChartPoints),
			map[string]any{"limit": "chart_points", "max": l.o.MaxChartPoints})
	}
	fig := chart.Layout(spec, l.w, chartMeasurer{l}, l.th.Chart)
	for _, w := range fig.Warnings {
		l.warn("chart_labels_thinned", w)
	}
	scale := 1.0
	if limit := l.th.BodyH() * 0.9; fig.H > limit {
		scale = limit / fig.H
		l.warn("chart_scaled", fmt.Sprintf("the chart is taller than a page and is drawn at %.0f%% of its size", scale*100))
	}
	l.ensure(fig.H * scale)
	l.add(FigOp{Fig: fig, X: l.x, Y: l.y, Scale: scale})
	l.y += fig.H * scale
}

// chartMeasurer measures chart text with the body fonts.
type chartMeasurer struct{ l *layouter }

func (m chartMeasurer) Width(s string, size float64, bold bool) float64 {
	f := m.l.th.Body
	if bold {
		f = m.l.th.BodyBold
	}
	return m.l.doc.Measure(s, m.l.style(Style{Font: f, Size: size}))
}

func (l *layouter) mermaid(v *markdown.CodeBlock, ts textStyle) {
	pic, err := l.o.Assets.Mermaid(v.Text, int(v.Line))
	if e, ok := err.(*Error); ok { // a document limit, not a diagram that cannot be drawn
		if e.Line == 0 {
			e.Line = int(v.Line)
		}
		panic(stop{e})
	}
	if err != nil {
		l.warn("mermaid_fallback", "the mermaid diagram is shown as its source: "+err.Error())
		l.code(v.Text)
		return
	}
	l.picture(pic, "", ts)
}

func (l *layouter) image(img *markdown.Image, ts textStyle) {
	pic, err := l.o.Assets.Image(img.Dest, l.line)
	if err != nil {
		if e, ok := err.(*Error); ok {
			if e.Line == 0 {
				e.Line = l.line
			}
			panic(stop{e})
		}
		l.fail(l.line, "image_invalid", err.Error(), nil)
	}
	l.picture(pic, plain(img.Alt), ts)
}

// picture places a raster at its display width, fitted to the column and to
// most of a page, with an optional caption under it.
func (l *layouter) picture(p Picture, caption string, ts textStyle) {
	l.flushMarker()
	if p.PW <= 0 || p.PH <= 0 {
		return
	}
	w := math.Min(p.DispW, l.w)
	h := w * float64(p.PH) / float64(p.PW)
	if limit := l.th.BodyH() * 0.75; h > limit {
		w, h = w*limit/h, limit
	}
	l.ensure(h)
	l.add(PicOp{Pic: p, X: l.x, Y: l.y, W: w, H: h})
	l.y += h
	if caption != "" {
		l.y += ts.size * 0.4
		cs := ts
		cs.size, cs.color = l.th.NoteSize, l.th.TextMuted
		l.text([]rich{{text: caption, style: tsStyle(cs, false)}}, cs.size, 0.3, 0)
	}
	l.space(ts.size * 0.75)
}

// onlyImages returns a paragraph's images when it holds nothing else (but
// whitespace and line breaks), else nil.
func onlyImages(ns []markdown.Inline) []*markdown.Image {
	var out []*markdown.Image
	for _, n := range ns {
		switch v := n.(type) {
		case *markdown.Image:
			out = append(out, v)
		case *markdown.SoftBreak, *markdown.HardBreak:
		case *markdown.Text:
			if strings.TrimSpace(v.Value) != "" {
				return nil
			}
		default:
			return nil
		}
	}
	return out
}

// titleBlock sets the front matter's title, subtitle and byline at the top of
// the first page.
func (l *layouter) titleBlock(meta map[string]string) {
	title := meta["title"]
	if title == "" {
		return
	}
	th := l.th
	l.text([]rich{{text: title, style: Style{Font: th.Heading, Size: th.TitleSize, R: th.Text[0], G: th.Text[1], B: th.Text[2]}}}, th.TitleSize, 0.25, 0)
	if s := meta["subtitle"]; s != "" {
		l.y += th.SubtitleSize * 0.4
		l.text([]rich{{text: s, style: Style{Font: th.Body, Size: th.SubtitleSize, R: th.TextMuted[0], G: th.TextMuted[1], B: th.TextMuted[2]}}}, th.SubtitleSize, 0.25, 0)
	}
	var by []string
	for _, k := range []string{"author", "date"} {
		if v := meta[k]; v != "" {
			by = append(by, v)
		}
	}
	if len(by) > 0 {
		l.y += th.MetaSize * 0.8
		l.text([]rich{{text: strings.Join(by, "　"), style: Style{Font: th.Body, Size: th.MetaSize, R: th.TextMuted[0], G: th.TextMuted[1], B: th.TextMuted[2]}}}, th.MetaSize, 0.25, 0)
	}
	l.y += th.BodySize * 0.8
	l.add(LineOp{X1: l.x, Y1: l.y, X2: l.x + l.w, Y2: l.y, Width: 0.75, Color: th.Rule})
	l.y += th.BodySize * 1.2
}

// footnotes sets the notes at the end of the document, numbered as their
// references are.
func (l *layouter) footnotes(notes []*markdown.Footnote) {
	if len(notes) == 0 {
		return
	}
	th := l.th
	l.space(th.BodySize)
	l.ensure(th.NoteSize * 3)
	l.add(LineOp{X1: l.x, Y1: l.y, X2: l.x + l.w/3, Y2: l.y, Width: 0.5, Color: th.Rule})
	l.y += th.NoteSize
	ts := textStyle{font: th.Body, bold: th.BodyBold, size: th.NoteSize, color: th.Text}
	ms := l.style(tsStyle(ts, false))
	indent := th.NoteSize * 2.2
	for _, n := range notes {
		x, w := l.x, l.w
		l.mark = &marker{spans: []Span{{Text: strconv.Itoa(n.Index) + ".", Style: ms}}, x: x + indent - th.NoteSize*0.4, size: th.NoteSize}
		l.x, l.w = x+indent, w-indent
		l.blocks(n.Children, ts, true)
		l.flushMarker()
		l.x, l.w = x, w
		l.space(th.NoteSize * 0.3)
	}
}
