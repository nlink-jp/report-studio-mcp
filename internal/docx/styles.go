package docx

import (
	"strings"
)

// styles is styles.xml: the profile as Word styles, so that the receiver can
// restyle the document in Word. Built-in styles keep Word's own style names
// ("heading 1", "toc 1", "footnote text", ...) so Word treats them as its
// own (navigation pane, table of contents, footnote insertion).
func (w *writer) styles() string {
	th := w.th
	body, heading, code := w.family(th.Body), w.family(th.Heading), w.family(th.Code)
	text, muted := hexRGB(th.Text), hexRGB(th.TextMuted)
	sz := func(pt float64) string {
		v := itoa(hp(pt))
		return `<w:sz w:val="` + v + `"/><w:szCs w:val="` + v + `"/>`
	}
	spacing := func(before, after float64, gap float64) string {
		return `<w:spacing w:before="` + itoa(tw(before)) + `" w:after="` + itoa(tw(after)) +
			`" w:line="` + itoa(lineAuto(gap)) + `" w:lineRule="auto"/>`
	}
	var b strings.Builder
	style := func(typ, id, name, basedOn, next string, qf bool, ppr, rpr string, extra ...string) {
		b.WriteString(`<w:style w:type="` + typ + `" w:styleId="` + id + `"><w:name w:val="` + name + `"/>`)
		if basedOn != "" {
			b.WriteString(`<w:basedOn w:val="` + basedOn + `"/>`)
		}
		if next != "" {
			b.WriteString(`<w:next w:val="` + next + `"/>`)
		}
		if qf {
			b.WriteString(`<w:qFormat/>`)
		}
		if ppr != "" {
			b.WriteString(`<w:pPr>` + ppr + `</w:pPr>`)
		}
		if rpr != "" {
			b.WriteString(`<w:rPr>` + rpr + `</w:rPr>`)
		}
		for _, e := range extra {
			b.WriteString(e)
		}
		b.WriteString(`</w:style>`)
	}

	b.WriteString(xmlHead + `<w:styles xmlns:w="` + nsW + `" xmlns:r="` + nsR + `">`)
	b.WriteString(`<w:docDefaults><w:rPrDefault><w:rPr>` + rFonts(body, "") +
		`<w:color w:val="` + text + `"/>` + sz(th.BodySize) + `<w:lang w:val="en-US" w:eastAsia="ja-JP"/>` +
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>`)

	b.WriteString(`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/>` +
		`<w:pPr>` + spacing(0, th.BodySize*0.75, th.LineGap) + `</w:pPr></w:style>`)
	b.WriteString(`<w:style w:type="character" w:default="1" w:styleId="DefaultParagraphFont"><w:name w:val="Default Paragraph Font"/><w:uiPriority w:val="1"/><w:semiHidden/><w:unhideWhenUsed/></w:style>`)
	b.WriteString(`<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:uiPriority w:val="99"/><w:semiHidden/><w:unhideWhenUsed/>` +
		`<w:tblPr><w:tblInd w:w="0" w:type="dxa"/><w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>`)
	b.WriteString(`<w:style w:type="numbering" w:default="1" w:styleId="NoList"><w:name w:val="No List"/><w:uiPriority w:val="99"/><w:semiHidden/><w:unhideWhenUsed/></w:style>`)

	// Headings name a weight of a family by PostScript name (W6); Word picks
	// a weight only through bold, so the heading styles are bold.
	for i, size := range th.HeadingSizes {
		n := itoa(i + 1)
		style("paragraph", "Heading"+n, "heading "+n, "Normal", "Normal", true,
			`<w:keepNext/><w:keepLines/>`+spacing(size*0.9, size*0.5, 0.3)+`<w:outlineLvl w:val="`+itoa(i)+`"/>`,
			rFonts(heading, "")+`<w:b/><w:bCs/>`+sz(size))
	}
	style("paragraph", "Title", "Title", "Normal", "Normal", true,
		spacing(0, th.TitleSize*0.4, 0.25), rFonts(heading, "")+`<w:b/><w:bCs/>`+sz(th.TitleSize))
	style("paragraph", "Subtitle", "Subtitle", "Normal", "Normal", true,
		spacing(0, th.SubtitleSize*0.4, 0.25), `<w:color w:val="`+muted+`"/>`+sz(th.SubtitleSize))
	// The contents title looks like a heading but has body outline level, so
	// it never lists itself.
	style("paragraph", "TOCHeading", "TOC Heading", "Heading1", "Normal", true, `<w:outlineLvl w:val="9"/>`, "")
	// The right tab with a dot leader is where Word and Pages put the page
	// numbers when they regenerate the table (phase 0, P0-2: without it Pages
	// glued the numbers to the entries).
	tab := `<w:tabs><w:tab w:val="right" w:leader="dot" w:pos="` + itoa(tw(th.ContentW())) + `"/></w:tabs>`
	for n := 1; n <= 3; n++ {
		rp := ""
		if n == 1 {
			rp = `<w:b/><w:bCs/>`
		}
		style("paragraph", "TOC"+itoa(n), "toc "+itoa(n), "Normal", "Normal", false,
			tab+`<w:spacing w:after="`+itoa(tw(th.BodySize*0.45))+`"/><w:ind w:left="`+itoa(tw(float64(n-1)*th.BodySize*1.5))+`"/>`, rp)
	}
	style("paragraph", "Quote", "Quote", "Normal", "Normal", true,
		`<w:pBdr>`+border("left", 2, w.barSpace(), hexRGB(th.QuoteBar))+`</w:pBdr><w:ind w:left="`+itoa(w.quoteIndent())+`"/>`,
		`<w:color w:val="`+muted+`"/>`)
	cb := hexRGB(th.CodeBG)
	// Borders in the background colour pad the shading around the text.
	style("paragraph", "CodeBlock", "Code Block", "Normal", "", true,
		`<w:pBdr>`+border("top", 0.5, codePad, cb)+border("left", 0.5, codePad, cb)+border("bottom", 0.5, codePad, cb)+border("right", 0.5, codePad, cb)+`</w:pBdr>`+
			`<w:shd w:val="clear" w:color="auto" w:fill="`+cb+`"/><w:spacing w:after="0" w:line="`+itoa(lineAuto(0.25))+`" w:lineRule="auto"/>`+
			`<w:ind w:left="`+itoa(tw(codePad+1))+`" w:right="`+itoa(tw(codePad+1))+`"/>`,
		rFonts(code, body)+sz(th.CodeSize))
	style("paragraph", "Caption", "caption", "Normal", "Normal", true,
		spacing(0, th.BodySize*0.75, 0.3), `<w:color w:val="`+muted+`"/>`+sz(th.NoteSize))
	style("paragraph", "FootnoteText", "footnote text", "Normal", "", false,
		spacing(0, th.NoteSize*0.3, 0.3), sz(th.NoteSize))
	style("paragraph", "TableText", "Table Text", "Normal", "", false,
		`<w:spacing w:after="0" w:line="`+itoa(lineAuto(0.25))+`" w:lineRule="auto"/>`, sz(th.TableSize))
	style("paragraph", "Header", "header", "Normal", "", false,
		`<w:spacing w:after="0"/>`, `<w:color w:val="`+muted+`"/>`+sz(th.NoteSize))
	style("paragraph", "Footer", "footer", "Normal", "", false,
		`<w:spacing w:after="0"/>`, `<w:color w:val="`+muted+`"/>`+sz(th.NoteSize))
	style("character", "FootnoteReference", "footnote reference", "DefaultParagraphFont", "", false,
		"", `<w:vertAlign w:val="superscript"/>`)
	style("character", "Hyperlink", "Hyperlink", "DefaultParagraphFont", "", false,
		"", `<w:color w:val="`+hexRGB(th.Link)+`"/><w:u w:val="single"/>`)
	style("character", "CodeChar", "Inline Code", "DefaultParagraphFont", "", false,
		"", rFonts(code, body)+`<w:shd w:val="clear" w:color="auto" w:fill="`+cb+`"/>`)
	style("table", "ReportTable", "Report Table", "TableNormal", "", false, "", "",
		`<w:tblPr>`+w.tableBorders()+w.cellMargins()+`</w:tblPr>`,
		`<w:tblStylePr w:type="firstRow"><w:rPr><w:b/><w:bCs/></w:rPr><w:tcPr><w:tcBorders>`+border("bottom", 0.75, 0, hexRGB(th.Rule))+
			`</w:tcBorders><w:shd w:val="clear" w:color="auto" w:fill="`+hexRGB(th.TableHeadBG)+`"/></w:tcPr></w:tblStylePr>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

// numbering is numbering.xml: one definition for bullets (• ◦ ▪ by level,
// as the PDF draws them) and one for decimals, the bullet instance, and one
// instance per ordered list that restarts its level at the list's first
// number (Word continues a definition's numbering across instances unless
// told to restart).
func (w *writer) numbering() string {
	step := w.listStep()
	var b strings.Builder
	b.WriteString(xmlHead + `<w:numbering xmlns:w="` + nsW + `">`)
	abstract := func(id int, fmtv func(ilvl int) (numFmt, text string)) {
		b.WriteString(`<w:abstractNum w:abstractNumId="` + itoa(id) + `"><w:multiLevelType w:val="multilevel"/>`)
		for i := 0; i < 9; i++ {
			f, t := fmtv(i)
			b.WriteString(`<w:lvl w:ilvl="` + itoa(i) + `"><w:start w:val="1"/><w:numFmt w:val="` + f + `"/>` +
				`<w:lvlText w:val="` + t + `"/><w:lvlJc w:val="left"/>` +
				`<w:pPr><w:ind w:left="` + itoa(step*(i+1)) + `" w:hanging="` + itoa(step) + `"/></w:pPr></w:lvl>`)
		}
		b.WriteString(`</w:abstractNum>`)
	}
	bullets := []string{"•", "◦", "▪"}
	abstract(0, func(i int) (string, string) { return "bullet", bullets[i%len(bullets)] })
	abstract(1, func(i int) (string, string) { return "decimal", "%" + itoa(i+1) + "." })
	b.WriteString(`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`)
	for i, n := range w.nums {
		b.WriteString(`<w:num w:numId="` + itoa(i+2) + `"><w:abstractNumId w:val="1"/>` +
			`<w:lvlOverride w:ilvl="` + itoa(n.ilvl) + `"><w:startOverride w:val="` + itoa(n.start) + `"/></w:lvlOverride></w:num>`)
	}
	b.WriteString(`</w:numbering>`)
	return b.String()
}
