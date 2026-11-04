package docx

import (
	"fmt"
	"math"
	"strings"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/placement"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// ctx is what the enclosing blocks decide for the paragraphs inside them.
type ctx struct {
	pstyle string // style of text paragraphs: "" (Normal), "Quote", "FootnoteText"
	indent int    // left indent in twips from the margin
	bar    string // the left border of an enclosing quote or alert, as w:left
	tight  bool   // a tight list's item: paragraphs without spacing
	depth  int    // list nesting
	lead   *lead  // what the next paragraph starts with
	top    bool   // the outermost level of the body: chapter breaks apply
}

// lead is what the first paragraph of a list item or footnote carries: the
// list number, the task box, or the footnote's own mark. A block that is not
// a paragraph first writes it as a paragraph of its own (a table cannot carry
// a number).
type lead struct {
	num  *numRef
	hang int    // hanging indent for the number, twips
	task string // "☐ " or "☑ "
	note bool   // the footnote's reference mark
	done bool
}

// availW is the width in points the context leaves for a picture or table.
func (w *writer) availW(c ctx) float64 { return math.Max(w.th.ContentW()-float64(c.indent)/20, 1) }

// quoteIndent is how far a quote or alert moves its content right.
func (w *writer) quoteIndent() int { return tw(w.th.BodySize) }

// listStep is the indent of one list level; the number hangs in it.
func (w *writer) listStep() int { return tw(w.th.BodySize * 2) }

// barSpace is the gap between a quote's bar and its text, in points.
func (w *writer) barSpace() int { return int(math.Round(math.Min(w.th.BodySize*0.6, 31))) }

// base is a paragraph's properties in a context: its indent and the bar of
// the quote it is in.
func (w *writer) base(c ctx, style string) ppr {
	p := newPPr(style)
	if c.indent > 0 {
		p.ind, p.left = true, c.indent
	}
	p.bdr = c.bar
	return p
}

// para writes a paragraph, giving it the pending lead if there is one.
func (w *writer) para(c ctx, p ppr, content string) {
	if l := c.lead; l != nil && !l.done {
		l.done = true
		if l.num != nil {
			p.num = l.num
			p.ind, p.left, p.hanging = true, c.indent, l.hang
		}
		if l.task != "" {
			content = w.run(l.task, rpr{}) + content
		}
		if l.note {
			content = `<w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteRef/></w:r>` +
				w.run(" ", rpr{}) + content
		}
	}
	if w.afterTable && p.before < 0 {
		p.before = tw(w.th.BodySize * 0.75) // a table has no spacing of its own after it
	}
	if w.breakNext {
		p.pageBreakBefore = true
		w.breakNext = false
	}
	if w.keep {
		p.keepNext, p.keepLines = true, true
	}
	// Headings do not make a page's body (a chapter break follows them only
	// after text), unless inside a quote or alert, whose bar the PDF draws as
	// body. A list marker beside a heading is drawn with it and does not count.
	if !strings.HasPrefix(p.style, "Heading") || c.bar != "" {
		w.hasBody = true
	}
	w.started = true
	w.out.WriteString(`<w:p>` + p.xml() + content + `</w:p>`)
	w.afterTable = false
}

// flushLead writes a pending lead as a paragraph of its own, before a block
// that cannot carry it.
func (w *writer) flushLead(c ctx) {
	if c.lead != nil && !c.lead.done {
		w.para(c, w.base(c, c.pstyle), "")
	}
}

// endContainer closes a body or footnote: Word wants a paragraph after a
// table there.
func (w *writer) endContainer() {
	if w.afterTable {
		w.out.WriteString(`<w:p/>`)
		w.afterTable = false
	}
}

// blocks writes a container's blocks. Caption groups and blocks a
// keep-with-next directive marked keep with the next paragraph (headings do
// through their style); a pagebreak directive, and a chapter heading after
// body text, start the next paragraph on a new page.
func (w *writer) blocks(bs []markdown.Block, c ctx) {
	for _, g := range placement.Runs(placement.Items(bs)) {
		if g.Items[0].BreakBefore {
			w.forceBreak()
		}
		if g.Kept() && c.top && !w.inNote && g.Chapter(w.th.BreakBefore) && w.hasBody {
			w.forceBreak()
		}
		for _, it := range g.Items {
			w.keep = it.Keep
			w.block(it.Block, c)
			w.keep = false
		}
	}
}

// forceBreak starts the next paragraph on a new page. Several in a row are
// one break, and at the top of a page it does nothing (as in the PDF).
func (w *writer) forceBreak() {
	if !w.started && w.topStart {
		return
	}
	w.breakNext = true
	w.hasBody = false
}

func (w *writer) block(b markdown.Block, c ctx) {
	th := w.th
	switch v := b.(type) {
	case *markdown.Paragraph:
		w.line = int(v.Line)
		if pics := onlyImages(v.Children); pics != nil {
			for _, img := range pics {
				w.image(img, c)
			}
			return
		}
		p := w.base(c, c.pstyle)
		if c.tight {
			p.after = tw(th.BodySize * 0.2)
		}
		w.para(c, p, w.inlines(v.Children, rpr{}))
	case *markdown.Heading:
		w.line = int(v.Line)
		lvl := min(max(v.Level, 1), 3) // deeper headings look like level 3, as in the PDF
		content := w.inlines(v.Children, rpr{})
		if w.o.TOC && !w.inNote {
			w.marks++
			name := fmt.Sprintf("_Toc%08d", w.marks)
			id := itoa(w.marks)
			content = `<w:bookmarkStart w:id="` + id + `" w:name="` + name + `"/>` + content + `<w:bookmarkEnd w:id="` + id + `"/>`
			w.heads = append(w.heads, tocEntry{level: lvl, text: plain(v.Children), bookmark: name})
		}
		w.para(c, w.base(c, "Heading"+itoa(lvl)), content)
	case *markdown.ThematicBreak:
		w.line = int(v.Line)
		p := w.base(c, c.pstyle)
		p.bdr += border("bottom", 0.5, 1, hexRGB(th.Rule))
		w.para(c, p, "")
	case *markdown.BlockQuote:
		w.line = int(v.Line)
		w.flushLead(c)
		q := c
		q.pstyle, q.tight, q.lead, q.top = "Quote", false, nil, false
		q.indent = c.indent + w.quoteIndent()
		q.bar = border("left", 2, w.barSpace(), hexRGB(th.QuoteBar))
		w.blocks(v.Children, q)
	case *markdown.Alert:
		w.line = int(v.Line)
		w.flushLead(c)
		look, ok := th.Alerts[v.Kind]
		if !ok {
			look = theme.AlertLook{Label: string(v.Kind), Color: th.Text}
		}
		a := c
		a.tight, a.lead, a.top = false, nil, false
		a.indent = c.indent + w.quoteIndent()
		a.bar = border("left", 2.5, w.barSpace(), hexRGB(look.Color))
		p := w.base(a, a.pstyle)
		p.keepNext, p.after = true, tw(th.BodySize*0.3)
		w.para(a, p, w.run(look.Label, rpr{b: true, color: hexRGB(look.Color)}))
		w.blocks(v.Children, a)
	case *markdown.List:
		w.line = int(v.Line)
		w.list(v, c)
	case *markdown.CodeBlock:
		w.line = int(v.Line)
		lang, _, _ := strings.Cut(strings.TrimSpace(v.Info), " ")
		switch strings.ToLower(lang) {
		case "chart":
			w.chart(v, c)
		case "mermaid":
			w.mermaid(v, c)
		default:
			w.code(v.Text, c)
		}
	case *markdown.HTMLBlock:
		w.line = int(v.Line)
		w.code(v.Raw, c)
	case *markdown.Table:
		w.line = int(v.Line)
		w.table(v, c)
	}
}

// list writes a list's items as numbered paragraphs. Bullets share one
// numbering instance; each ordered list gets its own, restarting at its
// first number. Nesting is the numbering level.
func (w *writer) list(v *markdown.List, c ctx) {
	w.flushLead(c)
	ilvl := min(c.depth, 8)
	id := 1
	if v.Ordered {
		w.nums = append(w.nums, numInst{ilvl: ilvl, start: v.Start})
		id = 1 + len(w.nums)
	}
	step := w.listStep()
	for _, it := range v.Items {
		w.line = int(it.Line)
		ic := c
		ic.indent, ic.depth, ic.tight, ic.top = c.indent+step, c.depth+1, v.Tight, false
		ld := &lead{hang: step}
		switch {
		case it.Task != nil && *it.Task:
			ld.task = "☑ "
		case it.Task != nil:
			ld.task = "☐ " // the box stands where the bullet would, as in the PDF
		default:
			ld.num = &numRef{id: id, ilvl: ilvl}
		}
		ic.lead = ld
		w.blocks(it.Children, ic)
		w.flushLead(ic)
	}
}

// code writes source text one paragraph per line in the Code Block style,
// spaces kept and tabs as tabs.
func (w *writer) code(text string, c ctx) {
	w.flushLead(c)
	src := strings.TrimSuffix(text, "\n")
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		p := w.base(c, "CodeBlock")
		// The first two lines stay together: what a heading or caption is kept
		// with (the PDF keeps the start of a code block the same way).
		p.keepNext = i == 0 && len(lines) > 1
		// The style's padding borders reach codePad left of the text; keep
		// them inside the column.
		p.ind, p.left = true, c.indent+tw(codePad+1)
		if i == len(lines)-1 {
			p.after = tw(w.th.BodySize * 0.75)
		}
		w.para(c, p, w.run(strings.TrimSuffix(ln, "\r"), rpr{}))
	}
}

// codePad is the padding, in points, of a code block's background.
const codePad = 4

func (w *writer) chart(v *markdown.CodeBlock, c ctx) {
	if w.o.Chart == nil {
		w.code(v.Text, c)
		return
	}
	pic, msgs, err := w.o.Chart(v.Text, int(v.Line), w.availW(c))
	if err != nil {
		panic(stop{err})
	}
	for _, m := range msgs {
		w.warn("chart_labels_thinned", m)
	}
	w.picture(c, pic, "", 0.9)
}

func (w *writer) mermaid(v *markdown.CodeBlock, c ctx) {
	if w.o.Mermaid == nil {
		w.code(v.Text, c)
		return
	}
	pic, fallback, err := w.o.Mermaid(v.Text, int(v.Line))
	if err != nil {
		panic(stop{err})
	}
	if fallback != "" {
		w.warn("mermaid_fallback", fallback)
		w.code(v.Text, c)
		return
	}
	w.picture(c, pic, "", 0.75)
}

func (w *writer) image(img *markdown.Image, c ctx) {
	alt := plain(img.Alt)
	if w.o.Image == nil {
		w.para(c, w.base(c, c.pstyle), w.run("["+alt+"]", rpr{}))
		return
	}
	pic, err := w.o.Image(img.Dest, w.line)
	if err != nil {
		panic(stop{err})
	}
	w.picture(c, pic, alt, 0.75)
}

// picture places a PNG in a paragraph of its own at its size, fitted to the
// column and to frac of the page body (as the PDF does, so a tall picture is
// not cut off at the page foot), with the caption, if any, under it.
func (w *writer) picture(c ctx, pic Picture, caption string, frac float64) {
	wpt, hpt, ok := fit(pic, w.availW(c), w.th.BodyH()*frac)
	if !ok {
		w.flushLead(c)
		return
	}
	p := w.base(c, c.pstyle)
	if caption != "" {
		p.keepNext, p.after = true, tw(w.th.BodySize*0.4)
	}
	w.para(c, p, w.drawing(pic.PNG, wpt, hpt, caption))
	if caption != "" {
		w.para(c, w.base(c, "Caption"), w.run(caption, rpr{}))
	}
}

// fit is a picture's shown size: never wider than maxW nor taller than maxH,
// aspect kept. ok is false for a picture with nothing to show.
func fit(pic Picture, maxW, maxH float64) (wpt, hpt float64, ok bool) {
	if len(pic.PNG) == 0 || !(pic.WidthPt > 0) || !(pic.HeightPt > 0) || math.IsInf(pic.WidthPt, 0) || math.IsInf(pic.HeightPt, 0) {
		return 0, 0, false
	}
	wpt = math.Min(pic.WidthPt, maxW)
	hpt = pic.HeightPt * wpt / pic.WidthPt
	if hpt > maxH {
		wpt, hpt = wpt*maxH/hpt, maxH
	}
	return wpt, hpt, wpt > 0 && hpt > 0
}

// drawing is an inline picture run. The picture's name is generic and its
// description is the caption only: never a path (RFP §2, metadata).
func (w *writer) drawing(png []byte, wpt, hpt float64, descr string) string {
	name, ok := w.mediaByPN[string(png)]
	if !ok {
		name = "media/image" + itoa(len(w.media)+1) + ".png"
		w.media = append(w.media, mediaFile{name: name, data: png})
		w.mediaByPN[string(png)] = name
	}
	rid := w.cur.add(relImage, name, false)
	w.pics++
	n := itoa(w.pics)
	cx, cy := fmt.Sprint(emu(wpt)), fmt.Sprint(emu(hpt))
	d := ""
	if descr != "" {
		d = ` descr="` + w.attr(descr) + `"`
	}
	return `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">` +
		`<wp:extent cx="` + cx + `" cy="` + cy + `"/><wp:effectExtent l="0" t="0" r="0" b="0"/>` +
		`<wp:docPr id="` + n + `" name="Picture ` + n + `"` + d + `/>` +
		`<wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr>` +
		`<a:graphic><a:graphicData uri="` + nsPic + `"><pic:pic>` +
		`<pic:nvPicPr><pic:cNvPr id="` + n + `" name="Picture ` + n + `"/><pic:cNvPicPr/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip r:embed="` + rid + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>` +
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + cx + `" cy="` + cy + `"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic>` +
		`</wp:inline></w:drawing></w:r>`
}

// onlyImages returns a paragraph's images when it holds nothing else (but
// whitespace and line breaks), else nil.
func onlyImages(ns []markdown.Inline) []*markdown.Image { return markdown.OnlyImages(ns) }

// footnotes is footnotes.xml: the two separators settings.xml refers to,
// then each note, id = its display number, in the FootnoteText style.
func (w *writer) footnotes() string {
	var b strings.Builder
	w.out, w.cur, w.inNote, w.afterTable = &b, &w.noteRels, true, false
	defer func() { w.inNote = false }()
	b.WriteString(xmlHead + `<w:footnotes xmlns:w="` + nsW + `" xmlns:r="` + nsR + `" xmlns:wp="` + nsWP +
		`" xmlns:a="` + nsA + `" xmlns:pic="` + nsPic + `">` +
		`<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>` +
		`<w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>`)
	for _, n := range w.doc.Footnotes {
		if !w.noteSeen[n.Index] {
			continue // no reference in the body: orphanNotes sets it there
		}
		b.WriteString(`<w:footnote w:id="` + itoa(n.Index) + `">`)
		c := ctx{pstyle: "FootnoteText", lead: &lead{note: true}}
		w.blocks(n.Children, c)
		w.flushLead(c)
		w.endContainer()
		b.WriteString(`</w:footnote>`)
	}
	b.WriteString(`</w:footnotes>`)
	return b.String()
}

// orphanNotes sets, at the end of the body, the notes nothing in the body
// refers to (only another note or an image description does). Word shows a
// footnote only at its reference, so such a note would vanish (independent
// review); here it keeps its number, which is what the text that refers to
// it shows.
func (w *writer) orphanNotes() {
	var orphans []*markdown.Footnote
	for _, n := range w.doc.Footnotes {
		if !w.noteSeen[n.Index] {
			orphans = append(orphans, n)
		}
	}
	if len(orphans) == 0 {
		return
	}
	w.inNote = true // references in them stay numbers, as in footnotes
	defer func() { w.inNote = false }()
	w.blocks([]markdown.Block{&markdown.ThematicBreak{}}, ctx{})
	for _, n := range orphans {
		c := ctx{pstyle: "FootnoteText", lead: &lead{task: itoa(n.Index) + ". "}}
		w.blocks(n.Children, c)
		w.flushLead(c)
	}
}

// titleBlock is the front matter's title, subtitle and byline at the top of
// the first page when there is no cover, as the PDF sets them; only a title
// the front matter names (not one taken from the first heading, which the
// body already shows) makes one.
func (w *writer) titleBlock() {
	if w.doc.Meta["title"] == "" {
		return
	}
	m := w.o.Meta
	title := m.Title
	if title == "" {
		title = w.doc.Meta["title"]
	}
	type par struct {
		p       ppr
		content string
	}
	ps := []par{{newPPr("Title"), w.run(title, rpr{})}}
	if m.Subtitle != "" {
		ps = append(ps, par{newPPr("Subtitle"), w.run(m.Subtitle, rpr{})})
	}
	var by []string
	for _, v := range []string{m.Author, m.Date} {
		if v != "" {
			by = append(by, v)
		}
	}
	if len(by) > 0 {
		p := newPPr("")
		p.before = tw(w.th.MetaSize * 0.8)
		ps = append(ps, par{p, w.run(strings.Join(by, "　"), rpr{sz: w.th.MetaSize, color: hexRGB(w.th.TextMuted)})})
	}
	last := &ps[len(ps)-1].p
	last.bdr = border("bottom", 0.75, int(math.Round(w.th.BodySize*0.8)), hexRGB(w.th.Rule))
	last.after = tw(w.th.BodySize * 1.2)
	for _, x := range ps {
		w.para(ctx{}, x.p, x.content)
	}
}

// cover is the title page: the logo at the top, the title block about a
// third of the way down, the classification below, all centred, as the PDF
// lays it out. The section's titlePg keeps the page furniture off it.
func (w *writer) cover() {
	th, m := w.th, w.o.Meta
	muted := hexRGB(th.TextMuted)
	logoH := 0.0
	if l := w.o.Logo; l != nil {
		if wpt, hpt, ok := fit(*l, th.ContentW()*0.35, th.BodyH()*0.15); ok {
			p := newPPr("")
			p.jc, p.after = "center", 0
			w.para(ctx{}, p, w.drawing(l.PNG, wpt, hpt, ""))
			logoH = hpt
		}
	}
	p := newPPr("Title")
	p.jc, p.before = "center", tw(math.Max(0, th.BodyH()*0.32-logoH))
	w.para(ctx{}, p, w.run(m.Title, rpr{sz: th.TitleSize * 1.3}))
	if m.Subtitle != "" {
		p := newPPr("Subtitle")
		p.jc, p.before = "center", tw(th.SubtitleSize)
		w.para(ctx{}, p, w.run(m.Subtitle, rpr{}))
	}
	for _, v := range []string{m.Author, m.Date} {
		if v != "" {
			p := newPPr("")
			p.jc, p.before = "center", tw(th.MetaSize*1.6)
			w.para(ctx{}, p, w.run(v, rpr{sz: th.MetaSize * 1.2, color: muted}))
		}
	}
	if m.Classification != "" {
		p := newPPr("")
		p.jc, p.before = "center", tw(th.BodyH()*0.2)
		w.para(ctx{}, p, w.run(m.Classification, rpr{b: true, sz: th.MetaSize * 1.2}))
	}
	w.out.WriteString(pageBreak)
}

// pageBreak is a paragraph holding only a page break, its mark kept small so
// that it adds no visible line.
const pageBreak = `<w:p><w:pPr><w:spacing w:before="0" w:after="0"/><w:rPr><w:sz w:val="2"/></w:rPr></w:pPr><w:r><w:br w:type="page"/></w:r></w:p>`

// toc is the table of contents on a page of its own: a TOC field whose
// result is pre-filled with one entry per heading, each a link to the
// bookmark on its heading. There are no page numbers in the pre-filled
// result (pagination is Word's); the field is not marked dirty, because a
// dirty field makes Word ask on every open, and updating it in Word or Pages
// adds the numbers at the TOC styles' right tab.
func (w *writer) toc() {
	if len(w.heads) == 0 {
		return // as in the PDF: no headings, no table of contents
	}
	if t := w.th.TOCTitle; t != "" {
		w.para(ctx{}, newPPr("TOCHeading"), w.run(t, rpr{}))
	}
	begin := `<w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
		`<w:r><w:instrText xml:space="preserve"> TOC \o "1-3" \h \z \u </w:instrText></w:r>` +
		`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`
	for i, h := range w.heads {
		content := `<w:hyperlink w:anchor="` + h.bookmark + `" w:history="1">` + w.run(h.text, rpr{}) + `</w:hyperlink>`
		if i == 0 {
			content = begin + content
		}
		w.para(ctx{}, newPPr("TOC"+itoa(h.level)), content)
	}
	w.out.WriteString(`<w:p><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:br w:type="page"/></w:r></w:p>`)
}
