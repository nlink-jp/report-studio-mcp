package chart

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// fixed measures ASCII at 0.55 em and everything else at a full em.
type fixed struct{}

func (fixed) Width(s string, size float64, bold bool) float64 {
	w := 0.0
	for _, r := range s {
		if r < 128 {
			w += 0.55
		} else {
			w += 1
		}
	}
	if bold {
		w *= 1.05
	}
	return w * size
}

type box struct{ x0, y0, x1, y1 float64 }

func (b box) overlaps(o box) bool {
	const eps = 0.01
	return b.x0 < o.x1-eps && o.x0 < b.x1-eps && b.y0 < o.y1-eps && o.y0 < b.y1-eps
}

// textBox is the box a text's glyphs can occupy.
func textBox(t Text) box {
	w := fixed{}.Width(t.S, t.Size, t.Bold)
	if t.Vertical {
		var y0 float64
		switch t.Anchor {
		case Start:
			y0 = t.Y - w
		case Middle:
			y0 = t.Y - w/2
		default:
			y0 = t.Y
		}
		return box{t.X - t.Size*0.8, y0, t.X + t.Size*0.25, y0 + w}
	}
	var x0 float64
	switch t.Anchor {
	case Start:
		x0 = t.X
	case Middle:
		x0 = t.X - w/2
	default:
		x0 = t.X - w
	}
	return box{x0, t.Y - t.Size*0.8, x0 + w, t.Y + t.Size*0.25}
}

// checkFigure asserts the properties every figure must have.
func checkFigure(t *testing.T, name string, s *Spec, f *Figure) {
	t.Helper()
	st := DefaultStyle()
	in := func(b box) bool { return b.x0 >= -0.01 && b.y0 >= -0.01 && b.x1 <= f.W+0.01 && b.y1 <= f.H+0.01 }
	var texts []box
	var bars []box
	series := map[Color]bool{}
	for _, c := range st.Palette[:len(s.Series)] {
		series[c] = true
	}
	legendSwatches := 0
	for _, it := range f.Items {
		switch v := it.(type) {
		case Text:
			b := textBox(v)
			if !in(b) {
				t.Errorf("%s: text %q at %+v outside the %vx%v figure", name, v.S, b, f.W, f.H)
			}
			if series[v.Color] {
				t.Errorf("%s: text %q wears a series colour", name, v.S)
			}
			texts = append(texts, b)
		case Rect:
			b := box{v.X, v.Y, v.X + v.W, v.Y + v.H}
			if !in(b) || v.W <= 0 || v.H <= 0 {
				t.Errorf("%s: rect %+v outside the figure or empty", name, v)
			}
			// The two rounded corners share the rounded side; the radius also
			// fits the bar's length.
			across, along := v.W, v.H
			if v.RoundSide == "left" || v.RoundSide == "right" {
				across, along = v.H, v.W
			}
			if v.Round > across/2+1e-9 || v.Round > along+1e-9 {
				t.Errorf("%s: rect rounding %v too large for %vx%v (%s)", name, v.Round, v.W, v.H, v.RoundSide)
			}
			if v.W == v.H && v.Round == 0 && v.W < st.FontSize { // a legend swatch
				legendSwatches++
			} else {
				bars = append(bars, b)
			}
		case Line:
			b := box{math.Min(v.X1, v.X2), math.Min(v.Y1, v.Y2), math.Max(v.X1, v.X2), math.Max(v.Y1, v.Y2)}
			if !in(b) {
				t.Errorf("%s: line %+v outside the figure", name, v)
			}
		case Polyline:
			for _, p := range v.Points {
				if !in(box{p[0], p[1], p[0], p[1]}) {
					t.Errorf("%s: polyline point %v outside the figure", name, p)
				}
			}
		case Circle:
			if !in(box{v.X - v.R - v.Ring, v.Y - v.R - v.Ring, v.X + v.R + v.Ring, v.Y + v.R + v.Ring}) {
				t.Errorf("%s: circle %+v outside the figure", name, v)
			}
		}
	}
	for i := range texts {
		for j := i + 1; j < len(texts); j++ {
			if texts[i].overlaps(texts[j]) {
				t.Errorf("%s: texts overlap: %+v and %+v", name, texts[i], texts[j])
			}
		}
	}
	for i := range bars {
		for j := i + 1; j < len(bars); j++ {
			if bars[i].overlaps(bars[j]) {
				t.Errorf("%s: bars overlap: %+v and %+v", name, bars[i], bars[j])
			}
		}
	}
	if s.Type != "line" {
		want := 0
		if len(s.Series) >= 2 {
			want = len(s.Series)
		}
		if legendSwatches != want {
			t.Errorf("%s: %d legend swatches, want %d", name, legendSwatches, want)
		}
	}
}

func layoutSrc(t *testing.T, src string) (*Spec, *Figure) {
	t.Helper()
	s, _, err := Parse([]byte(src), Limits{MaxPoints: 10000, MaxSeries: 8})
	if err != nil {
		t.Fatal(err)
	}
	return s, Layout(s, 481.9, fixed{}, DefaultStyle())
}

func TestLayoutExamples(t *testing.T) {
	for name, src := range map[string]string{
		"grouped": `{"type":"bar","title":"宛先別の件数","x":{"categories":["社内","社外","不明"]},"y":{"label":"件"},
			"series":[{"name":"9月","values":[1204,731,12]},{"name":"10月","values":[1100,802,0]}]}`,
		"negative": `{"type":"bar","x":{"categories":["a","b"]},"series":[{"values":[-5,8]}]}`,
		"stacked":  `{"type":"bar","stacked":true,"x":{"categories":["a","b"]},"series":[{"name":"p","values":[3,1]},{"name":"q","values":[2,null]}]}`,
		"hbar":     `{"type":"bar","orientation":"horizontal","x":{"categories":["とても長いカテゴリ名がここに入るので折り返しが必要になる","短い"]},"series":[{"values":[5,9]}]}`,
		"line":     `{"type":"line","x":{"categories":["1","2","3","4"]},"series":[{"name":"a","values":[1,null,3,4]},{"name":"b","values":[2,2,null,1]}]}`,
		"scatter":  `{"type":"scatter","x":{"label":"x"},"y":{"label":"y"},"series":[{"name":"a","points":[[0,0],[10,5]]},{"name":"b","points":[[3,9]]}]}`,
		"crowded":  `{"type":"bar","x":{"categories":[` + cats(120) + `]},"series":[{"values":[` + vals(120) + `]}]}`,
	} {
		s, f := layoutSrc(t, src)
		checkFigure(t, name, s, f)
	}
}

func TestCrowdedLabelsWarn(t *testing.T) {
	_, f := layoutSrc(t, `{"type":"bar","x":{"categories":[`+cats(400)+`]},"series":[{"values":[`+vals(400)+`]}]}`)
	if len(f.Warnings) == 0 {
		t.Error("400 categories drawn without a thinning warning")
	}
}

func TestSeriesColoursFollowTheFixedOrder(t *testing.T) {
	_, f := layoutSrc(t, `{"type":"bar","x":{"categories":["a"]},"series":[{"name":"p","values":[1]},{"name":"q","values":[2]},{"name":"r","values":[3]}]}`)
	pal := DefaultStyle().Palette
	var got []Color
	for _, it := range f.Items {
		if r, ok := it.(Rect); ok && r.Round > 0 {
			got = append(got, r.Fill)
		}
	}
	for i, c := range got {
		if c != pal[i] {
			t.Errorf("bar %d colour %v, want palette slot %d %v", i, c, i, pal[i])
		}
	}
}

// Random specs of every shape keep every property.
func TestLayoutProperties(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 400; i++ {
		src := randomSpec(r)
		s, _, err := Parse([]byte(src), Limits{MaxPoints: 10000, MaxSeries: 8})
		if err != nil {
			t.Fatalf("seed spec %d invalid: %v\n%s", i, err, src)
		}
		checkFigure(t, fmt.Sprintf("random %d", i), s, Layout(s, 200+r.Float64()*300, fixed{}, DefaultStyle()))
	}
}

func cats(n int) string {
	b := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			b += ","
		}
		b += fmt.Sprintf(`"c%d"`, i)
	}
	return b
}

func vals(n int) string {
	b := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			b += ","
		}
		b += fmt.Sprint(i % 17)
	}
	return b
}

func randomSpec(r *rand.Rand) string {
	types := []string{"bar", "bar", "line", "scatter"}
	typ := types[r.Intn(len(types))]
	spec := map[string]any{"type": typ}
	if r.Intn(2) == 0 {
		spec["title"] = []string{"件数", "A long English title that may need wrapping across the width", "月別の推移と内訳"}[r.Intn(3)]
	}
	maxS := 8
	if typ == "scatter" {
		maxS = 3
	}
	ns := 1 + r.Intn(maxS)
	stacked := typ == "bar" && r.Intn(3) == 0
	if typ == "bar" {
		spec["stacked"] = stacked
		if r.Intn(2) == 0 {
			spec["orientation"] = "horizontal"
		}
	}
	words := []string{"社内", "社外", "Tokyo", "大阪支社の第二営業部", "x", "2026-10", "不明な宛先", strings.Repeat("長い名前", 40)}
	var series []map[string]any
	if typ == "scatter" {
		for s := 0; s < ns; s++ {
			var pts [][2]float64
			for p := 0; p < 1+r.Intn(50); p++ {
				pts = append(pts, [2]float64{r.NormFloat64() * 100, r.Float64() * 1e4})
			}
			series = append(series, map[string]any{"name": fmt.Sprint("系列", s), "points": pts})
		}
	} else {
		nc := 1 + r.Intn(60)
		var cs []string
		for c := 0; c < nc; c++ {
			cs = append(cs, fmt.Sprint(words[r.Intn(len(words))], c))
		}
		spec["x"] = map[string]any{"categories": cs}
		scale := math.Pow(10, float64(r.Intn(7)-2))
		for s := 0; s < ns; s++ {
			var vs []any
			for c := 0; c < nc; c++ {
				switch {
				case r.Intn(10) == 0:
					vs = append(vs, nil)
				case stacked:
					vs = append(vs, r.Float64()*scale)
				default:
					vs = append(vs, (r.Float64()-0.2)*scale)
				}
			}
			series = append(series, map[string]any{"name": fmt.Sprint("系列", s), "values": vs})
		}
	}
	spec["series"] = series
	switch r.Intn(4) {
	case 0:
		spec["y"] = map[string]any{"label": "値"}
	case 1:
		spec["y"] = map[string]any{"label": strings.Repeat("とても長いラベル", 20)}
	}
	b, _ := json.Marshal(spec)
	return string(b)
}

// Specs from the independent review that crashed the server or never
// finished: every one now either draws inside its figure or is refused, and
// does so promptly.
func TestDegenerateRangesFinish(t *testing.T) {
	accept := []string{
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[null,null]}],"y":{"min":5}}`,
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[null,null]}],"y":{"max":-5}}`,
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[1e15,1e15]}]}`,
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[1e14,100000000000000.02]}]}`,
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[5e-324,1e-323]}]}`,
		`{"type":"bar","x":{"categories":["a"]},"series":[{"values":[0]}]}`,
		`{"type":"scatter","series":[{"points":[[3,3]]}]}`,
		`{"type":"bar","x":{"categories":["a","b","c"]},"series":[{"values":[-1e15,1e15,0]}]}`,
	}
	refuse := []string{
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[1e308,-1e308]}]}`,
		`{"type":"line","x":{"categories":["a","b"]},"series":[{"values":[1e16,1.0000000000000002e16]}]}`,
		`{"type":"bar","x":{"categories":["a"]},"series":[{"values":[1.7e308]}]}`,
		`{"type":"line","x":{"categories":["a"]},"y":{"max":1e300},"series":[{"values":[1]}]}`,
	}
	run := func(src string, wantErr bool) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			s, _, err := Parse([]byte(src), Limits{MaxPoints: 100, MaxSeries: 8})
			if wantErr {
				if err == nil {
					t.Errorf("accepted: %s", src)
				}
				return
			}
			if err != nil {
				t.Errorf("refused %s: %v", src, err)
				return
			}
			for _, w := range []float64{20, 120, 481.9} {
				checkFigure(t, src, s, Layout(s, w, fixed{}, DefaultStyle()))
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("did not finish: %s", src)
		}
	}
	for _, s := range accept {
		run(s, false)
	}
	for _, s := range refuse {
		run(s, true)
	}
}

func TestLongLabelsStayInside(t *testing.T) {
	long := strings.Repeat("とても長い軸のラベル", 19) // 190 characters
	for _, src := range []string{
		`{"type":"bar","title":"` + long + `","x":{"label":"` + long + `","categories":["a","b"]},"y":{"label":"` + long + `"},"series":[{"name":"` + long + `","values":[1,2]},{"name":"短い","values":[2,1]}]}`,
		`{"type":"bar","orientation":"horizontal","x":{"label":"` + long + `","categories":["` + long + `","b"]},"y":{"label":"` + long + `"},"series":[{"values":[1,2]}]}`,
		`{"type":"scatter","x":{"label":"` + long + `"},"y":{"label":"` + long + `"},"series":[{"name":"` + long + `","points":[[1,2]]},{"name":"b","points":[[2,1]]}]}`,
	} {
		s, _, err := Parse([]byte(src), Limits{MaxPoints: 100, MaxSeries: 8})
		if err != nil {
			t.Fatal(err)
		}
		checkFigure(t, "long labels", s, Layout(s, 481.9, fixed{}, DefaultStyle()))
	}
	_, _, err := Parse([]byte(`{"type":"bar","x":{"categories":["`+strings.Repeat("x", 201)+`"]},"series":[{"values":[1]}]}`), Limits{MaxPoints: 10, MaxSeries: 8})
	if err == nil || !strings.Contains(err.Error(), "at most 200") {
		t.Errorf("a 201-character category: %v", err)
	}
}
