package pdf

import (
	"math"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// table lays a GFM table out across the column. Columns take their natural
// widths when they fit; otherwise the type shrinks, down to the theme's floor,
// and then cells wrap. The header row repeats at the top of every page the
// table continues on, and a row taller than a page continues too.
func (l *layouter) table(t *markdown.Table) {
	l.flushMarker()
	th := l.th
	ncol := len(t.Align)
	if ncol == 0 {
		return
	}
	cells := func(size float64, header bool) ([][]rich, [][][]rich) {
		ts := textStyle{font: th.Body, bold: th.BodyBold, size: size, color: th.Text}
		hs := ts
		hs.font = th.BodyBold
		head := make([][]rich, ncol)
		for c := range head {
			head[c] = flatten(th, hs, t.Header[c], l.warn)
		}
		rows := make([][][]rich, len(t.Rows))
		for r, row := range t.Rows {
			rows[r] = make([][]rich, ncol)
			for c := range row {
				rows[r][c] = flatten(th, ts, row[c], l.warn)
			}
		}
		return head, rows
	}

	var size, pad float64
	var widths []float64
	var head [][]rich
	var rows [][][]rich
	for size = th.TableSize; ; size -= 0.5 {
		pad = size * 0.45
		head, rows = cells(size, true)
		natural := make([]float64, ncol)
		see := func(c int, rs []rich) {
			for _, ln := range l.doc.Typeset(l.spans(rs), math.MaxFloat32) {
				natural[c] = math.Max(natural[c], ln.Width+2*pad)
			}
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
			widths = fitColumns(natural, l.w, size*3+2*pad)
			if total > l.w {
				l.warn("table_wrapped", "a wide table is set at its smallest type and its cells wrap")
			}
			break
		}
	}

	headLines := l.cellLines(head, widths, pad)
	drawHead := func() {
		l.row(headLines, widths, pad, t.Align, true)
	}
	drawHead()
	l.hooks = append(l.hooks, drawHead)
	for _, row := range rows {
		l.row(l.cellLines(row, widths, pad), widths, pad, t.Align, false)
	}
	l.hooks = l.hooks[:len(l.hooks)-1]
}

// fitColumns scales natural widths to the available width: up
// proportionally when they fit, else down, keeping each column at least
// least wide (or an equal share when even that is too much).
func fitColumns(natural []float64, avail, least float64) []float64 {
	n := float64(len(natural))
	least = math.Min(least, avail/n)
	total := 0.0
	for _, w := range natural {
		total += w
	}
	out := make([]float64, len(natural))
	if total <= avail {
		for i, w := range natural {
			out[i] = w * avail / total
		}
		return out
	}
	over := 0.0
	for _, w := range natural {
		over += math.Max(w-least, 0)
	}
	room := avail - least*n
	for i, w := range natural {
		out[i] = least
		if over > 0 {
			out[i] += math.Max(w-least, 0) * room / over
		}
	}
	return out
}

func (l *layouter) cellLines(cells [][]rich, widths []float64, pad float64) [][]Line {
	out := make([][]Line, len(cells))
	for c, rs := range cells {
		if len(rs) > 0 {
			out[c] = l.doc.Typeset(l.spans(rs), widths[c]-2*pad)
		}
	}
	return out
}

func lineH(ln Line) float64 { return ln.Ascent + ln.Descent + ln.Leading }

// row places one table row, continuing it on following pages when it is
// taller than what is left of a fresh page.
func (l *layouter) row(cells [][]Line, widths []float64, pad float64, align []markdown.Align, header bool) {
	l.checkTime()
	th := l.th
	need := 0.0
	for _, lines := range cells {
		h := 0.0
		for _, ln := range lines {
			h += lineH(ln)
		}
		need = math.Max(need, h)
	}
	if !header && l.y+need+2*pad > l.bottom() && need+2*pad <= l.bottom()-l.top() {
		l.newPage() // the header hook redraws the header
	}
	next := make([]int, len(cells))
	for {
		avail := l.bottom() - l.y - 2*pad
		top := l.y
		start := len(l.page().Ops)
		used, more := 0.0, false
		x := l.x
		for c, lines := range cells {
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
