package tools

import (
	"context"
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
