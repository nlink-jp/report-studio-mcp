package fsio_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/fsio"
	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

// With the real pathguard floor behind the workdir adapter, a credential file
// inside the opened directory is refused for reading and writing — the floor
// still matters when the work directory is a home.
func TestRealFloorRefusesCredentials(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".ssh/id_rsa", filepath.Join(home, "chart.png")); err != nil {
		t.Fatal(err)
	}

	d, err := fsio.Open(home, workdir.NewResolverFor(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	for _, rel := range []string{".ssh/id_rsa", "chart.png"} {
		_, err := d.ReadFile(rel, 1<<10)
		var te *toolerr.Error
		if !errors.As(err, &te) || te.Code != toolerr.CodePathNotAllowed {
			t.Errorf("ReadFile(%q) = %v, want path_not_allowed", rel, err)
		}
	}
	_, err = d.WriteFile(".ssh/report.pdf", []byte("x"))
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != toolerr.CodePathNotAllowed {
		t.Errorf("WriteFile(.ssh/report.pdf) = %v, want path_not_allowed", err)
	}
}
