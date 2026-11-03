// Package assets resolves what a manuscript refers to — image files and
// mermaid diagrams — into pictures the typesetters place, inside the limits.
//
// Images come only from files under the work directory, read through fsio,
// and only as PNG or JPEG judged by their content. An image's dimensions are
// read before it is decoded, and every picture, image or diagram, draws on one
// pixel budget: placing pixels is what costs time and output size (phase 0,
// P0-4).
package assets

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"net/url"
	"path"
	"strings"

	mr "github.com/nlink-jp/mermaid-render"
	"github.com/nlink-jp/mermaid-render/raster"

	"github.com/nlink-jp/report-studio-mcp/internal/fsio"
	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
)

// Limits bound the pictures of one document.
type Limits struct {
	ImageBytes  int64 // one image file
	ImageSide   int   // one image's width and height, pixels
	Images      int   // image count
	Diagrams    int   // mermaid diagram count
	PixelBudget int   // all pictures' pixels together
}

// Layout is what the typesetter needs pictures sized for.
type Layout struct {
	BodyPt   float64 // body text size; a diagram's em is shown at it
	ContentW float64 // column width in points
	DPI      float64 // resolution diagrams are drawn for
}

// Assets resolves pictures for one document.
type Assets struct {
	dir    *fsio.Dir
	base   string // the manuscript's directory, relative to the work directory
	lim    Limits
	lay    Layout
	font   *raster.Font
	pixels int
	images int
	charts int
	// Results are kept so that writing a second format draws on the same
	// pictures instead of counting them against the limits again.
	seenImages   map[string]pdf.Picture
	seenDiagrams map[string]diagram
}

type diagram struct {
	pic pdf.Picture
	err error
}

// New returns the resolver for a manuscript at source (relative to the work
// directory opened as dir).
func New(dir *fsio.Dir, source string, lim Limits, lay Layout) *Assets {
	return &Assets{dir: dir, base: path.Dir(source), lim: lim, lay: lay,
		seenImages: map[string]pdf.Picture{}, seenDiagrams: map[string]diagram{}}
}

// Pixels is how much of the budget the document used.
func (a *Assets) Pixels() int { return a.pixels }

func stopErr(code, msg string, details map[string]any) error {
	return &pdf.Error{Code: code, Message: msg, Details: details}
}

func limit(name string, max int, msg string) error {
	return stopErr(toolerr.CodeLimitExceeded, msg, map[string]any{"limit": name, "max": max})
}

// Image reads, checks and decodes the image a manuscript names. Every failure
// stops the document.
func (a *Assets) Image(dest string, line int) (pdf.Picture, error) {
	rel, err := a.resolve(dest)
	if err != nil {
		return pdf.Picture{}, err
	}
	if pic, ok := a.seenImages[rel]; ok {
		return pic, nil
	}
	a.images++
	if a.images > a.lim.Images {
		return pdf.Picture{}, limit("images", a.lim.Images, fmt.Sprintf("the document has more than %d images", a.lim.Images))
	}
	data, err := a.dir.ReadFile(rel, a.lim.ImageBytes)
	if err != nil {
		var te *toolerr.Error
		if errors.As(err, &te) {
			return pdf.Picture{}, stopErr(te.Code, te.Message, te.Details)
		}
		return pdf.Picture{}, stopErr("image_invalid", err.Error(), nil)
	}
	pic, err := Decode(data, dest, a.lim.ImageSide, a.spend)
	if err != nil {
		if e, ok := err.(*pdf.Error); ok {
			if e.Details == nil {
				e.Details = map[string]any{}
			}
			e.Details["path"] = rel
		}
		return pdf.Picture{}, err
	}
	a.seenImages[rel] = pic
	return pic, nil
}

// Decode checks and decodes PNG or JPEG bytes named name: the format by
// content, the dimensions (at most side on each) before decoding, and spend,
// when non-nil, charged with the pixels before decoding too. The picture is
// shown at 150 dpi unless fitted smaller.
func Decode(data []byte, name string, side int, spend func(int) error) (pdf.Picture, error) {
	var decodeConfig func([]byte) (image.Config, error)
	var decode func([]byte) (image.Image, error)
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		decodeConfig = func(b []byte) (image.Config, error) { return png.DecodeConfig(bytes.NewReader(b)) }
		decode = func(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) }
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		decodeConfig = func(b []byte) (image.Config, error) { return jpeg.DecodeConfig(bytes.NewReader(b)) }
		decode = func(b []byte) (image.Image, error) { return jpeg.Decode(bytes.NewReader(b)) }
	default:
		return pdf.Picture{}, stopErr("image_invalid", fmt.Sprintf("%s is not a PNG or JPEG image", name), nil)
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return pdf.Picture{}, stopErr("image_invalid", fmt.Sprintf("%s cannot be read as an image: %v", name, err), nil)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > side || cfg.Height > side {
		return pdf.Picture{}, stopErr(toolerr.CodeLimitExceeded,
			fmt.Sprintf("%s is %dx%d pixels; the limit is %d on each side", name, cfg.Width, cfg.Height, side),
			map[string]any{"limit": "image_side", "max": side})
	}
	if spend != nil {
		if err := spend(cfg.Width * cfg.Height); err != nil {
			return pdf.Picture{}, err
		}
	}
	img, err := decode(data)
	if err != nil {
		return pdf.Picture{}, stopErr("image_invalid", fmt.Sprintf("%s cannot be decoded: %v", name, err), nil)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	return pdf.Picture{Pix: rgba.Pix, PW: cfg.Width, PH: cfg.Height, DispW: float64(cfg.Width) * 72 / 150}, nil
}

// resolve turns a destination into a path relative to the work directory:
// relative to the manuscript, as a relative link is, and never a URL.
func (a *Assets) resolve(dest string) (string, error) {
	if u, err := url.Parse(dest); err == nil && (u.Scheme != "" || u.Host != "") {
		return "", stopErr("image_invalid", fmt.Sprintf("image %q is a URL: images are read from files under the work directory only", dest), nil)
	}
	p, err := url.PathUnescape(dest)
	if err != nil {
		return "", stopErr("image_invalid", fmt.Sprintf("image path %q is not valid: %v", dest, err), nil)
	}
	if strings.HasPrefix(p, "/") {
		return "", stopErr(toolerr.CodePathNotAllowed, fmt.Sprintf("image path %q is absolute: name it relative to the manuscript", dest),
			map[string]any{"reason": "outside_work_dir"})
	}
	return path.Join(a.base, p), nil
}

func (a *Assets) spend(px int) error {
	if a.pixels+px > a.lim.PixelBudget {
		return limit("pixel_budget", a.lim.PixelBudget,
			fmt.Sprintf("the pictures would hold more than %d pixels in all", a.lim.PixelBudget))
	}
	a.pixels += px
	return nil
}

// Mermaid draws a diagram at the scale that gives the layout's resolution at
// its display size (its em shown at the body size, no wider than the column),
// lowered when that would exceed mermaid-render's per-image pixel limit. A
// diagram that cannot be drawn returns mermaid-render's error, which the
// typesetter turns into the source shown with a warning; running out of the
// document's limits returns a *pdf.Error, which stops it.
func (a *Assets) Mermaid(src string, line int) (pdf.Picture, error) {
	if d, ok := a.seenDiagrams[src]; ok {
		return d.pic, d.err
	}
	pic, err := a.mermaid(src)
	if _, stop := err.(*pdf.Error); !stop {
		a.seenDiagrams[src] = diagram{pic, err}
	}
	return pic, err
}

func (a *Assets) mermaid(src string) (pdf.Picture, error) {
	a.charts++
	if a.charts > a.lim.Diagrams {
		return pdf.Picture{}, limit("diagrams", a.lim.Diagrams, fmt.Sprintf("the document has more than %d mermaid diagrams", a.lim.Diagrams))
	}
	if a.font == nil {
		f, err := raster.DefaultFont()
		if err != nil {
			return pdf.Picture{}, err
		}
		a.font = f
	}
	d, err := mr.Parse(src)
	if err != nil {
		return pdf.Picture{}, err
	}
	one, err := raster.Render(d, raster.Options{Font: a.font, Scale: 1})
	if err != nil {
		return pdf.Picture{}, err
	}
	const baseEm = 14 // raster's pixels per em at scale 1
	w1, h1 := float64(one.Bounds().Dx()), float64(one.Bounds().Dy())
	dispW := math.Min(w1/baseEm*a.lay.BodyPt, a.lay.ContentW)
	scale := math.Min(dispW/72*a.lay.DPI/w1, raster.MaxScale)
	// Stay under the per-image limit with a margin for rounding.
	if most := math.Sqrt(float64(raster.MaxPixels)/(w1*h1)) * 0.98; scale > most {
		scale = most
	}
	img := one
	if scale > 1 {
		if img, err = raster.Render(d, raster.Options{Font: a.font, Scale: scale}); err != nil {
			return pdf.Picture{}, err
		}
	}
	b := img.Bounds()
	if err := a.spend(b.Dx() * b.Dy()); err != nil {
		return pdf.Picture{}, err
	}
	return pdf.Picture{Pix: img.Pix, PW: b.Dx(), PH: b.Dy(), DispW: dispW}, nil
}
