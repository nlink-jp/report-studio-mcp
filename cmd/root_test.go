package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/tools"
)

// The shared homebrew formula template — used by every tool in the org — runs
// `<binary> --version` in its test block. Both spellings must work and must
// print the same string.
func TestVersionFlagAndSubcommandAgree(t *testing.T) {
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if code := Run(args, strings.NewReader(""), &out, io.Discard); code != 0 {
			t.Fatalf("%v: exit code %d", args, code)
		}
		return out.String()
	}

	flag := run("--version")
	sub := run("version")

	if want := Version + "\n"; flag != want {
		t.Errorf("--version printed %q, want %q", flag, want)
	}
	if flag != sub {
		t.Errorf("--version printed %q but `version` printed %q; they must agree", flag, sub)
	}
}

// An argument the binary does not know is refused rather than silently served.
func TestUnknownArgumentIsRefused(t *testing.T) {
	var stderr bytes.Buffer
	if code := Run([]string{"--bogus"}, strings.NewReader(""), io.Discard, &stderr); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--bogus") {
		t.Errorf("stderr does not name the argument: %q", stderr.String())
	}
}

// TestServeListsTools drives the default serve path end-to-end over in-memory
// stdio: initialize answers with the server identity and the instructions, and
// tools/list includes the registered tools.
func TestServeListsTools(t *testing.T) {
	stdin := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}, "\n") + "\n")

	var stdout, stderr bytes.Buffer
	if code := Run(nil, stdin, &stdout, &stderr); code != 0 {
		t.Fatalf("serve exit code %d, stderr: %s", code, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 responses, got %d: %s", len(lines), stdout.String())
	}
	var init struct {
		Result struct {
			ServerInfo   struct{ Name string } `json:"serverInfo"`
			Instructions string                `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &init); err != nil {
		t.Fatalf("initialize response: %v", err)
	}
	if init.Result.ServerInfo.Name != "report-studio-mcp" {
		t.Errorf("server name = %q", init.Result.ServerInfo.Name)
	}
	if init.Result.Instructions != tools.Instructions {
		t.Errorf("initialize does not carry tools.Instructions: %q", init.Result.Instructions)
	}
	if !strings.Contains(lines[1], `"name":"get_usage"`) {
		t.Errorf("tools/list missing get_usage: %s", lines[1])
	}
}
