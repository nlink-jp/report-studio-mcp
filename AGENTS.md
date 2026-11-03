# AGENTS.md — report-studio-mcp

## Project summary

MCP server (Go, macOS/Apple Silicon only) that turns a Markdown report
manuscript into a typeset PDF and an editable Word document (.docx) in the
caller's work directory. PDF typesetting uses macOS Core Text and Core
Graphics through cgo; the Markdown parser, TOML reader and .docx writer are
written here. Dependencies are nlink-jp modules only (pathguard,
mermaid-render; mermaid-render carries its declared golang.org/x/image and
x/text exception). Callers: gem-agent and lagent.

**Current stage: phase 1 done (PDF).** `render` writes PDFs with the
`standard` profile. Phase 0 results are in `docs/ja/phase0-criteria.ja.md`
(the Word check is carried over to before release). Next is phase 2: .docx,
profiles from config.toml, `formal` / `compact`, cover and TOC.

Design: `docs/ja/report-studio-mcp-rfp.ja.md` (canonical) and
`docs/en/report-studio-mcp-rfp.md`.

## Build & test

```bash
make build    # → dist/report-studio-mcp, codesigned (never `go build` directly)
make test     # go test ./...
make package  # signed + notarized darwin-arm64 zip
make verify-release
```

`make build` needs `CGO_ENABLED=1` (set by the Makefile) and Xcode's command
line tools.

## Structure

```
main.go                  calls cmd.Execute()
cmd/root.go              stdlib argument handling: serve (default), version, --version
internal/jsonrpc         JSON-RPC 2.0 types and codes
internal/transport       stdio transport (newline-delimited JSON)
internal/mcpserver       MCP routing, tool registration, initialize, request _meta
internal/toolerr         structured tool errors {code, message, details}
internal/workdir         work_dir resolution and refusals (adapter over pathguard)
internal/chart           ```chart JSON → strict validation → figure (shapes + text in points, output-independent);
                         marks and palette from the org data-viz method, scaled for print; text measured via an
                         injected Measurer (Core Text in the PDF side, a fixed advance in tests)
internal/pdf             cgo bridge to Core Text / Core Graphics (bridge.c), the block layout into per-page
                         op lists (layout.go, table.go; frames carry quote bars and code backgrounds across
                         pages), and rendering ops to PDF (render.go). PDFKit reads output back in tests
internal/fsio            every read and write: os.Root + kernel path (F_GETPATH) judged by pathguard,
                         regular files only, O_EXCL temp + rename for outputs, output byte cap
internal/markdown        CommonMark 0.31.2 + GFM extensions + footnotes + alerts + front matter → ast.go's tree;
                         html.go exists for the spec tests only
internal/assets          images (PNG/JPEG by content, sized before decoding) and mermaid diagrams → pictures,
                         one pixel budget
internal/tools           get_usage (usage.go is the full reference), render, limits.go (phase 0 values)
e2e/                     builds the binary and drives it over stdio
scripts/                 codesign / notarize / brew (verbatim from .github/templates)
docs/{en,ja}/            RFP, phase 0 criteria and results
```

### Provenance of ported packages

Ported from `util-series/chrome-pilot-mcp` on 2026-11-03 (the newest copy of
each package among the siblings, compared per package with
`git log -1 --format=%cs -- internal/<pkg>`):

| Package | Source commit date | Changes on porting |
|---|---|---|
| `jsonrpc` | 2026-07-29 | import path |
| `transport` | 2026-07-29 | import path |
| `mcpserver` | 2026-09-22 | import path; server name in tests; ADR reference now org ADR-021 |
| `toolerr` | 2026-09-22 | browser-specific codes removed; tests rewritten to the remaining codes |
| `workdir` | 2026-09-22 | comments; the protected-place test uses the server's own config directory |

When a sibling fixes one of these packages, port the fix here too.

## Gotchas

- `sed` on this machine is GNU sed aliased; `sed -i ''` misbehaves. Edit files
  with the Edit tool or python3.
- The release is darwin/arm64 only; there is no cross-compiled matrix
  (cgo + macOS frameworks).
- The minimum macOS is 26.0, pinned by `MACOSX_DEPLOYMENT_TARGET` in the
  Makefile (CGO flags alone leave Go's runtime cgo objects at the build
  machine's OS — phase 0, P0-1). It takes effect because packages import "C"
  and the external linker runs (a cgo-free build reported `minos 13.0`).
  Check with `vtool -show-build dist/report-studio-mcp`: 26.0.
- Core Foundation refuses invalid UTF-8 (CFStringCreateWithCString returns
  NULL): render replaces it before parsing, and the bridge replaces NUL.
  Anything new that hands text to the bridge must keep that true.
- A block's `Line` is its opening line (for a fence, the fence). A chart JSON
  error with a position reports `Line + ce.Line`; one without, the fence.
- Typesetter stops travel as `panic(stop{...})` inside `pdf.Layout` and are
  recovered there; never let one escape the package.
- The CommonMark spec test runs with extensions off (as cmark-gfm does); the
  six examples that change with them on are pinned in
  `TestCommonMarkSpecWithExtensions`.
- Module path: `github.com/nlink-jp/report-studio-mcp`.
