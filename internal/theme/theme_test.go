package theme

import "testing"

func TestBuiltins(t *testing.T) {
	for _, n := range BuiltinNames() {
		th, ok := Builtin(n)
		if !ok || th.ContentW() <= 0 || th.BodyH() <= 0 || th.BodySize <= 0 || len(th.Chart.Palette) != 8 {
			t.Errorf("%s: %+v", n, th)
		}
	}
	if _, ok := Builtin("fancy"); ok {
		t.Error("unknown name accepted")
	}
	s, f, c := Standard(), Formal(), Compact()
	if s.Cover || s.TOC || !f.Cover || !f.TOC || c.Cover || c.TOC {
		t.Error("cover/toc defaults wrong")
	}
	if f.Body == s.Body || f.ContentW() >= s.ContentW() || c.ContentW() <= s.ContentW() || c.BodySize >= s.BodySize {
		t.Error("profiles do not differ as described")
	}
}
