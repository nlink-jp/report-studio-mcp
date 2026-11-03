# AGENTS.md — report-studio-mcp

## Project summary

MCP server (Go, macOS/Apple Silicon only) that turns a Markdown report
manuscript into a typeset PDF and an editable Word document (.docx) in the
caller's work directory. PDF typesetting uses macOS Core Text and Core
Graphics through cgo; the Markdown parser, TOML reader and .docx writer are
written here. No third-party dependencies (only nlink-jp/pathguard today).
Callers: gem-agent and lagent.

**Current stage: scaffold.** The MCP skeleton and `get_usage` exist; `render`
does not. Next is Phase 0 of the RFP (measure on real hardware before
implementing).

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
internal/fsio            every read and write: os.Root + kernel path (F_GETPATH) judged by pathguard,
                         regular files only, O_EXCL temp + rename for outputs, output byte cap
internal/tools           tool registration; get_usage
scripts/                 codesign / notarize / brew (verbatim from .github/templates)
docs/{en,ja}/            RFP
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
  machine's OS — phase 0, P0-1). It takes effect only once a package imports
  "C" and the external linker runs: the cgo-free scaffold still reports
  `minos 13.0`. Check with `vtool -show-build dist/report-studio-mcp`.
- Module path: `github.com/nlink-jp/report-studio-mcp`.
