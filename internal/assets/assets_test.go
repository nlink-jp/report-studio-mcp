package assets

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mr "github.com/nlink-jp/mermaid-render"
	"github.com/nlink-jp/mermaid-render/raster"

	"github.com/nlink-jp/report-studio-mcp/internal/fsio"
	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
)

type allow struct{}

func (allow) LocalPath(string, string) (string, string)    { return "", "" }
func (allow) OutboundPath(string, string) (string, string) { return "", "" }

var lim = Limits{ImageBytes: 1 << 20, ImageSide: 600, Images: 3, Diagrams: 2, PixelBudget: 1_000_000}
var lay = Layout{BodyPt: 10.5, ContentW: 481.9, DPI: 300}

func encode(t *testing.T, w, h int, f func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var b bytes.Buffer
	if err := f(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngOf(t *testing.T, w, h int) []byte {
	return encode(t, w, h, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) })
}

func setup(t *testing.T, files map[string][]byte) *fsio.Dir {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := fsio.Open(dir, allow{}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func code(err error) string {
	var e *pdf.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return fmt.Sprintf("not a pdf.Error: %v", err)
}

func TestImagesResolveFromTheManuscript(t *testing.T) {
	jpg := encode(t, 30, 20, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) })
	d := setup(t, map[string][]byte{
		"img/chart.png":    pngOf(t, 300, 150),
		"img/写真 1.jpg":     jpg,
		"other-ws/out.png": pngOf(t, 10, 10),
	})
	a := New(d, "report/r.md", lim, lay)
	p, err := a.Image("../img/chart.png", 3)
	if err != nil {
		t.Fatal(err)
	}
	if p.PW != 300 || p.PH != 150 || len(p.Pix) != 300*150*4 || p.DispW != 300*72.0/150 {
		t.Errorf("picture %dx%d, %d bytes, shown at %v", p.PW, p.PH, len(p.Pix), p.DispW)
	}
	// The parser hands destinations percent-encoded.
	if _, err := a.Image("../img/%E5%86%99%E7%9C%9F%201.jpg", 4); err != nil {
		t.Errorf("percent-encoded JPEG: %v", err)
	}
	if _, err := a.Image("../other-ws/out.png", 5); err != nil {
		t.Errorf("another workspace in the same work directory: %v", err)
	}
	if a.Pixels() != 300*150+30*20+10*10 {
		t.Errorf("pixels counted %d", a.Pixels())
	}
}

func TestImagesRefused(t *testing.T) {
	gifData := encode(t, 4, 4, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) })
	d := setup(t, map[string][]byte{
		"r.md":     []byte("x"),
		"anim.gif": gifData,
		"fake.png": []byte("\x89PNG\r\n\x1a\nnot really"),
		"huge.png": pngOf(t, 601, 1),
		"text.png": []byte("hello"),
	})
	for dest, want := range map[string]string{
		"anim.gif":                  "image_invalid",
		"text.png":                  "image_invalid",
		"fake.png":                  "image_invalid",
		"huge.png":                  "limit_exceeded",
		"missing.png":               "file_not_found",
		"https://example.com/a.png": "image_invalid",
		"data:image/png;base64,AA":  "image_invalid",
		"file:///etc/hosts":         "image_invalid",
		"/etc/hosts":                "path_not_allowed",
		"../outside.png":            "path_not_allowed",
	} {
		_, err := New(d, "r.md", lim, lay).Image(dest, 1)
		if got := code(err); got != want {
			t.Errorf("Image(%q) = %s (%v), want %s", dest, got, err, want)
		}
	}
}

func TestImageCountAndPixelBudget(t *testing.T) {
	d := setup(t, map[string][]byte{"a.png": pngOf(t, 500, 500)})
	a := New(d, "r.md", Limits{ImageBytes: 1 << 20, ImageSide: 600, Images: 10, Diagrams: 2, PixelBudget: 400_000}, lay)
	if _, err := a.Image("a.png", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Image("a.png", 2); code(err) != "limit_exceeded" {
		t.Errorf("over the pixel budget: %v", err)
	}
	b := New(d, "r.md", Limits{ImageBytes: 1 << 20, ImageSide: 600, Images: 1, Diagrams: 2, PixelBudget: 10_000_000}, lay)
	_, _ = b.Image("a.png", 1)
	if _, err := b.Image("a.png", 2); code(err) != "limit_exceeded" {
		t.Errorf("over the image count: %v", err)
	}
}

func TestMermaidDrawsWithinTheColumnAndTheLimit(t *testing.T) {
	d := setup(t, nil)
	a := New(d, "r.md", Limits{Diagrams: 5, PixelBudget: 100_000_000}, lay)
	p, err := a.Mermaid("flowchart LR\n  A[開始] --> B{判定}\n  B --> C[終了]\n", 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.DispW <= 0 || p.DispW > lay.ContentW || len(p.Pix) != p.PW*p.PH*4 {
		t.Errorf("picture %dx%d shown at %v", p.PW, p.PH, p.DispW)
	}
	// Drawn for 300 dpi at its display width (within rounding).
	if got := float64(p.PW) / (p.DispW / 72); got < 290 || got > 310 {
		t.Errorf("drawn at %.0f dpi", got)
	}

	// A resolution no diagram could reach is capped at the per-image limit
	// rather than refused.
	big := New(d, "r.md", Limits{Diagrams: 5, PixelBudget: 100_000_000}, Layout{BodyPt: 10.5, ContentW: 481.9, DPI: 5000})
	var chain strings.Builder
	chain.WriteString("flowchart LR\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&chain, "  N%d[ノード %d] --> N%d[ノード %d]\n", i, i, i+1, i+1)
	}
	p, err = big.Mermaid(chain.String(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.PW*p.PH > raster.MaxPixels {
		t.Errorf("%dx%d is over the %d pixel limit", p.PW, p.PH, raster.MaxPixels)
	}
}

func TestMermaidFailuresAreTheEnginesOrALimit(t *testing.T) {
	d := setup(t, nil)
	a := New(d, "r.md", Limits{Diagrams: 1, PixelBudget: 100_000_000}, lay)
	_, err := a.Mermaid("journey\n  title x\n", 1)
	var e *mr.Error
	if !errors.As(err, &e) {
		t.Errorf("unsupported type: %v, want a mermaid-render error", err)
	}
	if _, err := a.Mermaid("flowchart LR\n  A --> B\n", 2); code(err) != "limit_exceeded" {
		t.Errorf("second diagram over a limit of 1: %v", err)
	}
	var _ color.Color // keep image/color for encode helpers
}

// A second format asks for the same pictures; they are not counted again.
func TestRepeatedRequestsAreNotCountedTwice(t *testing.T) {
	d := setup(t, map[string][]byte{"a.png": pngOf(t, 100, 100)})
	a := New(d, "r.md", Limits{ImageBytes: 1 << 20, ImageSide: 600, Images: 1, Diagrams: 1, PixelBudget: 10_000_000}, lay)
	for i := 0; i < 3; i++ {
		if _, err := a.Image("a.png", 1); err != nil {
			t.Fatalf("image request %d: %v", i, err)
		}
		if _, err := a.Mermaid("flowchart LR\n  A --> B\n", 2); err != nil {
			t.Fatalf("diagram request %d: %v", i, err)
		}
	}
	if a.Pixels() == 0 || a.images != 1 || a.charts != 1 {
		t.Errorf("counted %d images, %d diagrams", a.images, a.charts)
	}
}
