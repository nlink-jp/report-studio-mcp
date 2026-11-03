#include "bridge.h"

#include <CoreFoundation/CoreFoundation.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreText/CoreText.h>
#include <stdlib.h>
#include <string.h>

struct rs_doc {
	int n;
	CFDictionaryRef *attrs;
};

static CFStringRef cfstr(const char *s) {
	return CFStringCreateWithCString(NULL, s, kCFStringEncodingUTF8);
}

static CGColorRef rgb(double r, double g, double b) {
	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	CGFloat c[4] = {r, g, b, 1};
	CGColorRef col = CGColorCreate(cs, c);
	CGColorSpaceRelease(cs);
	return col;
}

rs_doc *rs_doc_new(const rs_style *styles, int nstyles, int *missing) {
	rs_doc *d = calloc(1, sizeof *d);
	d->n = nstyles;
	d->attrs = calloc((size_t)nstyles, sizeof *d->attrs);
	for (int i = 0; i < nstyles; i++) {
		const rs_style *s = &styles[i];
		double size = s->superscript ? s->size * 0.65 : s->size;
		CFStringRef name = cfstr(s->font);
		CTFontRef font = CTFontCreateWithName(name, size, NULL);
		// Core Text substitutes a default for an unknown name: compare.
		CFStringRef got = CTFontCopyPostScriptName(font);
		int ok = CFStringCompare(got, name, 0) == kCFCompareEqualTo;
		CFRelease(got);
		CFRelease(name);
		if (!ok) {
			CFRelease(font);
			if (missing) *missing = i;
			d->n = i;
			rs_doc_free(d);
			return NULL;
		}
		CGColorRef col = rgb(s->r, s->g, s->b);
		CFMutableDictionaryRef a = CFDictionaryCreateMutable(NULL, 4,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		CFDictionarySetValue(a, kCTFontAttributeName, font);
		CFDictionarySetValue(a, kCTForegroundColorAttributeName, col);
		if (s->superscript) {
			double off = s->size * 0.35;
			CFNumberRef n = CFNumberCreate(NULL, kCFNumberDoubleType, &off);
			CFDictionarySetValue(a, kCTBaselineOffsetAttributeName, n);
			CFRelease(n);
		}
		if (s->underline) {
			int u = kCTUnderlineStyleSingle;
			CFNumberRef n = CFNumberCreate(NULL, kCFNumberIntType, &u);
			CFDictionarySetValue(a, kCTUnderlineStyleAttributeName, n);
			CFRelease(n);
		}
		d->attrs[i] = a;
		CFRelease(font);
		CGColorRelease(col);
	}
	return d;
}

void rs_doc_free(rs_doc *d) {
	if (!d) return;
	for (int i = 0; i < d->n; i++) {
		if (d->attrs[i]) CFRelease(d->attrs[i]);
	}
	free(d->attrs);
	free(d);
}

static CFAttributedStringRef attributed(rs_doc *d, const char *utf8, const rs_run *runs, int nruns) {
	CFStringRef s = cfstr(utf8);
	CFMutableAttributedStringRef as = CFAttributedStringCreateMutable(NULL, 0);
	CFAttributedStringReplaceString(as, CFRangeMake(0, 0), s);
	CFIndex len = CFStringGetLength(s);
	CFRelease(s);
	CFAttributedStringBeginEditing(as);
	for (int i = 0; i < nruns; i++) {
		CFIndex st = runs[i].start, ln = runs[i].len;
		if (st < 0 || ln <= 0 || st + ln > len || runs[i].style < 0 || runs[i].style >= d->n) continue;
		CFAttributedStringSetAttributes(as, CFRangeMake(st, ln), d->attrs[runs[i].style], true);
	}
	CFAttributedStringEndEditing(as);
	return as;
}

long rs_typeset(rs_doc *d, const char *utf8, const rs_run *runs, int nruns, double width,
                rs_line *out, long cap) {
	CFAttributedStringRef as = attributed(d, utf8, runs, nruns);
	CTTypesetterRef ts = CTTypesetterCreateWithAttributedString(as);
	CFIndex len = CFAttributedStringGetLength(as), start = 0;
	long n = 0;
	while (start < len) {
		CFIndex count = CTTypesetterSuggestLineBreak(ts, start, width);
		if (count <= 0) count = CTTypesetterSuggestClusterBreak(ts, start, width);
		if (count <= 0) count = 1;
		if (n < cap) {
			CTLineRef line = CTTypesetterCreateLine(ts, CFRangeMake(start, count));
			CGFloat a = 0, de = 0, le = 0;
			double w = CTLineGetTypographicBounds(line, &a, &de, &le);
			w -= CTLineGetTrailingWhitespaceWidth(line);
			out[n] = (rs_line){start, count, w, a, de, le};
			CFRelease(line);
		}
		n++;
		start += count;
	}
	CFRelease(ts);
	CFRelease(as);
	return n <= cap ? n : -n;
}

double rs_measure(rs_doc *d, const char *utf8, int style) {
	rs_run r = {0, 0, style};
	CFStringRef s = cfstr(utf8);
	r.len = CFStringGetLength(s);
	CFRelease(s);
	CFAttributedStringRef as = attributed(d, utf8, &r, 1);
	CTLineRef line = CTLineCreateWithAttributedString(as);
	double w = CTLineGetTypographicBounds(line, NULL, NULL, NULL);
	CFRelease(line);
	CFRelease(as);
	return w;
}

double rs_offset(rs_doc *d, const char *utf8, const rs_run *runs, int nruns, long index) {
	CFAttributedStringRef as = attributed(d, utf8, runs, nruns);
	CTLineRef line = CTLineCreateWithAttributedString(as);
	double x = CTLineGetOffsetForStringIndex(line, index, NULL);
	CFRelease(line);
	CFRelease(as);
	return x;
}

struct rs_pdf {
	CGContextRef ctx;
	CFMutableDataRef data; // PDF only
	double h;              // page height in points, for flipping
	int pdf;
};

rs_pdf *rs_pdf_new(double w, double h, const char *title, const char *author) {
	rs_pdf *p = calloc(1, sizeof *p);
	p->data = CFDataCreateMutable(NULL, 0);
	p->h = h;
	p->pdf = 1;
	CGDataConsumerRef dc = CGDataConsumerCreateWithCFData(p->data);
	CGRect box = CGRectMake(0, 0, w, h);
	CFMutableDictionaryRef aux = CFDictionaryCreateMutable(NULL, 2,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (title && *title) {
		CFStringRef t = cfstr(title);
		CFDictionarySetValue(aux, kCGPDFContextTitle, t);
		CFRelease(t);
	}
	if (author && *author) {
		CFStringRef a = cfstr(author);
		CFDictionarySetValue(aux, kCGPDFContextAuthor, a);
		CFRelease(a);
	}
	p->ctx = CGPDFContextCreate(dc, &box, aux);
	CFRelease(aux);
	CGDataConsumerRelease(dc);
	return p;
}

void rs_pdf_begin_page(rs_pdf *p) { CGPDFContextBeginPage(p->ctx, NULL); }
void rs_pdf_end_page(rs_pdf *p) { CGPDFContextEndPage(p->ctx); }

const unsigned char *rs_pdf_finish(rs_pdf *p, long *n) {
	CGPDFContextClose(p->ctx);
	*n = (long)CFDataGetLength(p->data);
	return CFDataGetBytePtr(p->data);
}

void rs_pdf_free(rs_pdf *p) {
	if (!p) return;
	CGContextRelease(p->ctx);
	if (p->data) CFRelease(p->data);
	free(p);
}

// fy flips a top-down y to Quartz's bottom-up space.
static double fy(rs_pdf *p, double y) { return p->h - y; }

void rs_draw_text(rs_pdf *p, rs_doc *d, const char *utf8, const rs_run *runs, int nruns,
                  double x, double y, int vertical) {
	CFAttributedStringRef as = attributed(d, utf8, runs, nruns);
	CTLineRef line = CTLineCreateWithAttributedString(as);
	CGContextSaveGState(p->ctx);
	CGContextSetTextMatrix(p->ctx, CGAffineTransformIdentity);
	if (vertical) {
		CGContextTranslateCTM(p->ctx, x, fy(p, y));
		CGContextRotateCTM(p->ctx, M_PI / 2);
		CGContextSetTextPosition(p->ctx, 0, 0);
	} else {
		CGContextSetTextPosition(p->ctx, x, fy(p, y));
	}
	CTLineDraw(line, p->ctx);
	CGContextRestoreGState(p->ctx);
	CFRelease(line);
	CFRelease(as);
}

void rs_fill_rect(rs_pdf *p, double x, double y, double w, double h,
                  double r, double g, double b, double radius, int round_side) {
	CGFloat x0 = x, x1 = x + w, y0 = fy(p, y + h), y1 = fy(p, y); // y1 is the top edge
	CGMutablePathRef path = CGPathCreateMutable();
	if (radius <= 0) {
		CGPathAddRect(path, NULL, CGRectMake(x0, y0, w, h));
	} else {
		// round_side: 1 top, 2 bottom, 3 left, 4 right (on the page).
		int tl = round_side == 1 || round_side == 3, tr = round_side == 1 || round_side == 4;
		int bl = round_side == 2 || round_side == 3, br = round_side == 2 || round_side == 4;
		CGPathMoveToPoint(path, NULL, x0 + (bl ? radius : 0), y0);
		if (br) CGPathAddArcToPoint(path, NULL, x1, y0, x1, y0 + radius, radius);
		else CGPathAddLineToPoint(path, NULL, x1, y0);
		if (tr) CGPathAddArcToPoint(path, NULL, x1, y1, x1 - radius, y1, radius);
		else CGPathAddLineToPoint(path, NULL, x1, y1);
		if (tl) CGPathAddArcToPoint(path, NULL, x0, y1, x0, y1 - radius, radius);
		else CGPathAddLineToPoint(path, NULL, x0, y1);
		if (bl) CGPathAddArcToPoint(path, NULL, x0, y0, x0 + radius, y0, radius);
		else CGPathAddLineToPoint(path, NULL, x0, y0);
		CGPathCloseSubpath(path);
	}
	CGContextSetRGBFillColor(p->ctx, r, g, b, 1);
	CGContextAddPath(p->ctx, path);
	CGContextFillPath(p->ctx);
	CGPathRelease(path);
}

void rs_stroke_line(rs_pdf *p, double x1, double y1, double x2, double y2, double width,
                    double r, double g, double b) {
	CGContextSetRGBStrokeColor(p->ctx, r, g, b, 1);
	CGContextSetLineWidth(p->ctx, width);
	CGContextSetLineCap(p->ctx, kCGLineCapButt);
	CGContextMoveToPoint(p->ctx, x1, fy(p, y1));
	CGContextAddLineToPoint(p->ctx, x2, fy(p, y2));
	CGContextStrokePath(p->ctx);
}

void rs_stroke_polyline(rs_pdf *p, const double *xy, int npoints, double width,
                        double r, double g, double b) {
	if (npoints < 2) return;
	CGContextSetRGBStrokeColor(p->ctx, r, g, b, 1);
	CGContextSetLineWidth(p->ctx, width);
	CGContextSetLineCap(p->ctx, kCGLineCapRound);
	CGContextSetLineJoin(p->ctx, kCGLineJoinRound);
	CGContextMoveToPoint(p->ctx, xy[0], fy(p, xy[1]));
	for (int i = 1; i < npoints; i++) CGContextAddLineToPoint(p->ctx, xy[2 * i], fy(p, xy[2 * i + 1]));
	CGContextStrokePath(p->ctx);
}

void rs_fill_circle(rs_pdf *p, double x, double y, double radius, double r, double g, double b) {
	CGContextSetRGBFillColor(p->ctx, r, g, b, 1);
	CGContextFillEllipseInRect(p->ctx, CGRectMake(x - radius, fy(p, y) - radius, 2 * radius, 2 * radius));
}

void rs_draw_rgba(rs_pdf *p, const unsigned char *pix, int pw, int ph,
                  double x, double y, double w, double h) {
	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	CFDataRef data = CFDataCreate(NULL, pix, (CFIndex)pw * ph * 4);
	CGDataProviderRef dp = CGDataProviderCreateWithCFData(data);
	CGImageRef img = CGImageCreate(pw, ph, 8, 32, (size_t)pw * 4, cs,
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrderDefault, dp, NULL, true, kCGRenderingIntentDefault);
	CGContextSetInterpolationQuality(p->ctx, kCGInterpolationHigh);
	CGContextDrawImage(p->ctx, CGRectMake(x, fy(p, y + h), w, h), img);
	CGImageRelease(img);
	CGDataProviderRelease(dp);
	CFRelease(data);
	CGColorSpaceRelease(cs);
}

void rs_link(rs_pdf *p, double x, double y, double w, double h, const char *url) {
	if (!p->pdf) return;
	CFStringRef s = cfstr(url);
	CFURLRef u = CFURLCreateWithString(NULL, s, NULL);
	CFRelease(s);
	if (!u) return;
	CGPDFContextSetURLForRect(p->ctx, u, CGRectMake(x, fy(p, y + h), w, h));
	CFRelease(u);
}

struct rs_bitmap {
	rs_pdf canvas;
};

rs_bitmap *rs_bitmap_new(int pw, int ph, double scale) {
	rs_bitmap *b = calloc(1, sizeof *b);
	CGColorSpaceRef cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
	b->canvas.ctx = CGBitmapContextCreate(NULL, pw, ph, 8, (size_t)pw * 4, cs,
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
	CGColorSpaceRelease(cs);
	CGContextScaleCTM(b->canvas.ctx, scale, scale);
	b->canvas.h = ph / scale;
	b->canvas.pdf = 0;
	return b;
}

rs_pdf *rs_bitmap_canvas(rs_bitmap *b) { return &b->canvas; }

const unsigned char *rs_bitmap_pixels(rs_bitmap *b) {
	return CGBitmapContextGetData(b->canvas.ctx);
}

void rs_bitmap_free(rs_bitmap *b) {
	if (!b) return;
	CGContextRelease(b->canvas.ctx);
	free(b);
}
