# report-studio-mcp

An MCP server that turns a Markdown report manuscript written by an agent —
prose, chart data, diagram definitions — into a typeset PDF, written into the
caller's work directory. It relies on no external service and no browser:
typesetting is done by macOS itself (Core Text and Core Graphics), with
Japanese line-breaking rules and fonts embedded as subsets.

[日本語 README](README.ja.md)

## Why

Investigation results from gem-agent and lagent end up as Markdown replies.
Handing them to someone means shaping them by hand, and charts or diagrams
cost even more. report-studio-mcp takes the manuscript the agent wrote and
produces a document ready to hand over.

## What it accepts

- **Ordinary Markdown**: CommonMark 0.31.2 with GitHub's tables,
  strikethrough, task lists and autolinks, plus footnotes and alerts
  (`> [!NOTE]` …). An optional front matter sets the title, subtitle,
  author, date and a classification printed on every page.
- **Charts from data**: a ` ```chart ` block holds JSON — bar (vertical,
  horizontal, grouped, stacked), line or scatter — and the server draws it.
- **Diagrams**: ` ```mermaid ` blocks (flowchart, sequence, ER, state, gantt,
  pie, mindmap) drawn by [mermaid-render](https://github.com/nlink-jp/mermaid-render).
  A diagram that cannot be drawn is shown as its source, with a warning.
- **Images**: PNG or JPEG files under the work directory.

Errors are reserved for output that would be wrong (invalid chart JSON, a
missing image, a limit exceeded); everything else is a warning carrying its
manuscript line. The full reference is the server's `get_usage` tool.

## Tools

| Tool | Role |
|---|---|
| `get_usage` | The full reference: manuscript format, chart JSON, limits, errors and warnings |
| `render` | `{work_dir, workspace_id, source}` → `<work_dir>/<workspace_id>/out/<source>.pdf`, with the page count and warnings |

`work_dir` is required on every call and has no default: an absolute
directory the caller can read back (organization ADR-021).

## Safety

- Every read and write goes through `os.Root` on the work directory; each
  file opened is named by the kernel and checked against
  [pathguard](https://github.com/nlink-jp/pathguard)'s floor. Only regular
  files are read; outputs are placed under a fresh name and renamed.
- Only `http`, `https` and `mailto` links become clickable. Defanged
  indicators stay as written.
- The PDF's document information holds the title and author only.

## Requirements

macOS 26 or later on Apple Silicon. No third-party dependencies beyond the
nlink-jp modules named above.

## Build

```bash
make build    # → dist/report-studio-mcp
make test
```

## Design

The requirements and design decisions are in the RFP
([docs/en/report-studio-mcp-rfp.md](docs/en/report-studio-mcp-rfp.md)); the
measurements taken before implementing are in
[docs/en/phase0-criteria.md](docs/en/phase0-criteria.md).

## License

MIT — see [LICENSE](LICENSE). The CommonMark and GFM spec examples used as
test data are CC BY-SA 4.0 (see `internal/markdown/testdata/NOTICE`).
