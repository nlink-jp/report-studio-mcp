# report-studio-mcp

An MCP server that turns a Markdown report manuscript written by an agent —
prose, chart data, diagram definitions — into a typeset PDF and an editable
Word document (.docx), written into the caller's work directory. It relies on
no external service and no browser: typesetting is done by macOS itself
(Core Text and Core Graphics).

[日本語 README](README.ja.md)

## Why

Investigation results from gem-agent and lagent end up as Markdown replies.
Handing them to someone means shaping them by hand, and charts or diagrams
cost even more. report-studio-mcp takes the manuscript the agent wrote and
produces documents ready to hand over.

## Design

The requirements and design decisions are in the RFP:
[docs/en/report-studio-mcp-rfp.md](docs/en/report-studio-mcp-rfp.md).

- Ordinary Markdown (CommonMark plus the GitHub extensions) in; PDF and .docx out.
- Charts from data (` ```chart `) and diagrams from mermaid (` ```mermaid `).
- Output profiles switchable by configuration.
- macOS (Apple Silicon) only. No third-party dependencies.

## Build

```bash
make build    # → dist/report-studio-mcp
make test
```

## License

MIT — see [LICENSE](LICENSE).
