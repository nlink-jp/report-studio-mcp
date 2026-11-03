package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nlink-jp/report-studio-mcp/internal/assets"
	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/config"
	"github.com/nlink-jp/report-studio-mcp/internal/docx"
	"github.com/nlink-jp/report-studio-mcp/internal/fsio"
	"github.com/nlink-jp/report-studio-mcp/internal/markdown"
	"github.com/nlink-jp/report-studio-mcp/internal/pdf"
	"github.com/nlink-jp/report-studio-mcp/internal/toolerr"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

type renderArgs struct {
	WorkDir     string   `json:"work_dir"`
	WorkspaceID string   `json:"workspace_id"`
	Source      string   `json:"source"`
	Formats     []string `json:"formats"`
	Profile     string   `json:"profile"`
	MaxWarnings *int     `json:"max_warnings"`
}

const renderSchema = `{
  "type": "object",
  "properties": {
    "work_dir": {"type": "string", "description": "Absolute path of a directory you can read back. Required; there is no default."},
    "workspace_id": {"type": "string", "description": "Workspace under work_dir the output goes to: <work_dir>/<workspace_id>/out/. Letters, digits, '.', '_' and '-'."},
    "source": {"type": "string", "description": "The manuscript, a Markdown file, as a path relative to work_dir."},
    "formats": {"type": "array", "items": {"type": "string", "enum": ["pdf", "docx"]}, "description": "Outputs to write. Default both: [\"pdf\", \"docx\"]."},
    "profile": {"type": "string", "description": "Output profile name. Default: the manuscript's front matter 'profile', else \"standard\"."},
    "max_warnings": {"type": "integer", "minimum": 0, "description": "How many warnings to return in full (default 50); the rest are counted."}
  },
  "required": ["work_dir", "workspace_id", "source"],
  "additionalProperties": false
}`

// output is one written file.
type output struct {
	Format  string `json:"format"`
	Path    string `json:"path"`     // relative to work_dir
	AbsPath string `json:"abs_path"` // as the kernel names it
	Bytes   int    `json:"bytes"`
}

type warning struct {
	Line    int    `json:"line,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type renderResult struct {
	WorkDir         string    `json:"work_dir"`
	Outputs         []output  `json:"outputs"`
	Pages           int       `json:"pages"`
	Warnings        []warning `json:"warnings"`
	WarningsTotal   int       `json:"warnings_total"`
	WarningsOmitted int       `json:"warnings_omitted"`
}

var workspaceName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// renderer holds what render needs beyond its arguments.
type renderer struct {
	resolver workdir.Resolver
	// configPaths is where config.toml is looked for (config.Paths in the
	// server; a test's own file in tests).
	configPaths []string
}

func decodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return toolerr.Newf(toolerr.CodeInvalidArguments, "invalid arguments: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return toolerr.New(toolerr.CodeInvalidArguments, "invalid arguments: trailing data")
	}
	return nil
}

func (r renderer) render(ctx context.Context, raw json.RawMessage) (any, error) {
	var a renderArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, err
	}
	dir, err := r.resolver.Resolve(ctx, a.WorkDir)
	if err != nil {
		return nil, err
	}
	if !workspaceName.MatchString(a.WorkspaceID) || a.WorkspaceID == "." || a.WorkspaceID == ".." {
		return nil, toolerr.Newf(toolerr.CodeInvalidArguments,
			"workspace_id %q is not a name: use 1-64 letters, digits, '.', '_' or '-'", a.WorkspaceID)
	}
	if err := r.resolver.CheckBeneath(filepath.Join(dir, a.WorkspaceID)); err != nil {
		return nil, err
	}
	if a.Source == "" {
		return nil, toolerr.New(toolerr.CodeMissingArgument, "source is required: the manuscript's path relative to work_dir")
	}
	formats, err := checkFormats(a.Formats)
	if err != nil {
		return nil, err
	}
	maxWarn := defaultMaxWarnings
	if a.MaxWarnings != nil {
		if *a.MaxWarnings < 0 {
			return nil, toolerr.New(toolerr.CodeInvalidArguments, "max_warnings must be 0 or more")
		}
		maxWarn = *a.MaxWarnings
	}

	d, err := fsio.Open(dir, r.resolver, maxOutputBytes)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	// Validate what was opened, by the kernel's name for it: the name
	// validated above could have been swapped for a link since.
	if _, err := r.resolver.Validate(d.Path()); err != nil {
		return nil, err
	}
	if err := r.resolver.CheckBeneath(filepath.Join(d.Path(), a.WorkspaceID)); err != nil {
		return nil, err
	}
	src, err := d.ReadFile(a.Source, maxManuscriptBytes)
	if err != nil {
		return nil, err
	}
	// Core Foundation refuses invalid UTF-8; the parser would pass it through.
	src = bytes.ToValidUTF8(src, []byte("\uFFFD"))
	doc := markdown.Parse(src)
	if err := checkTables(doc); err != nil {
		return nil, err
	}

	_, cfg, err := config.Load(r.configPaths)
	if err != nil {
		return nil, toolerr.Newf("config_invalid", "the configuration cannot be used: %v", err)
	}
	profile := a.Profile
	if profile == "" {
		profile = doc.Meta["profile"]
	}
	th, err := cfg.Theme(profile)
	if err != nil {
		return nil, toolerr.New("profile_unknown", err.Error()).
			WithDetails(map[string]any{"profile": profile, "available": cfg.Names()})
	}
	var extra []markdown.Warning
	cover, toc := th.Cover, th.TOC
	for key, dst := range map[string]*bool{"cover": &cover, "toc": &toc} {
		switch v := doc.Meta[key]; v {
		case "":
		case "true", "yes":
			*dst = true
		case "false", "no":
			*dst = false
		default:
			extra = append(extra, markdown.Warning{Line: 1, Code: "invalid_front_matter_value",
				Message: fmt.Sprintf("front matter %s: %q is not true or false; the profile's setting is used", key, v)})
		}
	}
	var logo *pdf.Picture
	if cover && th.Logo != "" {
		pic, err := r.readLogo(th.Logo)
		if err != nil {
			return nil, err
		}
		logo = &pic
	}

	ctdoc, err := pdf.NewDoc(nil)
	if err != nil {
		return nil, toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot start typesetting: %v", err)
	}
	defer ctdoc.Close()
	deadline := time.Now().Add(renderTimeBudget)
	as := assets.New(d, filepath.ToSlash(filepath.Clean(a.Source)), assets.Limits{
		ImageBytes: maxImageBytes, ImageSide: maxImageSide, Images: maxImages,
		Diagrams: maxDiagrams, PixelBudget: pixelBudget,
	}, assets.Layout{BodyPt: th.BodySize, ContentW: th.ContentW(), DPI: 300})
	chartLimits := chart.Limits{MaxPoints: maxChartPoints, MaxSeries: min(len(th.Chart.Palette), 8)}
	// The PDF layout runs whatever the formats: it checks the manuscript and
	// produces the warnings, and the .docx draws on the pictures it resolved.
	res, err := pdf.Layout(doc, ctdoc, pdf.Options{
		Theme:          th,
		Assets:         as,
		MaxPages:       maxPages,
		Chart:          chartLimits,
		MaxChartPoints: maxDocChartPoints,
		Cover:          cover,
		TOC:            toc,
		Logo:           logo,
		Deadline:       deadline,
	})
	if err != nil {
		return nil, stopError(err)
	}

	out := renderResult{WorkDir: dir, Pages: len(res.Pages)}
	warns := append(res.Warnings, extra...)
	for _, f := range formats {
		var data []byte
		switch f {
		case "pdf":
			data, err = pdf.Render(res, ctdoc, th, pdf.Meta{
				Title: titleOf(doc), Author: doc.Meta["author"], Classification: doc.Meta["classification"],
			})
		case "docx":
			var more []markdown.Warning
			data, more, err = docx.Write(doc, docxOptions(doc, th, cover, toc, logo, as, ctdoc, chartLimits, deadline))
			warns = mergeWarnings(warns, more)
		}
		if err != nil {
			return nil, stopError(err)
		}
		rel := outputPath(a.WorkspaceID, a.Source, f)
		abs, err := d.WriteFile(rel, data)
		if err != nil {
			return nil, err
		}
		out.Outputs = append(out.Outputs, output{Format: f, Path: rel, AbsPath: abs, Bytes: len(data)})
	}
	out.WarningsTotal = len(warns)
	out.Warnings = []warning{}
	for i, w := range warns {
		if i >= maxWarn {
			out.WarningsOmitted = len(warns) - maxWarn
			break
		}
		out.Warnings = append(out.Warnings, warning{Line: w.Line, Code: w.Code, Message: w.Message})
	}
	return out, nil
}

func checkFormats(fs []string) ([]string, error) {
	if len(fs) == 0 {
		return []string{"pdf", "docx"}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, f := range fs {
		switch f {
		case "pdf", "docx":
		default:
			return nil, toolerr.Newf(toolerr.CodeInvalidArguments, "unknown format %q: use \"pdf\" or \"docx\"", f)
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out, nil
}

// readLogo reads the cover logo the operator's configuration names. It is
// outside the work directory by design, so it is opened directly, but under
// the same rules as any image: a regular file, the floor, the size limits.
func (r renderer) readLogo(path string) (pdf.Picture, error) {
	// Named by base name only: the operator's path holds their user name.
	fail := func(format string, a ...any) error {
		return toolerr.Newf("logo_invalid", "the configured logo %s "+format, append([]any{filepath.Base(path)}, a...)...)
	}
	data, err := fsio.ReadOutside(path, maxImageBytes, r.resolver)
	if err != nil {
		return pdf.Picture{}, fail("%v", err)
	}
	pic, err := assets.Decode(data, filepath.Base(path), maxImageSide, nil)
	if err != nil {
		return pdf.Picture{}, fail("is not usable: %v", err)
	}
	return pic, nil
}

// outputPath keeps the manuscript's path relative to work_dir under the
// workspace's out/, so same-named manuscripts in different directories do not
// collide: a/report.md → <ws>/out/a/report.pdf.
func outputPath(workspace, source, format string) string {
	s := filepath.ToSlash(filepath.Clean(source))
	s = strings.TrimSuffix(s, path.Ext(s))
	return path.Join(workspace, "out", s+"."+format)
}

// titleOf is the document's title: the front matter's, else its first
// heading's text (code spans included: dropping them lost words, independent
// review).
func titleOf(doc *markdown.Document) string {
	if t := doc.Meta["title"]; t != "" {
		return t
	}
	for _, b := range doc.Children {
		if h, ok := b.(*markdown.Heading); ok {
			return pdf.PlainText(h.Children)
		}
	}
	return ""
}

// checkTables refuses tables over the limits before anything is laid out,
// footnotes included.
func checkTables(doc *markdown.Document) error {
	cells := 0
	var walk func([]markdown.Block) error
	walk = func(bs []markdown.Block) error {
		for _, b := range bs {
			switch v := b.(type) {
			case *markdown.Table:
				if len(v.Rows) > maxTableRows || len(v.Align) > maxTableCols {
					return tableLimit(int(v.Line), fmt.Sprintf("a table has %d rows and %d columns; the limits are %d and %d",
						len(v.Rows), len(v.Align), maxTableRows, maxTableCols))
				}
				cells += (len(v.Rows) + 1) * len(v.Align)
				if cells > maxTableCells {
					return tableLimit(int(v.Line), fmt.Sprintf("the tables hold more than %d cells in all", maxTableCells))
				}
			case *markdown.BlockQuote:
				if err := walk(v.Children); err != nil {
					return err
				}
			case *markdown.Alert:
				if err := walk(v.Children); err != nil {
					return err
				}
			case *markdown.List:
				for _, it := range v.Items {
					if err := walk(it.Children); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(doc.Children); err != nil {
		return err
	}
	for _, n := range doc.Footnotes {
		if err := walk(n.Children); err != nil {
			return err
		}
	}
	return nil
}

func tableLimit(line int, msg string) error {
	return toolerr.New(toolerr.CodeLimitExceeded, msg).WithDetails(map[string]any{"limit": "table", "line": line})
}

// stopError carries a typesetter stop onto toolerr with its line.
func stopError(err error) error {
	var e *pdf.Error
	if !errors.As(err, &e) {
		return toolerr.Newf(toolerr.CodeWorkspaceFailed, "cannot typeset: %v", err)
	}
	details := map[string]any{}
	for k, v := range e.Details {
		details[k] = v
	}
	if e.Line > 0 {
		details["line"] = e.Line
	}
	te := toolerr.New(e.Code, e.Error())
	if len(details) > 0 {
		te = te.WithDetails(details)
	}
	return te
}
