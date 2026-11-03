package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"path"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// pngOf is a small solid PNG.
func pngOf(t testing.TB, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

var (
	errMissing = errors.New("image not found")
	colorRed   = color.RGBA{0xd0, 0x30, 0x30, 0xff}
)

// call records what the fakes were asked.
type calls struct {
	images   []string
	chartW   []float64
	mermaids int
}

// fakeOptions are Options with fakes for every callback: images are 800 x
// 400 pt (wider than any column), "missing.png" fails; a mermaid source
// containing "bad" falls back; charts are as wide as asked and warn once.
func fakeOptions(t testing.TB, c *calls) Options {
	img := pngOf(t, 40, 20, color.RGBA{0x33, 0x66, 0xb3, 0xff})
	dia := pngOf(t, 30, 30, color.RGBA{0x10, 0x80, 0x10, 0xff})
	chartPNG := pngOf(t, 50, 25, color.RGBA{0xc0, 0x40, 0x40, 0xff})
	if c == nil {
		c = &calls{}
	}
	return Options{
		Theme:  theme.Standard(),
		Meta:   Meta{Title: "Report title", Author: "Author name"},
		Family: func(ps string) string { return "Fam-" + ps },
		Image: func(dest string, line int) (Picture, error) {
			c.images = append(c.images, dest)
			if dest == "missing.png" {
				return Picture{}, errMissing
			}
			return Picture{PNG: img, WidthPt: 800, HeightPt: 400}, nil
		},
		Mermaid: func(src string, line int) (Picture, string, error) {
			c.mermaids++
			if strings.Contains(src, "bad") {
				return Picture{}, "SyntaxError: the diagram cannot be read", nil
			}
			return Picture{PNG: dia, WidthPt: 200, HeightPt: 200}, "", nil
		},
		Chart: func(src string, line int, widthPt float64) (Picture, []string, error) {
			c.chartW = append(c.chartW, widthPt)
			return Picture{PNG: chartPNG, WidthPt: widthPt, HeightPt: widthPt / 2}, []string{"labels thinned"}, nil
		},
	}
}

func write(t testing.TB, md string, o Options) ([]byte, []markdown.Warning) {
	t.Helper()
	data, warns, err := Write(markdown.Parse([]byte(md)), o)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return data, warns
}

type pkg struct {
	names []string
	parts map[string][]byte
}

func unzipDocx(t testing.TB, data []byte) pkg {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	p := pkg{parts: map[string][]byte{}}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if _, dup := p.parts[f.Name]; dup {
			t.Fatalf("duplicate zip entry %s", f.Name)
		}
		p.names = append(p.names, f.Name)
		p.parts[f.Name] = b
	}
	return p
}

// xnode is a parsed XML element. Attribute keys are local names, except
// that relationship-namespace attributes are "r:<local>".
type xnode struct {
	space, name string
	attr        map[string]string
	kids        []*xnode
	text        string // character data directly inside
}

func parseXML(t testing.TB, name string, data []byte) *xnode {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(data))
	root := &xnode{}
	stack := []*xnode{root}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s is not well-formed: %v", name, err)
		}
		top := stack[len(stack)-1]
		switch v := tok.(type) {
		case xml.StartElement:
			n := &xnode{space: v.Name.Space, name: v.Name.Local, attr: map[string]string{}}
			for _, a := range v.Attr {
				k := a.Name.Local
				if a.Name.Space == nsR {
					k = "r:" + k
				}
				n.attr[k] = a.Value
			}
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			top.text += string(v)
		}
	}
	if len(stack) != 1 || len(root.kids) != 1 {
		t.Fatalf("%s: expected one root element", name)
	}
	return root.kids[0]
}

// all returns every descendant element (and n itself) with that local name.
func (n *xnode) all(name string) []*xnode {
	var out []*xnode
	var walk func(*xnode)
	walk = func(x *xnode) {
		if x.name == name {
			out = append(out, x)
		}
		for _, k := range x.kids {
			walk(k)
		}
	}
	walk(n)
	return out
}

func (n *xnode) first(name string) *xnode {
	if a := n.all(name); len(a) > 0 {
		return a[0]
	}
	return nil
}

// textOf is the text of w:t elements under n, with w:tab as a tab.
func (n *xnode) textOf() string {
	var b strings.Builder
	var walk func(*xnode)
	walk = func(x *xnode) {
		switch x.name {
		case "t":
			b.WriteString(x.text)
		case "tab":
			if x.space == nsW && len(x.attr) == 0 {
				b.WriteString("\t")
			}
		}
		for _, k := range x.kids {
			walk(k)
		}
	}
	walk(n)
	return b.String()
}

// para is a paragraph as the tests look at it.
type para struct {
	style       string
	numID, ilvl string
	text        string
	n           *xnode
}

func paragraphs(t testing.TB, data []byte) []para {
	t.Helper()
	doc := parseXML(t, "document", data)
	var out []para
	for _, p := range doc.all("p") {
		x := para{n: p, text: p.textOf()}
		if ppr := firstChild(p, "pPr"); ppr != nil {
			if s := firstChild(ppr, "pStyle"); s != nil {
				x.style = s.attr["val"]
			}
			if np := firstChild(ppr, "numPr"); np != nil {
				x.numID = firstChild(np, "numId").attr["val"]
				x.ilvl = firstChild(np, "ilvl").attr["val"]
			}
		}
		out = append(out, x)
	}
	return out
}

func firstChild(n *xnode, name string) *xnode {
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

func findPara(t testing.TB, ps []para, text string) para {
	t.Helper()
	for _, p := range ps {
		if p.text == text {
			return p
		}
	}
	t.Fatalf("no paragraph with text %q", text)
	return para{}
}

// checkPackage asserts what every .docx must be: every part well-formed XML,
// [Content_Types].xml covering every part, every internal relationship
// target present, every r:id used by a part declared in its relationships,
// and external relationships only for clickable hyperlinks.
func checkPackage(t testing.TB, data []byte) pkg {
	t.Helper()
	p := unzipDocx(t, data)
	for _, name := range p.names {
		if !strings.HasSuffix(name, ".png") {
			parseXML(t, name, p.parts[name])
		}
	}
	ct, ok := p.parts["[Content_Types].xml"]
	if !ok {
		t.Fatal("no [Content_Types].xml")
	}
	types := parseXML(t, "[Content_Types].xml", ct)
	defaults, overrides := map[string]bool{}, map[string]bool{}
	for _, d := range types.all("Default") {
		defaults[strings.ToLower(d.attr["Extension"])] = true
	}
	for _, o := range types.all("Override") {
		name := strings.TrimPrefix(o.attr["PartName"], "/")
		if _, ok := p.parts[name]; !ok {
			t.Errorf("content type override for missing part %s", name)
		}
		overrides[name] = true
	}
	for _, name := range p.names {
		if name == "[Content_Types].xml" || overrides[name] {
			continue
		}
		if !defaults[strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))] {
			t.Errorf("part %s has no content type", name)
		}
	}
	for _, name := range p.names {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		dir := path.Dir(path.Dir(name)) // word/_rels/x.rels → word
		source := path.Join(dir, strings.TrimSuffix(path.Base(name), ".rels"))
		ids := map[string]bool{}
		for _, r := range parseXML(t, name, p.parts[name]).all("Relationship") {
			ids[r.attr["Id"]] = true
			target := r.attr["Target"]
			if r.attr["TargetMode"] == "External" {
				if !strings.HasSuffix(r.attr["Type"], "/hyperlink") {
					t.Errorf("%s: external relationship of type %s", name, r.attr["Type"])
				}
				scheme, _, _ := strings.Cut(target, ":")
				if s := strings.ToLower(scheme); s != "http" && s != "https" && s != "mailto" {
					t.Errorf("%s: external hyperlink with scheme %q: %s", name, scheme, target)
				}
				continue
			}
			full := path.Join(dir, target)
			if dir == "." {
				full = target
			}
			if _, ok := p.parts[full]; !ok {
				t.Errorf("%s: relationship %s targets missing part %s", name, r.attr["Id"], full)
			}
		}
		if src, ok := p.parts[source]; ok {
			var used func(*xnode)
			used = func(n *xnode) {
				for k, v := range n.attr {
					if strings.HasPrefix(k, "r:") && !ids[v] {
						t.Errorf("%s uses %s=%q, not in %s", source, k, v, name)
					}
				}
				for _, k := range n.kids {
					used(k)
				}
			}
			used(parseXML(t, source, src))
		}
	}
	// A part that uses r: attributes must have relationships.
	for _, name := range p.names {
		if !strings.HasSuffix(name, ".xml") || strings.Contains(name, "_rels") {
			continue
		}
		relsName := path.Join(path.Dir(name), "_rels", path.Base(name)+".rels")
		if _, ok := p.parts[relsName]; ok {
			continue
		}
		var walk func(*xnode)
		walk = func(n *xnode) {
			for k := range n.attr {
				if strings.HasPrefix(k, "r:") {
					t.Errorf("%s uses %s but has no relationships part", name, k)
				}
			}
			for _, k := range n.kids {
				walk(k)
			}
		}
		walk(parseXML(t, name, p.parts[name]))
	}
	return p
}
