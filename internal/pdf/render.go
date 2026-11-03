package pdf

import (
	"strconv"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
)

// Meta is what the PDF's document information and page furniture carry.
type Meta struct {
	Title, Author, Classification string
}

// Render draws laid-out pages into a PDF: the content, then the page number
// in the bottom margin and the classification, if any, in the top one.
func Render(r *Result, doc *Doc, th Theme, meta Meta) ([]byte, error) {
	w := NewPDF(doc, th.PageW, th.PageH, meta.Title, meta.Author)
	furniture, err := doc.Add(Style{Font: th.Body, Size: th.NoteSize, R: th.TextMuted[0], G: th.TextMuted[1], B: th.TextMuted[2]})
	if err != nil {
		return nil, err
	}
	total := strconv.Itoa(len(r.Pages))
	for i, p := range r.Pages {
		w.BeginPage()
		if err := drawOps(&w.Canvas, doc, th, p.Ops); err != nil {
			return nil, err
		}
		num := strconv.Itoa(i+1) + " / " + total
		nw := doc.Measure(num, furniture)
		w.DrawText([]Span{{num, furniture}}, (th.PageW-nw)/2, th.PageH-th.MarginB/2, false)
		if meta.Classification != "" {
			cw := doc.Measure(meta.Classification, furniture)
			w.DrawText([]Span{{meta.Classification, furniture}}, th.PageW-th.MarginR-cw, th.MarginT/2+th.NoteSize/2, false)
		}
		w.EndPage()
	}
	return w.Finish(), nil
}

func drawOps(c *Canvas, doc *Doc, th Theme, ops []Op) error {
	for _, op := range ops {
		switch o := op.(type) {
		case TextOp:
			c.DrawText(o.Spans, o.X, o.Y, false)
		case RectOp:
			c.FillRect(o.X, o.Y, o.W, o.H, o.Color, o.Radius, o.Side)
		case LineOp:
			c.StrokeLine(o.X1, o.Y1, o.X2, o.Y2, o.Width, o.Color)
		case PicOp:
			c.DrawRGBA(o.Pic.Pix, o.Pic.PW, o.Pic.PH, o.X, o.Y, o.W, o.H)
		case FigOp:
			if err := DrawFigure(c, doc, th, o.Fig, o.X, o.Y, o.Scale); err != nil {
				return err
			}
		case LinkOp:
			c.Link(o.X, o.Y, o.W, o.H, o.URL)
		}
	}
	return nil
}

var roundSides = map[string]RoundSide{"top": RoundTop, "bottom": RoundBottom, "left": RoundLeft, "right": RoundRight}

// DrawFigure draws a chart figure with its top-left at (x, y), scaled. The
// same call draws into a PDF page and into the bitmap the .docx embeds.
func DrawFigure(c *Canvas, doc *Doc, th Theme, f *chart.Figure, x, y, s float64) error {
	px := func(v float64) float64 { return x + v*s }
	py := func(v float64) float64 { return y + v*s }
	for _, it := range f.Items {
		switch v := it.(type) {
		case chart.Rect:
			c.FillRect(px(v.X), py(v.Y), v.W*s, v.H*s, toRGB(v.Fill), v.Round*s, roundSides[v.RoundSide])
		case chart.Line:
			c.StrokeLine(px(v.X1), py(v.Y1), px(v.X2), py(v.Y2), v.Width*s, toRGB(v.Color))
		case chart.Polyline:
			pts := make([][2]float64, len(v.Points))
			for i, p := range v.Points {
				pts[i] = [2]float64{px(p[0]), py(p[1])}
			}
			c.StrokePolyline(pts, v.Width*s, toRGB(v.Color))
		case chart.Circle:
			if v.Ring > 0 {
				c.FillCircle(px(v.X), py(v.Y), (v.R+v.Ring)*s, toRGB(v.RingColor))
			}
			c.FillCircle(px(v.X), py(v.Y), v.R*s, toRGB(v.Fill))
		case chart.Text:
			font := th.Body
			if v.Bold {
				font = th.BodyBold
			}
			col := toRGB(v.Color)
			id, err := doc.Add(Style{Font: font, Size: v.Size * s, R: col[0], G: col[1], B: col[2]})
			if err != nil {
				return err
			}
			w := doc.Measure(v.S, id)
			tx, ty := px(v.X), py(v.Y)
			// The figure measured at size v.Size; anchor with the drawn width.
			off := 0.0
			switch v.Anchor {
			case chart.Middle:
				off = w / 2
			case chart.End:
				off = w
			}
			if v.Vertical {
				ty += off
			} else {
				tx -= off
			}
			c.DrawText([]Span{{v.S, id}}, tx, ty, v.Vertical)
		}
	}
	return nil
}
