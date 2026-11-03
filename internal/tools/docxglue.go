package tools

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"
	"time"

	"github.com/nlink-jp/report-studio-mcp/internal/assets"
	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/docx"
	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// docxOptions connects the .docx writer to what the PDF side already has: the
// same resolved pictures (not counted against the limits again), charts drawn
// by the same layout into PNGs, and font family names from Core Text.
func docxOptions(doc *markdown.Document, th theme.Theme, cover, toc bool, logo *pdf.Picture,
	as *assets.Assets, ctdoc *pdf.Doc, lim chart.Limits, deadline time.Time) docx.Options {
	// A picture placed many times is encoded once (encoding per placement
	// made a 5 KB manuscript take minutes: independent review).
	images := map[string]docx.Picture{}
	diagrams := map[string]docx.Picture{}
	late := func(line int) error {
		if time.Now().After(deadline) {
			return &pdf.Error{Line: line, Code: "limit_exceeded",
				Message: "the document takes longer to write than a call may last; split it or shorten it",
				Details: map[string]any{"limit": "time"}}
		}
		return nil
	}
	o := docx.Options{
		Theme: th,
		Meta: docx.Meta{Title: titleOf(doc), Subtitle: doc.Meta["subtitle"], Author: doc.Meta["author"],
			Date: doc.Meta["date"], Classification: doc.Meta["classification"]},
		Cover: cover,
		TOC:   toc,
		Family: func(ps string) string {
			if f := pdf.FamilyName(ps); f != "" {
				return f
			}
			return ps
		},
		Image: func(dest string, line int) (docx.Picture, error) {
			if pic, ok := images[dest]; ok {
				return pic, nil
			}
			if err := late(line); err != nil {
				return docx.Picture{}, err
			}
			p, err := as.Image(dest, line)
			if err != nil {
				return docx.Picture{}, withLine(err, line)
			}
			pic, err := toDocx(p, th.ContentW())
			if err == nil {
				images[dest] = pic
			}
			return pic, err
		},
		Mermaid: func(src string, line int) (docx.Picture, string, error) {
			if pic, ok := diagrams[src]; ok {
				return pic, "", nil
			}
			if err := late(line); err != nil {
				return docx.Picture{}, "", err
			}
			p, err := as.Mermaid(src, line)
			var stop *pdf.Error
			if errors.As(err, &stop) {
				return docx.Picture{}, "", withLine(err, line)
			}
			if err != nil {
				return docx.Picture{}, "the mermaid diagram is shown as its source: " + err.Error(), nil
			}
			pic, err := toDocx(p, th.ContentW())
			if err == nil {
				diagrams[src] = pic
			}
			return pic, "", err
		},
		Chart: func(src string, line int, width float64) (docx.Picture, []string, error) {
			if err := late(line); err != nil {
				return docx.Picture{}, nil, err
			}
			spec, _, err := chart.Parse([]byte(src), lim)
			if err != nil {
				l := line
				var ce *chart.Error
				if errors.As(err, &ce) && ce.Line > 0 {
					l += ce.Line
				}
				return docx.Picture{}, nil, &pdf.Error{Line: l, Code: "chart_invalid", Message: err.Error()}
			}
			// Not charged to the pixel budget: one chart's raster is capped and
			// freed at once, and charting fifty small charts must not be
			// refused (independent re-review); time is the deadline's.
			data, w, h, _, warns, err := pdf.ChartPNG(ctdoc, th, spec, width, 300)
			if err != nil {
				return docx.Picture{}, nil, &pdf.Error{Line: line, Code: "chart_invalid", Message: "the chart cannot be drawn for the .docx: " + err.Error()}
			}
			return docx.Picture{PNG: data, WidthPt: w, HeightPt: h}, warns, nil
		},
	}
	if logo != nil {
		if p, err := toDocx(*logo, th.ContentW()); err == nil {
			o.Logo = &p
		}
	}
	return o
}

// toDocx gives a picture as a PNG with the size the PDF shows it at: the
// source PNG as it is when there is one (re-encoding grey pixels as RGBA made
// the .docx many times its input), else the pixels encoded.
func toDocx(p pdf.Picture, column float64) (docx.Picture, error) {
	w := math.Min(p.DispW, column)
	pic := docx.Picture{WidthPt: w, HeightPt: w * float64(p.PH) / float64(p.PW)}
	if clean := assets.CleanPNG(p.Orig); clean != nil {
		pic.PNG = clean
		return pic, nil
	}
	img := &image.RGBA{Pix: p.Pix, Stride: p.PW * 4, Rect: image.Rect(0, 0, p.PW, p.PH)}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return docx.Picture{}, err
	}
	pic.PNG = b.Bytes()
	return pic, nil
}

func withLine(err error, line int) error {
	var e *pdf.Error
	if errors.As(err, &e) && e.Line == 0 {
		e.Line = line
	}
	return err
}

// mergeWarnings adds the .docx writer's warnings that the PDF layout did not
// already report (the same line, code and message).
func mergeWarnings(have, more []markdown.Warning) []markdown.Warning {
	seen := map[markdown.Warning]bool{}
	for _, w := range have {
		seen[w] = true
	}
	for _, w := range more {
		if !seen[w] {
			seen[w] = true
			have = append(have, w)
		}
	}
	return have
}
