// Package e2e drives the built binary over stdio the way an MCP client does:
// the only test that sees the real process — descriptor isolation, the real
// mermaid engine, the real fonts.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
)

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "report-studio-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "MACOSX_DEPLOYMENT_TARGET=26.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

const manuscript = "---\ntitle: 通しの試験\n---\n\n# 構成\n\n" +
	"```mermaid\nflowchart LR\n  A[受信] --> B{判定}\n  B -->|不審| C[隔離]\n  B -->|正常| D[配送]\n```\n\n" +
	"```mermaid\njourney\n  title 描けない種類\n```\n\n" +
	"```chart\n{\"type\":\"line\",\"x\":{\"categories\":[\"9月\",\"10月\",\"11月\"]},\"series\":[{\"name\":\"受信\",\"values\":[120,180,150]},{\"name\":\"隔離\",\"values\":[3,9,4]}]}\n```\n"

func TestServeRendersOverStdio(t *testing.T) {
	bin := build(t)
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "r.md"), []byte(manuscript), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 1<<20), 1<<20)

	send := func(id int, method string, params any) map[string]any {
		t.Helper()
		req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if _, err := fmt.Fprintf(stdin, "%s\n", req); err != nil {
			t.Fatal(err)
		}
		done := make(chan bool, 1)
		var line string
		go func() { done <- lines.Scan(); line = lines.Text() }()
		select {
		case ok := <-done:
			if !ok {
				t.Fatalf("no response to %s; stderr:\n%s", method, stderr.String())
			}
		case <-time.After(30 * time.Second):
			t.Fatalf("%s timed out", method)
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("stdout carried something that is not JSON-RPC: %q", line)
		}
		return resp
	}

	send(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "e2e", "version": "1"}})
	list := send(2, "tools/list", map[string]any{})
	if b, _ := json.Marshal(list); !strings.Contains(string(b), `"render"`) {
		t.Fatalf("tools/list: %s", b)
	}
	resp := send(3, "tools/call", map[string]any{"name": "render",
		"arguments": map[string]any{"work_dir": work, "workspace_id": "ws", "source": "r.md"}})
	res := resp["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("render failed: %v", res)
	}
	var out struct {
		Outputs []struct {
			AbsPath string `json:"abs_path"`
		} `json:"outputs"`
		Warnings []struct {
			Line int    `json:"line"`
			Code string `json:"code"`
		} `json:"warnings"`
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("result text: %v\n%s", err, text)
	}
	if len(out.Warnings) != 1 || out.Warnings[0].Code != "mermaid_fallback" || out.Warnings[0].Line != 14 {
		t.Errorf("warnings = %+v, want one mermaid_fallback at line 14", out.Warnings)
	}
	data, err := os.ReadFile(out.Outputs[0].AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := pdf.ExtractText(data)
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(got), "")
	for _, want := range []string{"通しの試験", "構成", "journey", "受信", "隔離"} {
		if !strings.Contains(flat, want) {
			t.Errorf("PDF lacks %q", want)
		}
	}
	if os.Getenv("REPORT_STUDIO_E2E_PDF") != "" {
		_ = os.WriteFile(os.Getenv("REPORT_STUDIO_E2E_PDF"), data, 0o644)
	}
}
