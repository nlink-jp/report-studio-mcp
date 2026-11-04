package pdf

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// table lays a GFM table out across the column. Columns take their natural
// widths when they fit; otherwise the type shrinks, down to the theme's floor,
// and then cells wrap, never narrower than the longest number in a column
// (numbers are not broken: RFP §2). The header row repeats at the top of
// every page the table continues on, and a row taller than a page continues
// too.
func (l *layouter) table(t *markdown.Table) {
	l.flushMarker()
	if len(t.Align) == 0 {
		return
	}
	p := l.tablePlan(t)
	delete(l.plans, planKey{b: t, w: l.w})
	l.emit(p.warns)
	widths, pad := p.widths, p.pad

	headCells := l.cellLines(p.head, widths, pad)
	headH := rowHeight(headCells, pad)
	// The header starts a page only together with the first row, or it is
	// left alone at the foot of the page.
	if p.first <= l.bottom()-l.top() {
		l.ensure(p.first)
	} else {
		l.ensure(headH)
	}
	var split []int // columns where a number broke across lines
	note := func(cells []cell, toks [][]span16) {
		for c := range cells {
			if c < len(toks) && brokenToken(cells[c].lines, toks[c]) && !slices.Contains(split, c) {
				split = append(split, c)
			}
		}
	}
	drawHead := func() {
		l.row(l.cellLines(p.head, widths, pad), widths, pad, t.Align, true)
	}
	note(headCells, p.headToks)
	drawHead()
	// A header taller than half a page is not repeated: repeating it would
	// leave no room for rows (and recursed through the page hook).
	repeat := headH <= (l.bottom()-l.top())/2
	if repeat {
		l.hooks = append(l.hooks, drawHead)
	}
	for r, row := range p.rows {
		cells := l.cellLines(row, widths, pad)
		note(cells, p.rowToks[r])
		l.row(cells, widths, pad, t.Align, false)
	}
	if repeat {
		l.hooks = l.hooks[:len(l.hooks)-1]
	}
	if len(split) > 0 {
		slices.Sort(split)
		var names []string
		for _, c := range split {
			names = append(names, strings.TrimSpace(plain(t.Header[c])))
		}
		l.line = int(t.Line)
		l.warn("cell_token_split", fmt.Sprintf("the table is too wide to keep its numbers whole: numbers break across lines in the columns %s; fewer columns or a split table would keep them whole", strings.Join(quoteAll(names), ", ")))
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strconv.Quote(s)
	}
	return out
}

// tablePlan is a table's computation at the current width: the type size,
// the column widths, the cells as rich text with their numbers joined, and
// the height of the header and first row.
type tablePlan struct {
	size, pad float64
	widths    []float64
	head      [][]rich
	rows      [][][]rich
	headToks  [][]span16   // per column, the number tokens of the header
	rowToks   [][][]span16 // per row and column
	first     float64      // header and first row
	warns     []markdown.Warning
}

func (l *layouter) tablePlan(t *markdown.Table) *tablePlan {
	k := planKey{b: t, w: l.w}
	if v, ok := l.plans[k]; ok {
		return v.(*tablePlan)
	}
	th := l.th
	ncol := len(t.Align)
	p := &tablePlan{}
	collect := func(code, msg string) {
		p.warns = append(p.warns, markdown.Warning{Line: int(t.Line), Code: code, Message: msg})
	}
	cells := func(size float64, warn func(code, msg string)) ([][]rich, [][][]rich) {
		ts := textStyle{font: th.Body, bold: th.BodyBold, size: size, color: th.Text}
		hs := ts
		hs.font = th.BodyBold
		head := make([][]rich, ncol)
		for c := range head {
			head[c] = flatten(th, hs, t.Header[c], warn)
		}
		rows := make([][][]rich, len(t.Rows))
		for r, row := range t.Rows {
			rows[r] = make([][]rich, ncol)
			for c := range row {
				rows[r][c] = flatten(th, ts, row[c], warn)
			}
		}
		return head, rows
	}

	quiet := func(string, string) {}
	var size, pad float64
	var natural, least []float64
	var head [][]rich
	var rows [][][]rich
	for size = th.TableSize; ; size -= 0.5 {
		pad = size * 0.45
		// Trial sizes stay quiet; the size chosen warns once below (each
		// trial warned again: independent review).
		head, rows = cells(size, quiet)
		natural = make([]float64, ncol)
		least = make([]float64, ncol)
		see := func(c int, rs []rich) {
			for _, ln := range l.doc.Typeset(l.spans(rs), math.MaxFloat32) {
				natural[c] = math.Max(natural[c], ln.Width+2*pad)
			}
			least[c] = math.Max(least[c], l.tokenWidth(rs)+2*pad)
		}
		for c := range head {
			see(c, head[c])
		}
		for _, row := range rows {
			for c := range row {
				see(c, row[c])
			}
		}
		total := 0.0
		for _, w := range natural {
			total += w
		}
		if total <= l.w || size-0.5 < th.TableMinSize {
			if total > l.w {
				collect("table_wrapped", "a wide table is set at its smallest type and its cells wrap")
			}
			break
		}
	}
	for c := range least {
		least[c] = math.Max(least[c], size*3+2*pad)
	}
	head, rows = cells(size, collect)
	p.size, p.pad = size, pad
	p.widths = fitColumns(natural, l.w, least)
	p.head, p.headToks = joinCells(head)
	p.rows = make([][][]rich, len(rows))
	p.rowToks = make([][][]span16, len(rows))
	for r := range rows {
		p.rows[r], p.rowToks[r] = joinCells(rows[r])
	}
	p.first = rowHeight(l.cellLines(p.head, p.widths, pad), pad)
	if len(p.rows) > 0 {
		p.first += rowHeight(l.cellLines(p.rows[0], p.widths, pad), pad)
	}
	l.plans[k] = p
	return p
}

// cell is one table cell's lines and the decorations they carry.
type cell struct {
	lines []Line
	decos []deco
}

func rowHeight(cells []cell, pad float64) float64 {
	need := 0.0
	for _, c := range cells {
		h := 0.0
		for _, ln := range c.lines {
			h += lineH(ln)
		}
		need = math.Max(need, h)
	}
	return need + 2*pad
}

// fitColumns scales natural widths to the available width: up
// proportionally when they fit, else down, keeping each column at least its
// least width. When the least widths alone are too wide, they are scaled
// down together (and cells break inside what they hold).
func fitColumns(natural []float64, avail float64, least []float64) []float64 {
	total, floor := 0.0, 0.0
	for i, w := range natural {
		total += w
		floor += least[i]
	}
	out := make([]float64, len(natural))
	if total <= avail {
		for i, w := range natural {
			out[i] = w * avail / total
		}
		return out
	}
	if floor >= avail {
		for i := range least {
			out[i] = least[i] * avail / floor
		}
		return out
	}
	over := 0.0
	for i, w := range natural {
		over += math.Max(w-least[i], 0)
	}
	room := avail - floor
	for i, w := range natural {
		out[i] = least[i]
		if over > 0 {
			out[i] += math.Max(w-least[i], 0) * room / over
		}
	}
	return out
}

func (l *layouter) cellLines(cells [][]rich, widths []float64, pad float64) []cell {
	out := make([]cell, len(cells))
	for c, rs := range cells {
		if len(rs) > 0 {
			out[c] = cell{lines: l.doc.Typeset(l.spans(rs), widths[c]-2*pad), decos: decorations(rs)}
		}
	}
	return out
}

func lineH(ln Line) float64 { return ln.Ascent + ln.Descent + ln.Leading }

// row places one table row, continuing it on following pages when it is
// taller than what is left of a fresh page.
func (l *layouter) row(cells []cell, widths []float64, pad float64, align []markdown.Align, header bool) {
	l.checkTime()
	th := l.th
	need := rowHeight(cells, pad)
	if !header && l.y+need > l.bottom() && need <= l.bottom()-l.top() {
		l.newPage() // the header hook redraws the header
	}
	next := make([]int, len(cells))
	for {
		avail := l.bottom() - l.y - 2*pad
		top := l.y
		start := len(l.page().Ops)
		used, more := 0.0, false
		x := l.x
		for c := range cells {
			lines := cells[c].lines
			y := top + pad
			i := next[c]
			for ; i < len(lines); i++ {
				h := lineH(lines[i])
				if y+h > top+pad+avail && y > top+pad {
					break
				}
				ln := lines[i]
				lx := x + pad
				switch align[c] {
				case markdown.AlignRight:
					lx = x + widths[c] - pad - ln.Width
				case markdown.AlignCenter:
					lx = x + (widths[c]-ln.Width)/2
				}
				// Strikes, links and code backgrounds as in running text (a
				// struck price read as current without them: independent
				// review), clipped to the cell.
				ox, ow := l.x, l.w
				l.x, l.w = x+pad, widths[c]-2*pad
				cells[c].decos = l.decorate(cells[c].decos, ln, lx, y+ln.Ascent)
				l.x, l.w = ox, ow
				if len(ln.Spans) > 0 {
					l.add(TextOp{Spans: ln.Spans, X: lx, Y: y + ln.Ascent, W: ln.Width, Asc: ln.Ascent, Desc: ln.Descent})
				}
				y += h
			}
			next[c] = i
			used = math.Max(used, y-top-pad)
			if i < len(lines) {
				more = true
			}
			x += widths[c]
		}
		h := used + 2*pad
		if header {
			bg := RectOp{X: l.x, Y: top, W: l.w, H: h, Color: th.TableHeadBG}
			ops := l.page().Ops
			l.page().Ops = append(ops[:start], append([]Op{bg}, ops[start:]...)...)
		}
		l.y = top + h
		width := 0.5
		if header {
			width = 0.75
		}
		l.add(LineOp{X1: l.x, Y1: l.y, X2: l.x + l.w, Y2: l.y, Width: width, Color: th.Rule})
		if !more {
			return
		}
		l.newPage()
	}
}
