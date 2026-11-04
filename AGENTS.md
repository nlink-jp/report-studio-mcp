# AGENTS.md — report-studio-mcp

## Project summary

MCP server (Go, macOS/Apple Silicon only) that turns a Markdown report
manuscript into a typeset PDF and an editable Word document (.docx) in the
caller's work directory. PDF typesetting uses macOS Core Text and Core
Graphics through cgo; the Markdown parser, TOML reader and .docx writer are
written here. Dependencies are nlink-jp modules only (pathguard,
mermaid-render, toml; mermaid-render carries its declared golang.org/x/image
and x/text exception). Callers: gem-agent and lagent.

**Current stage: released (the version is in `git tag` and the CHANGELOG) —
darwin/arm64, Homebrew formula in nlink-jp/tap.** Open follow-ups live as
issues elsewhere (ADR-025): nlink-jp/skills-series#1, narrowed by org ADR-027
to report writing that is not this tool's craft (nlink-jp/mcp-tactics#1 was
closed: ADR-027 keeps this server out of mcp-tactics; nlink-jp/pathguard#1 is
fixed in pathguard v0.3.1, which v0.1.1 takes). The .docx has NOT been checked in Microsoft Word (no Word
available; operator's decision 2026-11-04) — the README's Limitations say so;
keep that sentence true until someone checks. Phase 0 results are in
`docs/ja/phase0-criteria.ja.md`.

v0.2.0 added page breaks and placement (RFP Phase 4, Discussion Log 14-21):
reviewed, re-reviewed, and the .docx page breaks checked by a person in Pages
and LibreOffice.

Design: `docs/ja/report-studio-mcp-rfp.ja.md` (canonical) and
`docs/en/report-studio-mcp-rfp.md`.

## Build & test

```bash
make build    # → dist/report-studio-mcp, codesigned (never `go build` directly)
make test     # go test ./... + tests/validate-skill.sh (the organization's skill validator)
make package  # signed + notarized darwin-arm64 zip + package-skill → dist/report-manuscript-<version>.zip
make verify-release  # also verify-skill-zip: refuses a release without the current skill zip
make install-skill   # unpack the skill zip into ~/.claude/skills (SKILLS_DEST=...)
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
                         pages), page-break decisions and measuring (flow.go), numbers kept whole in cells
                         (numbers.go), and rendering ops to PDF (render.go). PDFKit reads output back in tests
internal/placement       what both typesetters read the same way for page breaks: caption groups,
                         directives, runs of kept blocks, number tokens and their unbreakable units
internal/fsio            every read and write: os.Root + kernel path (F_GETPATH) judged by pathguard,
                         regular files only, O_EXCL temp + rename for outputs, output byte cap
internal/theme           what a profile decides (shared by PDF and .docx) and the built-ins standard/formal/compact
internal/config          config.toml: [profiles.NAME] extending a built-in; decoded strictly with nlink-jp/toml
internal/docx            pure-Go .docx writer; pictures, charts and diagrams arrive as PNGs through callbacks
                         (tools/docxglue.go connects them to the PDF side)
internal/markdown        CommonMark 0.31.2 + GFM extensions + footnotes + alerts + front matter → ast.go's tree;
                         html.go exists for the spec tests only
internal/assets          images (PNG/JPEG by content, sized before decoding) and mermaid diagrams → pictures,
                         one pixel budget
internal/tools           get_usage (usage.go is the full reference), render, limits.go (phase 0 values);
                         skill_test.go holds the bundled skill to the server (see below)
report-manuscript/       the bundled skill (org ADR-027): writing a manuscript this server typesets well
tests/validate-skill.sh  the organization's skill validator, vendored byte-identical (org ADR-006)
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

### The bundled skill

`report-manuscript/` is this tool's craft (profile, front matter, caption form,
chapters, directives, acting on warnings), not a copy of `get_usage`.
`internal/tools/skill_test.go` holds it to the server:

- every backticked identifier must be a tool, a schema property, a code that
  `serverCodes` finds raised in the source, a json/toml tag, or a front-matter
  key that `markdown.Parse` accepts;
- every `<!-- … -->` must parse as a directive;
- the `~~~markdown` examples must parse with no warning, and their tables must
  be paired with a caption by `placement.Items`.

A renamed code, key or directive fails the build until the skill follows. When
you add a raise site of a new form, make sure `serverCodes`' pattern reads it.
`get_usage` lacked `workspace_failed` because that pattern read too few forms.

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
- The PDF layout runs for every render, whatever `formats` says: it checks the
  manuscript, produces the warnings and resolves the pictures the .docx then
  reuses (assets caches them, so they are not counted twice).
- Never gate a commit on `go test … | tail`: the pipe's status is tail's.
  Capture the status (`go test ./... >log 2>&1; rc=$?`).
- Module path: `github.com/nlink-jp/report-studio-mcp`.
- Page breaks: measuring a block's start (`lead`, `full` in pdf/flow.go) must use
  the same computation that places it. Paragraph lines, table plans and chart
  figures are cached in `l.plans` keyed by block and width, and the placement
  takes them from there and deletes them; never typeset a block a second way
  just to measure it (the start and the placement would disagree, and large
  tables would cost twice).
- `pageBody` decides chapter breaks: every op added outside `quiet` (headings,
  the title block) marks the page as holding body. Anything new that draws
  without counting as body must raise `l.quiet`.
- Word joiners (U+2060) go inside numbers in table cells in the PDF only. The
  .docx must not get them: Word keeps them in copied text.
- Caption, directive and number rules live once, in internal/placement; the
  PDF and the .docx both read from there. Change them there, never in one
  typesetter (a keep-with-next fix made only in the .docx left the PDF and
  the chapter breaks wrong: pre-release review).
- A plan's key includes the text style: the same paragraph is measured as
  the start of a quote and placed in the quote's colour. Measure a block in
  the style its placement uses.
- Digits inside an ASCII word holding a letter are an identifier, not a
  number (placement.NumberTokens). Counting hash digits as numbers made a
  hash column as wide as the hash and broke the real numbers.
