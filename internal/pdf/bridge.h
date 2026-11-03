// Bridge between Go and macOS Core Text / Core Graphics. Every function takes
// plain C values so the Go side never holds a Core Foundation object across a
// call except the two opaque handles below.
#ifndef REPORT_STUDIO_BRIDGE_H
#define REPORT_STUDIO_BRIDGE_H

#include <stddef.h>

// A text style, registered once per document and referred to by index.
typedef struct {
	const char *font;   // PostScript name
	double size;        // points
	double r, g, b;     // 0..1
	int superscript;    // 1 raises and shrinks (footnote marks)
	int underline;      // 1 underlines (links)
	int oblique;        // 1 slants the glyphs (emphasis in fonts without an italic)
} rs_style;

// A run of text in one style. start and len are in UTF-16 units.
typedef struct {
	long start;
	long len;
	int style;
} rs_run;

// One line the typesetter broke, in UTF-16 units, with its metrics.
typedef struct {
	long start;
	long len;
	double width;
	double ascent;
	double descent;
	double leading;
} rs_line;

typedef struct rs_doc rs_doc;

// rs_doc_new creates a document whose styles are given once; it returns NULL
// and fills missing (when non-NULL) with the index of a style whose font is
// not installed.
rs_doc *rs_doc_new(const rs_style *styles, int nstyles, int *missing);
// rs_doc_add_style registers one more style and returns its index, or -1 when
// its font is not installed.
int rs_doc_add_style(rs_doc *d, const rs_style *s);
void rs_doc_free(rs_doc *d);

// rs_typeset breaks utf8 text into lines no wider than width. It returns the
// number of lines written to out (at most cap), or -(lines needed) when cap is
// too small.
long rs_typeset(rs_doc *d, const char *utf8, const rs_run *runs, int nruns, double width,
                rs_line *out, long cap);

// rs_measure returns the typographic width of utf8 in one style.
double rs_measure(rs_doc *d, const char *utf8, int style);

// rs_offset returns the x offset of a UTF-16 index within a laid-out line.
double rs_offset(rs_doc *d, const char *utf8, const rs_run *runs, int nruns, long index);

// PDF drawing. Coordinates are points with the origin at the page's top-left,
// y growing downward; the bridge flips them.
typedef struct rs_pdf rs_pdf;

rs_pdf *rs_pdf_new(double w, double h, const char *title, const char *author);
void rs_pdf_begin_page(rs_pdf *p);
void rs_pdf_end_page(rs_pdf *p);
// rs_pdf_finish closes the document and returns its bytes (owned by p until
// rs_pdf_free); *n receives the length.
const unsigned char *rs_pdf_finish(rs_pdf *p, long *n);
void rs_pdf_free(rs_pdf *p);

// rs_draw_text draws utf8 with its runs as one line, baseline at (x, y).
// vertical rotates it to read bottom to top about (x, y).
void rs_draw_text(rs_pdf *p, rs_doc *d, const char *utf8, const rs_run *runs, int nruns,
                  double x, double y, int vertical);
void rs_fill_rect(rs_pdf *p, double x, double y, double w, double h,
                  double r, double g, double b, double radius, int round_side);
void rs_stroke_line(rs_pdf *p, double x1, double y1, double x2, double y2, double width,
                    double r, double g, double b);
void rs_stroke_polyline(rs_pdf *p, const double *xy, int npoints, double width,
                        double r, double g, double b);
void rs_fill_circle(rs_pdf *p, double x, double y, double radius, double r, double g, double b);
// rs_draw_rgba draws premultiplied RGBA pixels into the rectangle.
void rs_draw_rgba(rs_pdf *p, const unsigned char *pix, int pw, int ph,
                  double x, double y, double w, double h);
// rs_link makes the rectangle a link to url.
void rs_link(rs_pdf *p, double x, double y, double w, double h, const char *url);

// Bitmap drawing for the .docx side: the same primitives into an RGBA buffer.
typedef struct rs_bitmap rs_bitmap;
rs_bitmap *rs_bitmap_new(int pw, int ph, double scale);
rs_pdf *rs_bitmap_canvas(rs_bitmap *b);
const unsigned char *rs_bitmap_pixels(rs_bitmap *b);
void rs_bitmap_free(rs_bitmap *b);

#endif
