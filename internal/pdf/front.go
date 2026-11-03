package pdf

import (
	"math"
	"strconv"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
)

// sub returns a layouter for separate pages (cover, table of contents) that
// shares this one's styles and theme.
func (l *layouter) sub() *layouter {
	s := &layouter{doc: l.doc, o: l.o, th: l.th}
	s.x, s.w = l.th.MarginL, l.th.ContentW()
	return s
}

// cover lays out the title page: the logo at the top, the title block about a
// third of the way down, the classification near the bottom, all centred.
func (l *layouter) cover(md *markdown.Document) []*Page {
	th := l.th
	s := l.sub()
	s.newPage()
	s.page().Cover = true
	if p := l.o.Logo; p != nil && p.PW > 0 && p.PH > 0 {
		w := math.Min(p.DispW, th.ContentW()*0.35)
		h := w * float64(p.PH) / float64(p.PW)
		if limit := th.BodyH() * 0.15; h > limit {
			w, h = w*limit/h, limit
		}
		s.add(PicOp{Pic: *p, X: th.MarginL + (th.ContentW()-w)/2, Y: th.MarginT, W: w, H: h})
	}
	title := md.Meta["title"]
	if title == "" && len(l.heads) > 0 {
		title = l.heads[0].Text
	}
	style := func(font string, size float64, col RGB) Style {
		return Style{Font: font, Size: size, R: col[0], G: col[1], B: col[2]}
	}
	s.y = th.MarginT + th.BodyH()*0.32
	if title != "" {
		size := th.TitleSize * 1.3
		s.text([]rich{{text: title, style: style(th.Heading, size, th.Text)}}, size, 0.25, 1)
	}
	if v := md.Meta["subtitle"]; v != "" {
		s.y += th.SubtitleSize
		s.text([]rich{{text: v, style: style(th.Body, th.SubtitleSize, th.TextMuted)}}, th.SubtitleSize, 0.25, 1)
	}
	for _, k := range []string{"author", "date"} {
		if v := md.Meta[k]; v != "" {
			s.y += th.MetaSize * 1.6
			s.text([]rich{{text: v, style: style(th.Body, th.MetaSize*1.2, th.TextMuted)}}, th.MetaSize*1.2, 0.25, 1)
		}
	}
	if v := md.Meta["classification"]; v != "" {
		s.y = math.Max(s.y+th.MetaSize*2, s.bottom()-th.MetaSize*4)
		s.text([]rich{{text: v, style: style(th.BodyBold, th.MetaSize*1.2, th.Text)}}, th.MetaSize*1.2, 0.25, 1)
	}
	for _, p := range s.pages {
		p.Cover = true // a title block longer than a page continues, unnumbered
	}
	return s.pages
}

// toc lays out the table of contents. Each entry links to its heading and
// shows the page it is printed on: front pages come before it, then the
// table's own n pages (nil lays it out with placeholders, to learn n).
func (l *layouter) toc(heads []HeadingRef, front int, n *int) []*Page {
	th := l.th
	s := l.sub()
	s.newPage()
	ts := textStyle{font: th.Body, bold: th.BodyBold, size: th.BodySize, color: th.Text}
	hs := textStyle{font: th.Heading, bold: th.Heading, size: th.HeadingSizes[0], color: th.Text}
	s.text([]rich{{text: th.TOCTitle, style: tsStyle(hs, false)}}, hs.size, 0.3, 0)
	s.y += th.BodySize * 1.2
	for _, h := range heads {
		num := "000"
		if n != nil {
			num = strconv.Itoa(front + *n + h.Page + 1)
		}
		st := tsStyle(ts, h.Level == 1)
		numID := s.style(st)
		numW := s.doc.Measure(num, numID)
		indent := float64(h.Level-1) * th.BodySize * 1.5
		x, w := s.x, s.w
		s.ensure(s.lineHeight(th.BodySize))
		y0, page := s.y, len(s.pages)
		s.x, s.w = x+indent, w-indent-numW-th.BodySize*1.5
		text := h.Text
		if strings.TrimSpace(text) == "" {
			text = "—" // an empty heading still has a page; give the number a line
		}
		s.text([]rich{{text: text, style: st}}, th.BodySize, 0.3, 0)
		s.x, s.w = x, w
		s.add(TextOp{Spans: []Span{{Text: num, Style: numID}}, X: x + w - numW, Y: s.base, W: numW,
			Asc: th.BodySize * 0.88, Desc: th.BodySize * 0.12})
		if page != len(s.pages) { // broken across pages: link the part on this one
			y0 = s.top()
		}
		s.add(LinkOp{X: x, Y: y0, W: w, H: s.y - y0, Dest: h.Dest})
		s.y += th.BodySize * 0.45
	}
	return s.pages
}
