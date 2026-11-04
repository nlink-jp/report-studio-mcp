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
	re := regexp.MustCompile(`(?:warn\(|Code:\s*|toolerr\.New\(|toolerr\.Newf\(|c\.warn\([^,]+,\s*)"([a-z]+(?:_[a-z]+)+)"`)
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
	for code := range seen {
		if !strings.Contains(usage, code) {
			t.Errorf("get_usage does not name %s", code)
		}
	}
}
