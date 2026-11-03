package toolerr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
)

func TestErrorString(t *testing.T) {
	e := toolerr.New(toolerr.CodeWorkspaceFailed, "cannot create out/")
	if got := e.Error(); got != "workspace_failed: cannot create out/" {
		t.Errorf("got %q", got)
	}
}

func TestErrorIsByCode(t *testing.T) {
	sentinel := toolerr.New(toolerr.CodeWorkspaceFailed, "")
	actual := toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot create %s", "out/")
	if !errors.Is(actual, sentinel) {
		t.Errorf("errors.Is should match by Code")
	}
	other := toolerr.New(toolerr.CodeMissingArgument, "")
	if errors.Is(actual, other) {
		t.Errorf("errors.Is should not match a different Code")
	}
}

func TestErrorWrappedIs(t *testing.T) {
	inner := toolerr.New(toolerr.CodePathNotAllowed, "outside work_dir")
	wrapped := fmt.Errorf("render: %w", inner)
	if !errors.Is(wrapped, toolerr.New(toolerr.CodePathNotAllowed, "")) {
		t.Errorf("errors.Is should walk wrapper chain")
	}
}

func TestErrorJSONMarshal(t *testing.T) {
	e := toolerr.New(toolerr.CodePathNotAllowed, "boom").WithDetails(map[string]any{
		"reason": "outside_work_dir",
	})
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"code":"path_not_allowed"`, `"message":"boom"`, `"reason":"outside_work_dir"`} {
		if !strings.Contains(s, want) {
			t.Errorf("marshaled error missing %q: %s", want, s)
		}
	}
}

func TestWithDetailsDoesNotMutate(t *testing.T) {
	e := toolerr.New(toolerr.CodeWorkspaceFailed, "x")
	_ = e.WithDetails(map[string]any{"k": "v"})
	if e.Details != nil {
		t.Errorf("WithDetails should not mutate receiver")
	}
}
