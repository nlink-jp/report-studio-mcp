package pdf

import (
	"bytes"
	"image"
	"image/png"
	"math"

	"github.com/nlink-jp/report-studio-mcp/internal/chart"
)

// ChartPNG draws a chart spec at width points into a PNG at dpi, with the
// same layout and drawing the PDF uses: a chart is drawn once and emitted
// twice. It returns the PNG, the size to show it at in points, and the
// layout's warnings.
func ChartPNG(doc *Doc, th Theme, spec *chart.Spec, width, dpi float64) ([]byte, float64, float64, []string, error) {
	l := &layouter{doc: doc, th: th}
	fig := chart.Layout(spec, width, chartMeasurer{l}, th.Chart)
	scale := dpi / 72
	pw, ph := int(math.Ceil(fig.W*scale)), int(math.Ceil(fig.H*scale))
	b := NewBitmap(doc, pw, ph, scale)
	surface := toRGB(th.Chart.Surface)
	b.FillRect(0, 0, fig.W, fig.H, surface, 0, RoundNone)
	if err := DrawFigure(&b.Canvas, doc, th, fig, 0, 0, 1); err != nil {
		b.Pixels()
		return nil, 0, 0, nil, err
	}
	img := &image.RGBA{Pix: b.Pixels(), Stride: pw * 4, Rect: image.Rect(0, 0, pw, ph)}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, 0, 0, nil, err
	}
	return out.Bytes(), fig.W, fig.H, fig.Warnings, nil
}
