package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

const manuscript = `---
title: 調査報告書
author: 調査班
classification: 社外秘
---

# 1. 概要

調査の結果、社内から社外への通信の一部が
既知の不審な宛先に向かっていた[^src]。詳しくは [報告書](https://example.com/r) を参照。

| 宛先 | 件数 |
|------|-----:|
| 社内 | 1,204 |
| 社外 | 731 |

` + "```chart" + `
{"type":"bar","x":{"categories":["社内","社外"]},"series":[{"values":[1204,731]}]}
` + "```" + `

![宛先の内訳](../img/a.png)

> [!WARNING]
> 不審な宛先は hxxp://bad[.]example のまま記す。

[^src]: 試験用の架空の観測値。
`

func workDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func put(t *testing.T, dir, rel string, body []byte) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func smallPNG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func call(t *testing.T, args map[string]any, config ...string) (renderResult, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := renderer{resolver: workdir.NewResolverFor(), configPaths: config}
	out, err := r.render(context.Background(), raw)
	if err != nil {
		return renderResult{}, err
	}
	return out.(renderResult), nil
}

func codeOf(err error) string {
	var te *toolerr.Error
	if errors.As(err, &te) {
		return te.Code
	}
	return "not a toolerr: " + err.Error()
}

func TestRenderWritesAReadablePDF(t *testing.T) {
	dir := workDir(t)
	put(t, dir, "report/report.md", []byte(manuscript))
	put(t, dir, "img/a.png", smallPNG(t))
	res, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "report/report.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outputs) != 1 || res.Outputs[0].Path != "ws/out/report/report.pdf" ||
		res.Outputs[0].AbsPath != filepath.Join(dir, "ws", "out", "report", "report.pdf") || res.WorkDir != dir {
		t.Fatalf("result %+v", res)
	}
	data, err := os.ReadFile(res.Outputs[0].AbsPath)
	if err != nil || len(data) != res.Outputs[0].Bytes {
		t.Fatalf("output %d bytes, %v", len(data), err)
	}
	text, pages, err := pdf.ExtractText(data)
	if err != nil || pages != res.Pages {
		t.Fatalf("pages %d vs %d, %v", pages, res.Pages, err)
	}
	flat := strings.Join(strings.Fields(text), "")
	for _, want := range []string{"調査報告書", "社外秘", "1.概要", "既知の不審な宛先に向かっていた", "1,204", "宛先の内訳",
		"Warning", "hxxp://bad[.]example", "試験用の架空の観測値。"} {
		if !strings.Contains(flat, want) {
			t.Errorf("PDF lacks %q", want)
		}
	}
	// A soft line break between Japanese characters is not a space.
	if !strings.Contains(flat, "通信の一部が既知の") {
		t.Errorf("soft break between Japanese lines left a gap")
	}
	if !bytes.Contains(data, []byte("example.com/r")) || bytes.Contains(data, []byte("bad[.]example)")) {
		t.Errorf("links wrong")
	}
	for _, leak := range []string{dir, "/Users/"} {
		if bytes.Contains(data, []byte(leak)) {
			t.Errorf("PDF contains %q", leak)
		}
	}
}

func TestRenderErrors(t *testing.T) {
	dir := workDir(t)
	put(t, dir, "r.md", []byte("# x\n"))
	put(t, dir, "chart.md", []byte("# x\n\n```chart\n{\"type\":\"pie\"}\n```\n"))
	put(t, dir, "img.md", []byte("![a](nope.png)\n"))
	base := func(kv ...any) map[string]any {
		m := map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md"}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	cases := map[string]struct {
		args map[string]any
		code string
	}{
		"no work_dir":     {map[string]any{"workspace_id": "ws", "source": "r.md"}, "work_dir_required"},
		"bad workspace":   {base("workspace_id", "../x"), "invalid_arguments"},
		"no source":       {base("source", ""), "missing_argument"},
		"missing source":  {base("source", "nope.md"), "file_not_found"},
		"source outside":  {base("source", "../etc/hosts"), "path_not_allowed"},
		"docx":            {base("formats", []string{"pdf", "docx"}), "unsupported_format"},
		"unknown format":  {base("formats", []string{"html"}), "invalid_arguments"},
		"unknown profile": {base("profile", "fancy"), "profile_unknown"},
		"unknown arg":     {base("colour", "red"), "invalid_arguments"},
		"bad chart":       {base("source", "chart.md"), "chart_invalid"},
		"missing image":   {base("source", "img.md"), "file_not_found"},
	}
	for name, c := range cases {
		if _, err := call(t, c.args); err == nil || codeOf(err) != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
	// A chart error without a position names the block's opening fence.
	_, err := call(t, base("source", "chart.md"))
	var te *toolerr.Error
	if errors.As(err, &te) && te.Details["line"] != 3 {
		t.Errorf("chart error details %v, want line 3", te.Details)
	}
}

func TestRenderSurvivesInvalidUTF8AndCapsWarnings(t *testing.T) {
	dir := workDir(t)
	var b strings.Builder
	b.WriteString("# 見出し\xff\xfe\n\n")
	for i := 0; i < 80; i++ {
		b.WriteString("<span>raw</span>\n\n")
	}
	put(t, dir, "r.md", []byte(b.String()))
	res, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md", "max_warnings": 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 5 || res.WarningsTotal < 80 || res.WarningsOmitted != res.WarningsTotal-5 {
		t.Errorf("warnings %d shown, %d total, %d omitted", len(res.Warnings), res.WarningsTotal, res.WarningsOmitted)
	}
	if res.Warnings[0].Line == 0 || res.Warnings[0].Code != "raw_html" {
		t.Errorf("first warning %+v", res.Warnings[0])
	}
}

func TestOutputPathKeepsTheManuscriptsPlace(t *testing.T) {
	for src, want := range map[string]string{
		"report.md":        "ws/out/report.pdf",
		"a/report.md":      "ws/out/a/report.pdf",
		"a/./b/../r.v2.md": "ws/out/a/r.v2.pdf",
		"noext":            "ws/out/noext.pdf",
	} {
		if got := outputPath("ws", src, "pdf"); got != want {
			t.Errorf("outputPath(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestTableLimits(t *testing.T) {
	dir := workDir(t)
	var b strings.Builder
	b.WriteString("|" + strings.Repeat(" h |", 51) + "\n|" + strings.Repeat("---|", 51) + "\n")
	put(t, dir, "wide.md", []byte(b.String()))
	_, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "wide.md"})
	if codeOf(err) != "limit_exceeded" {
		t.Errorf("51 columns: %v", err)
	}
}

func pagesOf(t *testing.T, res renderResult) []string {
	t.Helper()
	data, err := os.ReadFile(res.Outputs[0].AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := pdf.ExtractText(data)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(text, "\f")
}

func TestProfilesFromTheConfiguration(t *testing.T) {
	dir := workDir(t)
	logo := filepath.Join(workDir(t), "logo.png")
	if err := os.WriteFile(logo, smallPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(workDir(t), "config.toml")
	if err := os.WriteFile(cfg, []byte("[profiles.customer]\nextends = \"formal\"\ntoc_title = \"目次\"\nlogo = \""+logo+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "r.md", []byte("---\ntitle: 顧客向け\nprofile: customer\n---\n\n# 第一章\n\n本文。\n"))
	res, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	pages := pagesOf(t, res)
	if len(pages) != 3 || !strings.Contains(pages[0], "顧客向け") || !strings.Contains(pages[1], "目次") || !strings.Contains(pages[2], "本文") {
		t.Errorf("cover, contents, body expected: %q", pages)
	}
	// The front matter turns the cover and contents off again.
	put(t, dir, "s.md", []byte("---\nprofile: customer\ncover: false\ntoc: no\n---\n\n# 第一章\n\n本文。\n"))
	res, err = call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "s.md"}, cfg)
	if err != nil || len(pagesOf(t, res)) != 1 {
		t.Errorf("front matter override: %v", err)
	}
	put(t, dir, "u.md", []byte("---\ncover: maybe\n---\n\nx\n"))
	res, err = call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "u.md"}, cfg)
	if err != nil || res.WarningsTotal != 1 || res.Warnings[0].Code != "invalid_front_matter_value" {
		t.Errorf("bad cover value: %+v %v", res.Warnings, err)
	}
	// Built-ins are always there; an unknown name lists what is.
	_, err = call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md", "profile": "nope"}, cfg)
	var te *toolerr.Error
	if !errors.As(err, &te) || te.Code != "profile_unknown" || !strings.Contains(fmt.Sprint(te.Details["available"]), "customer") {
		t.Errorf("unknown profile: %v", err)
	}
}

func TestConfigAndLogoErrors(t *testing.T) {
	dir := workDir(t)
	put(t, dir, "r.md", []byte("# x\n"))
	bad := filepath.Join(workDir(t), "config.toml")
	_ = os.WriteFile(bad, []byte("[profiles.x]\ncolour = 1\n"), 0o644)
	if _, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md"}, bad); codeOf(err) != "config_invalid" {
		t.Errorf("broken config: %v", err)
	}
	noLogo := filepath.Join(workDir(t), "config.toml")
	_ = os.WriteFile(noLogo, []byte("[profiles.x]\nextends = \"formal\"\nlogo = \"/nonexistent/logo.png\"\n"), 0o644)
	if _, err := call(t, map[string]any{"work_dir": dir, "workspace_id": "ws", "source": "r.md", "profile": "x"}, noLogo); codeOf(err) != "logo_invalid" {
		t.Errorf("missing logo: %v", err)
	}
	if !strings.Contains(usageWithConfig([]string{bad}), "cannot be used") {
		t.Error("get_usage does not report the broken configuration")
	}
	if !strings.Contains(usageWithConfig(nil), "standard, formal, compact") {
		t.Error("get_usage does not list the profiles")
	}
}
