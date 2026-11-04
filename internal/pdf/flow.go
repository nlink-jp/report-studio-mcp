package pdf

import (
	"math"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/placement"
)

// This file decides where blocks go relative to page breaks (RFP §2, "Page
// breaks and placement"): headings and caption groups are kept with the start
// of what follows, chapters can start a new page, and directives break or
// keep. Measuring is side-effect free: it shares each block's computation
// (paragraph lines, table plan, figure size) with the placement through
// l.plans, so nothing is typeset twice.

// flow places a container's blocks. top is the outermost level of the body,
// the only place chapter breaks apply.
func (l *layouter) flow(bs []markdown.Block, ts textStyle, tight, top bool) {
	for _, g := range placement.Runs(placement.Items(bs)) {
		if g.Items[0].BreakBefore && !l.atTop() {
			l.newPage()
		}
		if g.Kept() {
			if top && g.Chapter(l.th.BreakBefore) && l.pageBody {
				l.newPage()
			}
			l.keepTogether(g.Items, g.Tail, ts, tight)
		}
		for _, it := range g.Items {
			l.block(it.Block, ts, tight)
		}
	}
}

// keepTogether starts a new page when a group and the start of the block
// after it do not fit what is left of this one but fit a fresh page. A group
// that fits no page is placed in order; a caption in it is then reported.
func (l *layouter) keepTogether(group []placement.Item, tail markdown.Block, ts textStyle, tight bool) {
	fresh := l.bottom() - l.top()
	h := 0.0
	for _, g := range group {
		h += l.full(g.Block, ts, tight)
		if h > fresh {
			break
		}
	}
	if tail != nil && h <= fresh {
		h += l.lead(tail, ts, tight)
	}
	if h > fresh {
		for _, g := range group {
			if g.Caption {
				l.line = blockLine(g.Block)
				l.warn("caption_separated", "a caption and what it captions are too tall for one page together and are set apart")
				break
			}
		}
		return
	}
	if l.y+h > l.bottom()+0.01 && !l.atTop() {
		l.newPage()
	}
}

func blockLine(b markdown.Block) int {
	switch v := b.(type) {
	case *markdown.Heading:
		return int(v.Line)
	case *markdown.Paragraph:
		return int(v.Line)
	case *markdown.Table:
		return int(v.Line)
	case *markdown.CodeBlock:
		return int(v.Line)
	case *markdown.List:
		return int(v.Line)
	case *markdown.BlockQuote:
		return int(v.Line)
	case *markdown.Alert:
		return int(v.Line)
	case *markdown.HTMLBlock:
		return int(v.Line)
	case *markdown.ThematicBreak:
		return int(v.Line)
	}
	return 0
}

// after is the space a block leaves below itself.
func after(ts textStyle, tight bool) float64 {
	if tight {
		return ts.size * 0.2
	}
	return ts.size * 0.75
}

// --- measuring ------------------------------------------------------------

// planKey identifies a block's computation at a column width and in a text
// style: a quote sets its paragraphs in a muted colour, so a paragraph measured
// as the start of a quote and placed in it must be computed in that style.
type planKey struct {
	b  any
	w  float64
	ts textStyle
}

type paraPlan struct {
	rs    []rich
	lines []Line
	warns []markdown.Warning
}

// paraPlan is a paragraph's rich text and lines at the current width and in
// ts, computed once; take removes it for the placement, which uses it last.
func (l *layouter) paraPlan(p *markdown.Paragraph, ts textStyle, take bool) paraPlan {
	k := planKey{p, l.w, ts}
	pl, ok := l.plans[k].(paraPlan)
	if !ok {
		var ws []markdown.Warning
		rs := flatten(l.th, ts, p.Children, func(code, msg string) {
			ws = append(ws, markdown.Warning{Code: code, Message: msg})
		})
		pl = paraPlan{rs, l.doc.Typeset(l.spans(rs), l.w), ws}
		l.plans[k] = pl
	}
	if take {
		delete(l.plans, k)
	}
	return pl
}

// headingPlan is paraPlan for a heading, in its heading style.
func (l *layouter) headingPlan(h *markdown.Heading, take bool) paraPlan {
	size := l.th.HeadingSizes[min(h.Level, 3)-1]
	hs := textStyle{font: l.th.Heading, bold: l.th.Heading, size: size, color: l.th.Text}
	k := planKey{h, l.w, hs}
	pl, ok := l.plans[k].(paraPlan)
	if !ok {
		var ws []markdown.Warning
		rs := flatten(l.th, hs, h.Children, func(code, msg string) {
			ws = append(ws, markdown.Warning{Code: code, Message: msg})
		})
		pl = paraPlan{rs, l.doc.Typeset(l.spans(rs), l.w), ws}
		l.plans[k] = pl
	}
	if take {
		delete(l.plans, k)
	}
	return pl
}

// linesHeight is the height of lines set one under another with gap between.
func linesHeight(ls []Line, gap float64) float64 {
	h := 0.0
	for i, ln := range ls {
		if i > 0 {
			h += gap
		}
		h += ln.Ascent + ln.Descent
	}
	return h
}

// paraStart is how many of a paragraph's n lines must stay together at its
// start: all of a paragraph of up to three lines (no single line alone on a
// page), two of a longer one.
func paraStart(n int) int {
	if n <= 3 {
		return n
	}
	return 2
}

// full is the height a kept block takes, all of it, with the space around it.
// Blocks other than headings, paragraphs and figures report +Inf: a group
// holding one is placed without keeping.
func (l *layouter) full(b markdown.Block, ts textStyle, tight bool) float64 {
	switch v := b.(type) {
	case *markdown.Heading:
		size := l.th.HeadingSizes[min(v.Level, 3)-1]
		return size*0.9 + l.headingHeight(v) + size*0.5
	case *markdown.Paragraph:
		if pics := onlyImages(v.Children); pics != nil {
			h := 0.0
			for _, img := range pics {
				h += l.pictureLead(l.imagePicture(img), plain(img.Alt), ts, false) + ts.size*0.75
			}
			return h
		}
		lines := l.paraPlan(v, ts, false).lines
		return linesHeight(lines, ts.size*l.th.LineGap) + after(ts, tight)
	case *markdown.CodeBlock:
		lang, _, _ := strings.Cut(strings.TrimSpace(v.Info), " ")
		// A kept block is kept whole: a figure at its original size (it is
		// not shrunk, or the start after it would no longer fit), code all of
		// it. Measuring stops once past a fresh page: the group is not kept.
		switch strings.ToLower(lang) {
		case "chart":
			fig, scale, ok := l.chartPlan(v)
			if !ok {
				return 0
			}
			return fig.H*scale + after(ts, tight)
		case "mermaid":
			pic, err := l.o.Assets.Mermaid(v.Text, int(v.Line))
			if err != nil {
				return l.codeHeight(v.Text, -1, l.bottom()-l.top()) + after(ts, tight) // shown as its source
			}
			return l.pictureLead(pic, "", ts, false) + ts.size*0.75 + after(ts, tight)
		}
		return l.codeHeight(v.Text, -1, l.bottom()-l.top()) + after(ts, tight)
	case *markdown.ThematicBreak:
		return ts.size + after(ts, tight)
	}
	return math.Inf(1)
}

// lead is the height of the smallest first part a block's placement allows on
// a page: the start the rules keep a heading or caption with.
func (l *layouter) lead(b markdown.Block, ts textStyle, tight bool) float64 {
	switch v := b.(type) {
	case *markdown.Heading:
		return l.full(v, ts, tight)
	case *markdown.Paragraph:
		if pics := onlyImages(v.Children); pics != nil {
			return l.pictureLead(l.imagePicture(pics[0]), plain(pics[0].Alt), ts, true)
		}
		lines := l.paraPlan(v, ts, false).lines
		return linesHeight(lines[:paraStart(len(lines))], ts.size*l.th.LineGap)
	case *markdown.ThematicBreak:
		return ts.size
	case *markdown.List:
		if len(v.Items) == 0 || len(v.Items[0].Children) == 0 {
			return 0
		}
		indent := l.listIndent(v, ts)
		w := l.w
		l.w -= indent
		defer func() { l.w = w }()
		return l.lead(v.Items[0].Children[0], ts, v.Tight)
	case *markdown.BlockQuote:
		if len(v.Children) == 0 {
			return 0
		}
		d := l.indentBy(ts.size)
		w := l.w
		l.w -= d
		defer func() { l.w = w }()
		q := ts
		q.color = l.th.TextMuted // as block() sets a quote's text
		return l.lead(v.Children[0], q, false)
	case *markdown.Alert:
		label := l.lineHeight(ts.size) + ts.size*0.3
		if len(v.Children) == 0 {
			return label
		}
		d := l.indentBy(ts.size)
		w := l.w
		l.w -= d
		defer func() { l.w = w }()
		return label + l.lead(v.Children[0], ts, false)
	case *markdown.CodeBlock:
		lang, _, _ := strings.Cut(strings.TrimSpace(v.Info), " ")
		switch strings.ToLower(lang) {
		case "chart":
			fig, scale, ok := l.chartPlan(v)
			if !ok {
				return 0
			}
			return l.shrinkable(fig.H * scale)
		case "mermaid":
			pic, err := l.o.Assets.Mermaid(v.Text, int(v.Line))
			if err != nil {
				return l.codeLead(v.Text) // shown as its source
			}
			return l.pictureLead(pic, "", ts, true)
		}
		return l.codeLead(v.Text)
	case *markdown.HTMLBlock:
		return l.codeLead(v.Raw)
	case *markdown.Table:
		p := l.tablePlan(v)
		return p.first
	}
	return 0
}

// headingHeight is the height of a heading's lines.
func (l *layouter) headingHeight(h *markdown.Heading) float64 {
	size := l.th.HeadingSizes[min(h.Level, 3)-1]
	return linesHeight(l.headingPlan(h, false).lines, size*0.3)
}

// codeLead is a code block's padding and its first two lines.
func (l *layouter) codeLead(text string) float64 { return l.codeHeight(text, 2, math.Inf(1)) }

// codeHeight is the height of a code block's top padding and its first max
// lines as code() sets them (all of them, and the bottom padding, when max is
// negative). code() leaves a quarter of the size under every line it sets.
// Measuring stops once the height passes limit.
func (l *layouter) codeHeight(text string, max int, limit float64) float64 {
	size := l.th.CodeSize
	cs := l.style(Style{Font: l.th.Code, Size: size})
	src := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	h, n := size*0.8, 0
	for _, ln := range src {
		if n == max {
			break
		}
		if ln == "" {
			ln = " "
		}
		for _, vl := range l.doc.Typeset([]Span{{Text: expandTabs(ln, 4), Style: cs}}, l.w-size*1.6) {
			if n == max {
				break
			}
			h += vl.Ascent + vl.Descent + size*0.25
			n++
		}
		if h > limit {
			return h
		}
	}
	if max < 0 {
		h += size * 0.8
	}
	return h
}

// Shrinking a figure to fit the page (RFP §2): never below minShrink of its
// size, and only where at least shrinkLines body lines are left.
const (
	minShrink   = 0.7
	shrinkLines = 8
)

// shrinkable is the least height a figure of height h may take where it is
// shrunk to fit: all of it if it is short, else what shrinking allows.
func (l *layouter) shrinkable(h float64) float64 {
	return math.Min(h, math.Max(h*minShrink, shrinkLines*l.lineHeight(l.th.BodySize)))
}

// fitScale is the scale a figure of height h (and extra below it) is drawn at
// here: 1 when it fits, the shrink when that is allowed, 0 to move it on.
func (l *layouter) fitScale(h, extra float64) float64 {
	room := l.bottom() - l.y - extra
	if h <= room+0.01 || l.atTop() {
		return 1
	}
	if room >= h*minShrink && room >= shrinkLines*l.lineHeight(l.th.BodySize) {
		return room / h
	}
	return 0
}

// pictureSize is a picture's size fitted to the column and to the page.
func (l *layouter) pictureSize(p Picture) (w, h float64) {
	if p.PW <= 0 || p.PH <= 0 {
		return 0, 0
	}
	w = math.Min(p.DispW, l.w)
	h = w * float64(p.PH) / float64(p.PW)
	if limit := l.th.BodyH() * 0.75; h > limit {
		w, h = w*limit/h, limit
	}
	return w, h
}

// pictureLead is a picture's least height with its caption; least allows the
// shrink.
func (l *layouter) pictureLead(p Picture, caption string, ts textStyle, least bool) float64 {
	_, h := l.pictureSize(p)
	if least {
		h = l.shrinkable(h)
	}
	return h + l.captionHeight(caption, ts)
}

func (l *layouter) captionHeight(caption string, ts textStyle) float64 {
	if caption == "" {
		return 0
	}
	cs := ts
	cs.size = l.th.NoteSize
	lines := l.doc.Typeset(l.spans([]rich{{text: caption, style: tsStyle(cs, false)}}), l.w)
	return ts.size*0.4 + linesHeight(lines, cs.size*0.3)
}

// imagePicture resolves an image for measuring; one that cannot be had
// measures as nothing and stops the document where it is placed.
func (l *layouter) imagePicture(img *markdown.Image) Picture {
	p, err := l.o.Assets.Image(img.Dest, l.line)
	if err != nil {
		return Picture{}
	}
	return p
}

// chartFig is a chart's parsed and laid-out figure, computed once.
type chartFig struct {
	fig    *chart.Figure
	scale  float64
	points int
	err    error
}

func (l *layouter) chartPlan(v *markdown.CodeBlock) (*chart.Figure, float64, bool) {
	c := l.chartFigure(v)
	return c.fig, c.scale, c.err == nil
}

func (l *layouter) chartFigure(v *markdown.CodeBlock) chartFig {
	k := planKey{b: v, w: l.w}
	if c, ok := l.plans[k]; ok {
		return c.(chartFig)
	}
	spec, n, err := chart.Parse([]byte(v.Text), l.o.Chart)
	c := chartFig{points: n, err: err, scale: 1}
	if err == nil {
		c.fig = chart.Layout(spec, l.w, chartMeasurer{l}, l.th.Chart)
		if limit := l.th.BodyH() * 0.9; c.fig.H > limit {
			c.scale = limit / c.fig.H
		}
	}
	l.plans[k] = c
	return c
}
