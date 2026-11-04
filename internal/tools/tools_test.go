package tools

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGetUsageReturnsTheReference(t *testing.T) {
	out, err := getUsage(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := out.(string)
	if !ok || !strings.Contains(s, "report-studio-mcp") {
		t.Errorf("get_usage returned %#v", out)
	}
}

// Every warning and error code the server can produce is in get_usage: a
// model can only act on what the reference names.
func TestUsageNamesEveryCode(t *testing.T) {
	for code := range serverCodes(t) {
		if !strings.Contains(usage, code) {
			t.Errorf("get_usage does not name %s", code)
		}
	}
}

// serverCodes reads every warning and error code the server's own source can
// produce: the codes as written where they are raised, not as copied.
func serverCodes(t *testing.T) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`(?:warn\(|collect\(|stopErr\(|Code:\s*|Code[A-Za-z]+\s*=\s*|toolerr\.New\(|toolerr\.Newf\(|c\.warn\([^,]+,\s*|\.fail\([^,]+,\s*)"([a-z]+(?:_[a-z]+)+)"`)
	var files []string
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			seen[m[1]] = true
		}
	}
	if len(seen) < 20 {
		t.Fatalf("found only %d codes: %v", len(seen), seen)
	}
	return seen
}
