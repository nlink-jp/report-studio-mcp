package pdf

/*
#cgo CFLAGS: -mmacosx-version-min=26.0
#cgo LDFLAGS: -mmacosx-version-min=26.0 -framework CoreFoundation -framework CoreGraphics -framework CoreText
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"
)

// Style is a registered text style.
type Style struct {
	Font        string // PostScript name
	Size        float64
	R, G, B     float64 // 0..1
	Superscript bool
	Underline   bool
	Oblique     bool
}

// Span is text in one style.
type Span struct {
	Text  string
	Style int
}

// Doc owns the styles of one document.
type Doc struct {
	d      *C.rs_doc
	styles []Style
}

// ErrFontMissing is returned when a style names a font that is not installed.
var ErrFontMissing = errors.New("font is not installed")

// NewDoc registers styles. A style whose font is not installed is an error
// naming it: Core Text would otherwise substitute another font silently.
func NewDoc(styles []Style) (*Doc, error) {
	cs := make([]C.rs_style, len(styles))
	var frees []unsafe.Pointer
	defer func() {
		for _, p := range frees {
			C.free(p)
		}
	}()
	for i, s := range styles {
		name := C.CString(s.Font)
		frees = append(frees, unsafe.Pointer(name))
		cs[i] = cstyle(s, name)
	}
	var missing C.int = -1
	var ptr *C.rs_style
	if len(cs) > 0 {
		ptr = &cs[0]
	}
	d := C.rs_doc_new(ptr, C.int(len(cs)), &missing)
	if d == nil {
		if missing >= 0 && int(missing) < len(styles) {
			return nil, fmt.Errorf("%w: %s", ErrFontMissing, styles[missing].Font)
		}
		return nil, errors.New("cannot create text styles")
	}
	doc := &Doc{d: d, styles: styles}
	runtime.SetFinalizer(doc, func(x *Doc) { x.Close() })
	return doc, nil
}

func cstyle(s Style, name *C.char) C.rs_style {
	return C.rs_style{font: name, size: C.double(s.Size), r: C.double(s.R), g: C.double(s.G), b: C.double(s.B),
		superscript: cbool(s.Superscript), underline: cbool(s.Underline), oblique: cbool(s.Oblique)}
}

// Add registers one more style and returns its index; registering the same
// style twice returns the first index.
func (d *Doc) Add(s Style) (int, error) {
	for i, have := range d.styles {
		if have == s {
			return i, nil
		}
	}
	name := C.CString(s.Font)
	defer C.free(unsafe.Pointer(name))
	cs := cstyle(s, name)
	i := int(C.rs_doc_add_style(d.d, &cs))
	if i < 0 {
		return 0, fmt.Errorf("%w: %s", ErrFontMissing, s.Font)
	}
	d.styles = append(d.styles, s)
	return i, nil
}

// Close releases the styles.
func (d *Doc) Close() {
	if d.d != nil {
		C.rs_doc_free(d.d)
		d.d = nil
	}
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// encoded is spans flattened into one C string with UTF-16 runs.
type encoded struct {
	text *C.char
	runs []C.rs_run
	// u16 maps each span's start to its UTF-16 offset; total length last.
	u16 []int
}

func encode(spans []Span) *encoded {
	var b strings.Builder
	e := &encoded{}
	off := 0
	for _, s := range spans {
		n := len(utf16.Encode([]rune(s.Text)))
		e.u16 = append(e.u16, off)
		if n > 0 {
			e.runs = append(e.runs, C.rs_run{start: C.long(off), len: C.long(n), style: C.int(s.Style)})
		}
		b.WriteString(s.Text)
		off += n
	}
	e.u16 = append(e.u16, off)
	// Core Foundation's C-string constructor stops at NUL; the parser never
	// produces one (CommonMark replaces U+0000), but be sure.
	e.text = C.CString(strings.ReplaceAll(b.String(), "\x00", "�"))
	return e
}

func (e *encoded) free() { C.free(unsafe.Pointer(e.text)) }

func (e *encoded) runPtr() *C.rs_run {
	if len(e.runs) == 0 {
		return nil
	}
	return &e.runs[0]
}

// Line is one line the typesetter broke: spans in their styles, the runes
// [Start, End) of the input it holds, its visible width and vertical metrics
// in points.
type Line struct {
	Spans                    []Span
	Start, End               int
	Width                    float64
	Ascent, Descent, Leading float64
}

// Typeset breaks spans into lines no wider than width.
func (d *Doc) Typeset(spans []Span, width float64) []Line {
	units := utf16Units(spans)
	var lines []Line
	// Core Text's line breaking slows with the length of what it is given
	// (a 2 MiB paragraph took minutes: independent review), so the paragraph
	// is set a window at a time. A window's last line may be cut by the
	// window's end, so it is dropped and the next window starts there; a
	// break depends only on its own line, so the result is the same.
	for pos := 0; pos < len(units); {
		end := min(pos+typesetWindow, len(units))
		for end < len(units) && end > pos+1 && !units[end].first {
			end-- // never split a surrogate pair
		}
		raw := d.typesetRaw(slice(units, pos, end), width)
		if len(raw) == 0 {
			break
		}
		if end < len(units) && len(raw) > 1 {
			raw = raw[:len(raw)-1]
		}
		for _, l := range raw {
			from := pos + int(l.start)
			to := from + int(l.len)
			lines = append(lines, Line{
				Spans:   slice(units, from, to),
				Start:   runeAt(units, from),
				End:     runeAt(units, to),
				Width:   float64(l.width),
				Ascent:  float64(l.ascent),
				Descent: float64(l.descent),
				Leading: float64(l.leading),
			})
		}
		last := raw[len(raw)-1]
		pos += int(last.start + last.len)
	}
	return lines
}

// typesetWindow is how many UTF-16 units Core Text is given at once: dozens
// of lines, so a window's dropped last line costs little. A variable so a
// test can compare against the whole paragraph set at once.
var typesetWindow = 4096

// typesetRaw breaks spans into lines with Core Text, in UTF-16 offsets
// relative to the spans.
func (d *Doc) typesetRaw(spans []Span, width float64) []C.rs_line {
	e := encode(spans)
	defer e.free()
	out := make([]C.rs_line, 64)
	n := C.rs_typeset(d.d, e.text, e.runPtr(), C.int(len(e.runs)), C.double(width), &out[0], C.long(len(out)))
	if n < 0 {
		out = make([]C.rs_line, -n)
		n = C.rs_typeset(d.d, e.text, e.runPtr(), C.int(len(e.runs)), C.double(width), &out[0], C.long(len(out)))
	}
	return out[:n]
}

// runeAt converts a UTF-16 offset into a rune index. Each unit carries its
// rune's index, so this is constant time (counting from the start for every
// line made long paragraphs quadratic: independent review).
func runeAt(us []unit, off int) int {
	if off >= len(us) {
		if len(us) == 0 {
			return 0
		}
		return us[len(us)-1].idx + 1
	}
	return us[max(off, 0)].idx
}

// unit is one UTF-16 code unit's worth of a span: the rune it belongs to is
// emitted at the first unit of the rune only.
type unit struct {
	r     rune
	first bool
	style int
	idx   int // index of the rune in the concatenated text
}

func utf16Units(spans []Span) []unit {
	var us []unit
	idx := 0
	for _, s := range spans {
		for _, r := range s.Text {
			n := len(utf16.Encode([]rune{r}))
			for k := 0; k < n; k++ {
				us = append(us, unit{r: r, first: k == 0, style: s.Style, idx: idx})
			}
			idx++
		}
	}
	return us
}

// slice returns the spans covering UTF-16 units [from, to).
func slice(us []unit, from, to int) []Span {
	var out []Span
	var b strings.Builder
	style := -1
	flush := func() {
		if b.Len() > 0 {
			out = append(out, Span{Text: b.String(), Style: style})
			b.Reset()
		}
	}
	for i := max(from, 0); i < to && i < len(us); i++ {
		u := us[i]
		if !u.first {
			continue
		}
		if u.style != style {
			flush()
			style = u.style
		}
		b.WriteRune(u.r)
	}
	flush()
	return out
}

// FamilyName returns the family name of an installed font given by its
// PostScript name (what Word files name fonts by), or "" when it is not
// installed.
func FamilyName(postScript string) string {
	cn := C.CString(postScript)
	defer C.free(unsafe.Pointer(cn))
	var buf [256]C.char
	if C.rs_family(cn, &buf[0], C.long(len(buf))) == 0 {
		return ""
	}
	return C.GoString(&buf[0])
}

// Measure returns the width of text in one style.
func (d *Doc) Measure(text string, style int) float64 {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	return float64(C.rs_measure(d.d, cs, C.int(style)))
}

// Offset returns the x offset, within the line the spans form, of the start
// of span i (or the end of the line for i == len(spans)).
func (d *Doc) Offset(spans []Span, i int) float64 {
	e := encode(spans)
	defer e.free()
	return float64(C.rs_offset(d.d, e.text, e.runPtr(), C.int(len(e.runs)), C.long(e.u16[i])))
}

// OffsetAt returns the x offset, within the line the spans form, of rune
// index r of their concatenated text.
func (d *Doc) OffsetAt(spans []Span, r int) float64 {
	e := encode(spans)
	defer e.free()
	u16 := 0
	n := 0
	for _, s := range spans {
		for _, c := range s.Text {
			if n == r {
				return float64(C.rs_offset(d.d, e.text, e.runPtr(), C.int(len(e.runs)), C.long(u16)))
			}
			u16 += len(utf16.Encode([]rune{c}))
			n++
		}
	}
	return float64(C.rs_offset(d.d, e.text, e.runPtr(), C.int(len(e.runs)), C.long(u16)))
}

// Canvas draws on a PDF page or a bitmap. Coordinates are points, origin at
// the top-left, y growing downward.
type Canvas struct {
	p   *C.rs_pdf
	doc *Doc
}

// DrawText draws spans as one line with its baseline at (x, y).
func (c *Canvas) DrawText(spans []Span, x, y float64, vertical bool) {
	e := encode(spans)
	defer e.free()
	C.rs_draw_text(c.p, c.doc.d, e.text, e.runPtr(), C.int(len(e.runs)), C.double(x), C.double(y), cbool(vertical))
}

// RoundSide names the side of a rectangle whose corners are rounded.
type RoundSide int

// Rounded sides; the numbers are the bridge's.
const (
	RoundNone RoundSide = iota
	RoundTop
	RoundBottom
	RoundLeft
	RoundRight
)

// FillRect fills a rectangle, optionally rounding one side's corners.
func (c *Canvas) FillRect(x, y, w, h float64, col [3]float64, radius float64, side RoundSide) {
	C.rs_fill_rect(c.p, C.double(x), C.double(y), C.double(w), C.double(h),
		C.double(col[0]), C.double(col[1]), C.double(col[2]), C.double(radius), C.int(side))
}

// StrokeLine strokes a straight line.
func (c *Canvas) StrokeLine(x1, y1, x2, y2, width float64, col [3]float64) {
	C.rs_stroke_line(c.p, C.double(x1), C.double(y1), C.double(x2), C.double(y2), C.double(width),
		C.double(col[0]), C.double(col[1]), C.double(col[2]))
}

// StrokePolyline strokes joined segments with round joins and caps.
func (c *Canvas) StrokePolyline(pts [][2]float64, width float64, col [3]float64) {
	if len(pts) < 2 {
		return
	}
	xy := make([]C.double, 0, 2*len(pts))
	for _, p := range pts {
		xy = append(xy, C.double(p[0]), C.double(p[1]))
	}
	C.rs_stroke_polyline(c.p, &xy[0], C.int(len(pts)), C.double(width), C.double(col[0]), C.double(col[1]), C.double(col[2]))
}

// FillCircle fills a circle.
func (c *Canvas) FillCircle(x, y, r float64, col [3]float64) {
	C.rs_fill_circle(c.p, C.double(x), C.double(y), C.double(r), C.double(col[0]), C.double(col[1]), C.double(col[2]))
}

// DrawRGBA draws premultiplied RGBA pixels (row-major, top row first) into a
// rectangle.
func (c *Canvas) DrawRGBA(pix []byte, pw, ph int, x, y, w, h float64) {
	if len(pix) < pw*ph*4 || pw <= 0 || ph <= 0 {
		return
	}
	C.rs_draw_rgba(c.p, (*C.uchar)(unsafe.Pointer(&pix[0])), C.int(pw), C.int(ph),
		C.double(x), C.double(y), C.double(w), C.double(h))
}

// Link makes a rectangle a link to url (PDF only; a bitmap ignores it).
func (c *Canvas) Link(x, y, w, h float64, url string) {
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	C.rs_link(c.p, C.double(x), C.double(y), C.double(w), C.double(h), cu)
}

// Dest names a point on the current page as a link destination.
func (c *Canvas) Dest(name string, x, y float64) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	C.rs_dest(c.p, cn, C.double(x), C.double(y))
}

// LinkDest makes a rectangle a link to a named destination in the document.
func (c *Canvas) LinkDest(x, y, w, h float64, name string) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	C.rs_link_dest(c.p, C.double(x), C.double(y), C.double(w), C.double(h), cn)
}

// PDFWriter writes a multi-page PDF in memory.
type PDFWriter struct {
	Canvas
}

// NewPDF starts a PDF with pages of w x h points. Title and author go into the
// document information; nothing else about the machine does.
func NewPDF(doc *Doc, w, h float64, title, author string) *PDFWriter {
	ct, ca := C.CString(title), C.CString(author)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(ca))
	return &PDFWriter{Canvas{p: C.rs_pdf_new(C.double(w), C.double(h), ct, ca), doc: doc}}
}

// BeginPage starts a page; EndPage ends it.
func (w *PDFWriter) BeginPage() { C.rs_pdf_begin_page(w.p) }

// EndPage ends the current page.
func (w *PDFWriter) EndPage() { C.rs_pdf_end_page(w.p) }

// Finish closes the document and returns its bytes. The writer is spent.
func (w *PDFWriter) Finish() []byte {
	var n C.long
	ptr := C.rs_pdf_finish(w.p, &n)
	out := C.GoBytes(unsafe.Pointer(ptr), C.int(n))
	C.rs_pdf_free(w.p)
	w.p = nil
	return out
}

// Bitmap is an RGBA raster drawn with the same primitives.
type Bitmap struct {
	Canvas
	b      *C.rs_bitmap
	pw, ph int
}

// MaxBitmapPixels bounds one raster: what an int32 byte count holds, with
// room to spare (the same 12 Mpx mermaid-render draws within).
const MaxBitmapPixels = 12 << 20

// NewBitmap makes a pw x ph pixel raster whose drawing coordinates are points
// scaled by scale (pixels per point). It refuses a raster over
// MaxBitmapPixels or one Core Graphics cannot allocate.
func NewBitmap(doc *Doc, pw, ph int, scale float64) (*Bitmap, error) {
	if pw <= 0 || ph <= 0 || int64(pw)*int64(ph) > MaxBitmapPixels {
		return nil, fmt.Errorf("a %dx%d raster is outside 1..%d pixels", pw, ph, MaxBitmapPixels)
	}
	b := C.rs_bitmap_new(C.int(pw), C.int(ph), C.double(scale))
	if b == nil {
		return nil, errors.New("Core Graphics cannot allocate the raster")
	}
	return &Bitmap{Canvas: Canvas{p: C.rs_bitmap_canvas(b), doc: doc}, b: b, pw: pw, ph: ph}, nil
}

// Pixels copies the raster out as premultiplied RGBA, top row first, and
// releases it.
func (b *Bitmap) Pixels() []byte {
	out := C.GoBytes(unsafe.Pointer(C.rs_bitmap_pixels(b.b)), C.int(b.pw*b.ph*4))
	C.rs_bitmap_free(b.b)
	b.b = nil
	return out
}
