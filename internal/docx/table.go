package docx

import (
	"math"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// table writes a GFM table across the column: fixed layout, so the grid is
// what the reader shows; the header row repeats on every page the table
// continues on; rules between rows as the PDF draws them.
func (w *writer) table(t *markdown.Table, c ctx) {
	w.flushLead(c)
	th := w.th
	ncol := len(t.Align)
	if ncol == 0 {
		return
	}
	if w.afterTable {
		// Two tables in a row would merge into one in Word.
		w.para(c, w.base(c, c.pstyle), "")
	}
	cell := func(row [][]markdown.Inline, i int) []markdown.Inline {
		if i < len(row) {
			return row[i]
		}
		return nil
	}
	units := make([]float64, ncol)
	for i := range units {
		units[i] = float64(textWidth(plain(cell(t.Header, i))))
		for _, row := range t.Rows {
			units[i] = math.Max(units[i], float64(textWidth(plain(cell(row, i)))))
		}
	}
	pad := th.TableSize * 0.45
	total := tw(w.availW(c))
	widths := gridWidths(units, total, tw(th.TableSize*3+2*pad))

	rule := hexRGB(th.Rule)
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="ReportTable"/><w:tblW w:w="` + itoa(total) + `" w:type="dxa"/>`)
	if c.indent > 0 {
		b.WriteString(`<w:tblInd w:w="` + itoa(c.indent) + `" w:type="dxa"/>`)
	}
	// Borders and margins are also written directly: not every reader
	// applies table styles.
	b.WriteString(w.tableBorders() + `<w:tblLayout w:type="fixed"/>` + w.cellMargins() +
		`<w:tblLook w:val="0420" w:firstRow="1" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/>` +
		`</w:tblPr><w:tblGrid>`)
	for _, wd := range widths {
		b.WriteString(`<w:gridCol w:w="` + itoa(wd) + `"/>`)
	}
	b.WriteString(`</w:tblGrid>`)

	row := func(cells [][]markdown.Inline, header bool) {
		b.WriteString(`<w:tr>`)
		if header {
			b.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for i := 0; i < ncol; i++ {
			b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="` + itoa(widths[i]) + `" w:type="dxa"/>`)
			if header {
				b.WriteString(`<w:tcBorders>` + border("bottom", 0.75, 0, rule) + `</w:tcBorders>` +
					`<w:shd w:val="clear" w:color="auto" w:fill="` + hexRGB(th.TableHeadBG) + `"/>`)
			}
			b.WriteString(`</w:tcPr>`)
			p := newPPr("TableText")
			switch t.Align[i] {
			case markdown.AlignLeft:
				p.jc = "left"
			case markdown.AlignCenter:
				p.jc = "center"
			case markdown.AlignRight:
				p.jc = "right"
			}
			b.WriteString(`<w:p>` + p.xml() + w.inlines(cell(cells, i), rpr{b: header}) + `</w:p></w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	row(t.Header, true)
	for _, r := range t.Rows {
		row(r, false)
	}
	b.WriteString(`</w:tbl>`)
	w.out.WriteString(b.String())
	w.afterTable = true
}

func (w *writer) tableBorders() string {
	rule := hexRGB(w.th.Rule)
	return `<w:tblBorders>` + border("top", 0.5, 0, rule) + border("bottom", 0.5, 0, rule) +
		border("insideH", 0.5, 0, rule) + `</w:tblBorders>`
}

func (w *writer) cellMargins() string {
	pad := w.th.TableSize * 0.45
	v, h := itoa(tw(pad*0.6)), itoa(tw(pad))
	return `<w:tblCellMar><w:top w:w="` + v + `" w:type="dxa"/><w:left w:w="` + h + `" w:type="dxa"/>` +
		`<w:bottom w:w="` + v + `" w:type="dxa"/><w:right w:w="` + h + `" w:type="dxa"/></w:tblCellMar>`
}

// gridWidths shares total twips among columns in proportion to their
// content (units; an empty column counts as one), keeping every column at
// least least wide (or an equal share when even that does not fit). The
// widths add up to total exactly.
func gridWidths(units []float64, total, least int) []int {
	n := len(units)
	if least*n > total {
		least = total / n
	}
	share := make([]float64, n)
	fixed := make([]bool, n)
	for {
		room, sum := float64(total), 0.0
		for i, u := range units {
			if fixed[i] {
				room -= float64(least)
			} else {
				sum += math.Max(u, 1)
			}
		}
		changed := false
		for i, u := range units {
			if fixed[i] {
				share[i] = float64(least)
				continue
			}
			share[i] = room * math.Max(u, 1) / sum
			if share[i] < float64(least) {
				fixed[i], changed = true, true
			}
		}
		if !changed {
			break
		}
	}
	out := make([]int, n)
	got, widest := 0, 0
	for i, s := range share {
		out[i] = int(math.Floor(s))
		got += out[i]
		if out[i] > out[widest] {
			widest = i
		}
	}
	out[widest] += total - got
	return out
}
