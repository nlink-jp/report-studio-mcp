# CLAUDE.md — report-studio-mcp

Read AGENTS.md for project summary, build commands, and structure. The design
is the RFP in `docs/ja/report-studio-mcp-rfp.ja.md` (canonical) and
`docs/en/report-studio-mcp-rfp.md`.

## Non-negotiable project rules

1. **No third-party dependencies.** `go.mod` may require only modules of the
   nlink-jp organization (today `nlink-jp/pathguard`; `nlink-jp/mermaid-render`
   when diagrams land, carrying its own declared exception for
   `golang.org/x/image` and `golang.org/x/text`). The Markdown, TOML and .docx
   code is written here. If a need seems to require any other dependency,
   stop and discuss instead of adding it (org ADR-024).
2. **No browser, no network.** Typesetting is macOS Core Text and Core
   Graphics through cgo. Nothing is fetched at runtime; images come only from
   files under the call's work directory.
3. **Every read and write goes through `os.Root`** on the work directory.
   pathguard is a floor (places never to touch), not confinement — layer it on
   top, never instead. Open regular files only; place outputs by creating a
   temporary name with `O_EXCL` and renaming it.
4. **Errors only for mistakes.** A document is refused only when the output
   would be wrong. Anything else is a warning and the document is still
   written; appearance is never a gate (not in code, not in tests).
5. **MCP owns stdout** — logs go to stderr only. Native frameworks are linked
   in, so the JSON-RPC stdout is isolated at the fd level once the server
   starts.
6. **Changes to other projects are issues**, not steps of this plan (org
   ADR-025): mcp-tactics and the support skill live elsewhere.

Org-wide rules (tests mandatory, `make build` not `go build`, docs in sync,
typed commits) apply as usual; see the workspace CLAUDE.md and
CONVENTIONS.md.
