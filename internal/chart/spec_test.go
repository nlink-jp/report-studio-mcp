package chart

import (
	"errors"
	"strings"
	"testing"
)

var lim = Limits{MaxPoints: 100, MaxSeries: 8}

func TestParseAcceptsEachType(t *testing.T) {
	for _, src := range []string{
		`{"type":"bar","x":{"categories":["a","b"]},"series":[{"values":[1,2]}]}`,
		`{"type":"bar","orientation":"horizontal","stacked":true,"x":{"categories":["a"]},"series":[{"name":"p","values":[1]},{"name":"q","values":[null]}]}`,
		`{"type":"line","x":{"categories":["1月","2月","3月"]},"y":{"label":"件数"},"series":[{"values":[3,null,5]}]}`,
		`{"type":"scatter","series":[{"points":[[1,2],[3,4]]}]}`,
	} {
		if _, _, err := Parse([]byte(src), lim); err != nil {
			t.Errorf("Parse(%s) = %v", src, err)
		}
	}
}

func TestParseCountsPoints(t *testing.T) {
	_, n, err := Parse([]byte(`{"type":"line","x":{"categories":["a","b","c"]},"series":[{"name":"p","values":[1,2,3]},{"name":"q","values":[1,null,3]}]}`), lim)
	if err != nil || n != 6 {
		t.Fatalf("points = %d, %v; want 6", n, err)
	}
}

func TestParseRefuses(t *testing.T) {
	cases := map[string]string{
		`{"type":"pie","series":[]}`: "unknown type",
		`{"series":[]}`:              `"type" is required`,
		`{"type":"bar","x":{"categories":["a"]},"series":[{"values":[1]}],"colour":"red"}`:                                                      "unknown field",
		`{"type":"bar","x":{"categories":["a"]},"series":[{"values":[1]}]} {}`:                                                                  "after the JSON object",
		`{"type":"bar","x":{"categories":["a","b"]},"series":[{"values":[1]}]}`:                                                                 "1 values for 2 categories",
		`{"type":"bar","x":{"categories":["a"]},"series":[{"values":[1]},{"values":[2]}]}`:                                                      "has no name",
		`{"type":"bar","x":{"categories":["a"]},"series":[{"name":"p","values":[1]},{"name":"p","values":[2]}]}`:                                "used twice",
		`{"type":"bar","stacked":true,"x":{"categories":["a"]},"series":[{"values":[-1]}]}`:                                                     "negative",
		`{"type":"bar","x":{"categories":["a"]},"y":{"min":1},"series":[{"values":[2]}]}`:                                                       "must include 0",
		`{"type":"line","x":{"categories":["a"]},"y":{"max":1},"series":[{"values":[2]}]}`:                                                      "above y.max",
		`{"type":"line","x":{"categories":["a"]},"y":{"min":3,"max":1},"series":[{"values":[2]}]}`:                                              "less than",
		`{"type":"line","orientation":"horizontal","x":{"categories":["a"]},"series":[{"values":[2]}]}`:                                         "bar charts only",
		`{"type":"scatter","x":{"categories":["a"]},"series":[{"points":[[1,2]]}]}`:                                                             "numeric",
		`{"type":"scatter","series":[{"values":[1]}]}`:                                                                                          `not "values"`,
		`{"type":"line","series":[{"values":[1]}]}`:                                                                                             "x.categories",
		`{"type":"bar","stacked":true,"x":{"categories":["a"]},"y":{"max":2},"series":[{"name":"p","values":[1.5]},{"name":"q","values":[1]}]}`: "above y.max",
	}
	cases[`{"type":"line","x":{"categories":["a"]},"y":{"min":3},"series":[{"values":[2]}]}`] = "below y.min"
	cases[`{"type":"scatter","x":{"min":0},"series":[{"points":[[-1,2]]}]}`] = "below x.min"
	for src, want := range cases {
		_, _, err := Parse([]byte(src), lim)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want error containing %q", src, err, want)
		}
	}
}

func TestParseCapsSeriesByWhatColoursTellApart(t *testing.T) {
	mk := func(typ string, n int) string {
		var b strings.Builder
		b.WriteString(`{"type":"` + typ + `",`)
		if typ != "scatter" {
			b.WriteString(`"x":{"categories":["a"]},`)
		}
		b.WriteString(`"series":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			if typ == "scatter" {
				b.WriteString(`{"name":"s` + string(rune('a'+i)) + `","points":[[1,1]]}`)
			} else {
				b.WriteString(`{"name":"s` + string(rune('a'+i)) + `","values":[1]}`)
			}
		}
		b.WriteString("]}")
		return b.String()
	}
	if _, _, err := Parse([]byte(mk("bar", 8)), lim); err != nil {
		t.Errorf("8 bar series: %v", err)
	}
	if _, _, err := Parse([]byte(mk("bar", 9)), lim); err == nil {
		t.Error("9 bar series accepted")
	}
	if _, _, err := Parse([]byte(mk("scatter", 3)), lim); err != nil {
		t.Errorf("3 scatter series: %v", err)
	}
	if _, _, err := Parse([]byte(mk("scatter", 4)), lim); err == nil {
		t.Error("4 scatter series accepted")
	}
}

func TestParseRefusesTooManyPoints(t *testing.T) {
	_, _, err := Parse([]byte(`{"type":"scatter","series":[{"points":[[1,1],[2,2],[3,3]]}]}`), Limits{MaxPoints: 2, MaxSeries: 8})
	if err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSyntaxErrorHasItsLine(t *testing.T) {
	_, _, err := Parse([]byte("{\n  \"type\": \"bar\",\n  \"series\": [1,,2]\n}"), lim)
	var e *Error
	if !errors.As(err, &e) || e.Line != 3 {
		t.Fatalf("err = %#v, want line 3", err)
	}
}

func TestTicksAndCompact(t *testing.T) {
	ts, dec := ticks(0, 87, 5)
	if ts[0] != 0 || ts[len(ts)-1] < 87 || dec != 0 {
		t.Errorf("ticks(0,87) = %v, %d", ts, dec)
	}
	ts, dec = ticks(0.1, 0.43, 5) // step 0.1
	if dec != 1 || ts[0] > 0.1 || ts[len(ts)-1] < 0.43 {
		t.Errorf("ticks(0.1,0.43) = %v, %d", ts, dec)
	}
	for v, want := range map[float64]string{0: "0", 950: "950", 1000: "1K", 1500: "1.5K", 2_500_000: "2.5M", -3000: "-3K"} {
		if got := compact(v, 0); got != want {
			t.Errorf("compact(%v) = %q, want %q", v, got, want)
		}
	}
}
