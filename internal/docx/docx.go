// Package docx writes a parsed manuscript as a Word document: Office Open
// XML, ECMA-376 5th edition Part 1, in the Transitional form Word itself
// writes (pinned in docs/ja/phase0-criteria.ja.md, P0-5).
//
// The profile becomes Word styles (Normal, Heading 1-3, Quote, Code Block,
// caption, footnote text, a table style, ...) so that whoever receives the
// file can restyle it in Word; the body carries as little direct formatting
// as the layout allows. Pictures (images, charts, mermaid diagrams) arrive as
// PNGs from the caller; this package never reads files and never draws.
//
// Nothing here guesses at pagination: the table of contents is a real TOC
// field whose result is pre-filled with the headings (no page numbers), so a
// reader that never updates fields still shows it, and Word or Pages can
// regenerate it with numbers.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// Picture is a PNG ready to place, with the size to show it at.
type Picture struct {
	PNG               []byte
	WidthPt, HeightPt float64
}

// Meta is the document's front matter as the writer needs it.
type Meta struct{ Title, Subtitle, Author, Date, Classification string }

// Options configure one document.
type Options struct {
	Theme theme.Theme
	Meta  Meta
	// Cover and TOC are already resolved from profile and front matter by the
	// caller.
	Cover, TOC bool
	// Family maps a PostScript font name (theme.Body etc.) to the family name
	// Word uses. Nil uses the PostScript name as is.
	Family func(postScript string) string
	// Image resolves ![alt](dest). An error stops the document and is
	// returned as is. Nil shows each image as its description.
	Image func(dest string, line int) (Picture, error)
	// Mermaid draws a ```mermaid block. fallback != "" means: show the source
	// as a code block and warn with code "mermaid_fallback" and that message.
	// err != nil stops. Nil shows every diagram as its source.
	Mermaid func(src string, line int) (pic Picture, fallback string, err error)
	// Chart draws a ```chart block at widthPt. The returned strings are
	// warnings (code "chart_labels_thinned"); err != nil stops. Nil shows
	// every chart as its source.
	Chart func(src string, line int, widthPt float64) (Picture, []string, error)
	// Logo, when non-nil, goes on the cover.
	Logo *Picture
}

// stop carries a caller's error out of the walk through panic, so the many
// places that can stop need no error plumbing; Write recovers it.
type stop struct{ err error }

// Relationship types.
const (
	relStyles    = relDoc + "/styles"
	relSettings  = relDoc + "/settings"
	relNumbering = relDoc + "/numbering"
	relFootnotes = relDoc + "/footnotes"
	relHeader    = relDoc + "/header"
	relFooter    = relDoc + "/footer"
	relImage     = relDoc + "/image"
	relLink      = relDoc + "/hyperlink"
)

type rel struct {
	id, typ, target string
	external        bool
}

// rels are one part's relationships. Hyperlinks and pictures in footnotes
// belong to footnotes.xml's own relationships, not the document's.
type rels struct {
	list  []rel
	byKey map[string]string
}

func (r *rels) add(typ, target string, external bool) string {
	key := typ + "\x00" + target
	if id, ok := r.byKey[key]; ok {
		return id
	}
	if r.byKey == nil {
		r.byKey = map[string]string{}
	}
	id := "rId" + itoa(len(r.list)+1)
	r.list = append(r.list, rel{id: id, typ: typ, target: target, external: external})
	r.byKey[key] = id
	return id
}

func (r *rels) xml() string {
	var b strings.Builder
	b.WriteString(xmlHead + `<Relationships xmlns="` + nsRel + `">`)
	for _, x := range r.list {
		b.WriteString(`<Relationship Id="` + x.id + `" Type="` + x.typ + `" Target="` + attrEscaper.Replace(x.target) + `"`)
		if x.external {
			b.WriteString(` TargetMode="External"`)
		}
		b.WriteString(`/>`)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

type mediaFile struct {
	name string // under word/
	data []byte
}

type tocEntry struct {
	level    int
	text     string
	bookmark string
}

// numInst is one ordered list's numbering instance: it restarts at start on
// level ilvl.
type numInst struct{ ilvl, start int }

type writer struct {
	o   Options
	th  theme.Theme
	doc *markdown.Document

	out        *strings.Builder
	cur        *rels
	afterTable bool // the last thing written to out is a table
	inNote     bool

	// Page breaks (RFP §2, "Page breaks and placement"); Word paginates, so
	// they become paragraph properties.
	breakNext bool // the next paragraph starts a page
	hasBody   bool // something other than headings since the last forced break
	keep      bool // the block being written is kept with the next one

	warns    []markdown.Warning
	line     int
	ctrl     bool // a character XML cannot carry was removed
	ctrlLine int

	docRels   rels
	noteRels  rels
	media     []mediaFile
	mediaByPN map[string]string
	pics      int
	nums      []numInst // numId 1 is the bullets; ordered lists are 2, 3, ...
	heads     []tocEntry
	marks     int
	noteSeen  map[int]bool
}

func (w *writer) warn(code, msg string) {
	w.warns = append(w.warns, markdown.Warning{Line: w.line, Code: code, Message: msg})
}

// clean drops what XML cannot carry, remembering the first block it
// happened in.
func (w *writer) clean(s string) string {
	s, removed := clean(s)
	if removed && !w.ctrl {
		w.ctrl, w.ctrlLine = true, w.line
	}
	return s
}

// attr is s, cleaned, as an attribute value.
func (w *writer) attr(s string) string { return attrEscaper.Replace(w.clean(s)) }

func (w *writer) family(ps string) string {
	if w.o.Family == nil {
		return ps
	}
	return w.o.Family(ps)
}

// fixedTime stamps every zip entry so that the same input gives the same
// bytes.
var fixedTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Write returns the .docx bytes and the warnings it adds (not the parser's,
// which the caller already has).
func Write(doc *markdown.Document, o Options) (data []byte, warns []markdown.Warning, err error) {
	if doc == nil {
		doc = &markdown.Document{}
	}
	w := &writer{o: o, th: o.Theme, doc: doc, noteSeen: map[int]bool{}, mediaByPN: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			s, ok := r.(stop)
			if !ok {
				panic(r)
			}
			data, warns, err = nil, nil, s.err
		}
	}()

	hasNotes := len(doc.Footnotes) > 0
	w.docRels.add(relStyles, "styles.xml", false)
	w.docRels.add(relSettings, "settings.xml", false)
	w.docRels.add(relNumbering, "numbering.xml", false)
	if hasNotes {
		w.docRels.add(relFootnotes, "footnotes.xml", false)
	}
	w.line = 1 // the front matter heads the manuscript
	type hf struct{ part, kind, ref, xml string }
	var furniture []hf
	if o.Meta.Classification != "" {
		furniture = append(furniture, hf{"header1.xml", "default", "headerReference", w.header()})
	}
	furniture = append(furniture, hf{"footer1.xml", "default", "footerReference", w.footer()})
	if o.Cover {
		// titlePg alone would leave the cover's furniture to each reader's
		// reading of the spec; explicit empty parts say it outright.
		furniture = append(furniture, hf{"header2.xml", "first", "headerReference", emptyHdrFtr("hdr")},
			hf{"footer2.xml", "first", "footerReference", emptyHdrFtr("ftr")})
	}
	var refs strings.Builder
	for _, f := range furniture {
		typ := relFooter
		if f.ref == "headerReference" {
			typ = relHeader
		}
		id := w.docRels.add(typ, f.part, false)
		refs.WriteString(`<w:` + f.ref + ` w:type="` + f.kind + `" r:id="` + id + `"/>`)
	}

	// The body first: the table of contents lists what it found.
	var body strings.Builder
	w.out, w.cur = &body, &w.docRels
	w.blocks(doc.Children, ctx{top: true})
	w.endContainer()
	w.breakNext = false // a break at the very end has nothing to start

	var notes string
	if hasNotes {
		notes = w.footnotes()
		w.out, w.cur, w.afterTable = &body, &w.docRels, false
		w.orphanNotes()
		w.endContainer()
	}

	w.line = 1
	var front strings.Builder
	w.out, w.cur, w.afterTable = &front, &w.docRels, false
	// Page order as in the PDF: cover, table of contents, then the body,
	// which opens with the title block when there is no cover.
	if o.Cover {
		w.cover()
	}
	if o.TOC {
		w.toc()
	}
	if !o.Cover {
		w.titleBlock()
	}

	var d strings.Builder
	d.WriteString(xmlHead + `<w:document xmlns:w="` + nsW + `" xmlns:r="` + nsR + `" xmlns:wp="` + nsWP +
		`" xmlns:a="` + nsA + `" xmlns:pic="` + nsPic + `"><w:body>`)
	d.WriteString(front.String())
	d.WriteString(body.String())
	if front.Len()+body.Len() == 0 {
		d.WriteString(`<w:p/>`) // a body holds at least one paragraph
	}
	d.WriteString(w.sectPr(refs.String()))
	d.WriteString(`</w:body></w:document>`)

	core := w.core()
	if w.ctrl {
		w.warns = append(w.warns, markdown.Warning{Line: w.ctrlLine, Code: "control_characters",
			Message: "characters that a Word document cannot hold (control characters) were removed"})
	}

	type part struct {
		name, ctype string
		data        []byte
	}
	const ct = "application/vnd.openxmlformats-officedocument.wordprocessingml."
	parts := []part{
		{"docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml", []byte(core)},
		{"word/document.xml", ct + "document.main+xml", []byte(d.String())},
		{"word/styles.xml", ct + "styles+xml", []byte(w.styles())},
		{"word/settings.xml", ct + "settings+xml", []byte(w.settings(hasNotes))},
		{"word/numbering.xml", ct + "numbering+xml", []byte(w.numbering())},
	}
	if hasNotes {
		parts = append(parts, part{"word/footnotes.xml", ct + "footnotes+xml", []byte(notes)})
	}
	for _, f := range furniture {
		kind := "footer+xml"
		if f.ref == "headerReference" {
			kind = "header+xml"
		}
		parts = append(parts, part{"word/" + f.part, ct + kind, []byte(f.xml)})
	}

	var types strings.Builder
	types.WriteString(xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Default Extension="png" ContentType="image/png"/>`)
	for _, p := range parts {
		types.WriteString(`<Override PartName="/` + p.name + `" ContentType="` + p.ctype + `"/>`)
	}
	types.WriteString(`</Types>`)

	rootRels := rels{}
	rootRels.add(relDoc+"/officeDocument", "word/document.xml", false)
	rootRels.add("http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties", "docProps/core.xml", false)

	all := []part{{"[Content_Types].xml", "", []byte(types.String())}, {"_rels/.rels", "", []byte(rootRels.xml())}}
	all = append(all, parts...)
	all = append(all, part{"word/_rels/document.xml.rels", "", []byte(w.docRels.xml())})
	if len(w.noteRels.list) > 0 {
		all = append(all, part{"word/_rels/footnotes.xml.rels", "", []byte(w.noteRels.xml())})
	}
	for _, m := range w.media {
		all = append(all, part{"word/" + m.name, "", m.data})
	}

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, p := range all {
		// Every entry deflated, PNGs too: Go writes a stored entry with a data
		// descriptor, and LibreOffice refuses such a file ("source file could
		// not be loaded"; measured with LibreOffice 26.8).
		h := &zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: fixedTime}
		fw, err := z.CreateHeader(h)
		if err != nil {
			return nil, nil, fmt.Errorf("docx: %w", err)
		}
		if _, err := fw.Write(p.data); err != nil {
			return nil, nil, fmt.Errorf("docx: %w", err)
		}
	}
	if err := z.Close(); err != nil {
		return nil, nil, fmt.Errorf("docx: %w", err)
	}
	return buf.Bytes(), w.warns, nil
}

// core is docProps/core.xml: the title and author from the front matter and
// nothing else — no paths, no user names, no "last modified by" (RFP §2,
// metadata).
func (w *writer) core() string {
	var b strings.Builder
	b.WriteString(xmlHead + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">`)
	if t := w.clean(w.o.Meta.Title); t != "" {
		b.WriteString(`<dc:title>` + textEscaper.Replace(t) + `</dc:title>`)
	}
	if a := w.clean(w.o.Meta.Author); a != "" {
		b.WriteString(`<dc:creator>` + textEscaper.Replace(a) + `</dc:creator>`)
	}
	b.WriteString(`</cp:coreProperties>`)
	return b.String()
}

// settings holds the footnote separators' references (Word requires them
// when footnotes exist) and the compatibility mode, so that Word does not
// open the file in compatibility mode. updateFields is deliberately not set:
// it makes Word ask about updating fields on every open.
func (w *writer) settings(notes bool) string {
	var b strings.Builder
	b.WriteString(xmlHead + `<w:settings xmlns:w="` + nsW + `">`)
	b.WriteString(`<w:defaultTabStop w:val="` + itoa(tw(w.th.BodySize*4)) + `"/>`)
	if notes {
		b.WriteString(`<w:footnotePr><w:footnote w:id="-1"/><w:footnote w:id="0"/></w:footnotePr>`)
	}
	b.WriteString(`<w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat>`)
	b.WriteString(`</w:settings>`)
	return b.String()
}

// sectPr is the one section: page size and margins from the theme, the
// header and footer, and titlePg when a cover goes first.
func (w *writer) sectPr(refs string) string {
	th := w.th
	s := `<w:sectPr>` + refs +
		`<w:pgSz w:w="` + itoa(tw(th.PageW)) + `" w:h="` + itoa(tw(th.PageH)) + `"/>` +
		`<w:pgMar w:top="` + itoa(tw(th.MarginT)) + `" w:right="` + itoa(tw(th.MarginR)) +
		`" w:bottom="` + itoa(tw(th.MarginB)) + `" w:left="` + itoa(tw(th.MarginL)) +
		`" w:header="` + itoa(tw(th.MarginT/2)) + `" w:footer="` + itoa(tw(th.MarginB/2)) + `" w:gutter="0"/>`
	if w.o.Cover {
		s += `<w:titlePg/>`
	}
	return s + `</w:sectPr>`
}

func (w *writer) header() string {
	p := newPPr("Header")
	p.jc = "right"
	return xmlHead + `<w:hdr xmlns:w="` + nsW + `" xmlns:r="` + nsR + `">` +
		`<w:p>` + p.xml() + w.run(w.o.Meta.Classification, rpr{}) + `</w:p></w:hdr>`
}

// footer is "PAGE / NUMPAGES", centred. The fields carry a result so that a
// reader that does not compute them still shows something sensible.
func (w *writer) footer() string {
	field := func(instr string) string {
		return `<w:r><w:fldChar w:fldCharType="begin"/></w:r>` +
			`<w:r><w:instrText xml:space="preserve"> ` + instr + ` </w:instrText></w:r>` +
			`<w:r><w:fldChar w:fldCharType="separate"/></w:r>` +
			`<w:r><w:t>1</w:t></w:r>` +
			`<w:r><w:fldChar w:fldCharType="end"/></w:r>`
	}
	p := newPPr("Footer")
	p.jc = "center"
	return xmlHead + `<w:ftr xmlns:w="` + nsW + `" xmlns:r="` + nsR + `">` +
		`<w:p>` + p.xml() + field("PAGE") + `<w:r><w:t xml:space="preserve"> / </w:t></w:r>` + field("NUMPAGES") +
		`</w:p></w:ftr>`
}

func emptyHdrFtr(el string) string {
	return xmlHead + `<w:` + el + ` xmlns:w="` + nsW + `" xmlns:r="` + nsR + `"><w:p/></w:` + el + `>`
}
