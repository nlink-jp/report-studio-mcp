package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// TestIsolatedStdoutCarriesOnlyTheStream runs this test binary again as a
// child that isolates stdout, prints to descriptor 1 the way native code
// would, then writes through the private descriptor: the parent's stdout must
// hold only what went through the private one.
func TestIsolatedStdoutCarriesOnlyTheStream(t *testing.T) {
	if os.Getenv("RSM_ISOLATION_CHILD") == "1" {
		f, err := isolateStdout()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		fmt.Println("noise from a native library")
		_, _ = os.Stdout.WriteString("more noise\n")
		_, _ = f.WriteString(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestIsolatedStdoutCarriesOnlyTheStream$")
	cmd.Env = append(os.Environ(), "RSM_ISOLATION_CHILD=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("child: %v\n%s", err, stderr.String())
	}
	if got := stdout.String(); got != `{"jsonrpc":"2.0","id":1,"result":{}}`+"\n" {
		t.Errorf("stdout = %q", got)
	}
	if !strings.Contains(stderr.String(), "noise from a native library") || !strings.Contains(stderr.String(), "more noise") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestServingIsTheDefault(t *testing.T) {
	for args, want := range map[string]bool{"": true, "serve": true, "version": false, "--version": false, "serve x": false} {
		if got := serving(strings.Fields(args)); got != want {
			t.Errorf("serving(%q) = %v", args, got)
		}
	}
}
