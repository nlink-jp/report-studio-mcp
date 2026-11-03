# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [0.1.0] - Unreleased

### Added

- `render`: typesets a Markdown manuscript under `work_dir` into a PDF at
  `<work_dir>/<workspace_id>/out/<source>.pdf` with macOS Core Text and Core
  Graphics (Hiragino, Japanese line breaking, subset-embedded fonts), and
  returns the output paths, page count and warnings with manuscript lines
  (`max_warnings` caps how many are returned in full).
- Manuscripts: CommonMark 0.31.2 (all 652 spec examples), GitHub's tables,
  strikethrough, task lists and autolinks (GFM 0.29-gfm), footnotes, alerts,
  and an optional front matter (title, subtitle, author, date, classification,
  profile).
- ` ```chart ` blocks: bar (vertical, horizontal, grouped, stacked), line and
  scatter charts drawn from JSON, with the organisation's data-visualisation
  marks and colour-blind-safe palette.
- ` ```mermaid ` blocks drawn with nlink-jp/mermaid-render v0.6.0; a diagram
  that cannot be drawn is shown as its source with a warning.
- PNG and JPEG images from files under the work directory.
- The `standard` profile: A4, 20 mm margins, page numbers, the classification
  in the header.
- Limits from phase 0's measurements (manuscript, pages, tables, chart points,
  images, a shared pixel budget for images and diagrams, output bytes).
- `get_usage`: the full reference, including the error and warning table.
- Safety: every read and write through `os.Root`, kernel-named paths judged by
  pathguard's floor, regular files only, outputs placed under a fresh name and
  renamed; only http, https and mailto links are clickable; the JSON-RPC stdout
  is isolated at the descriptor level.
- Docs: the RFP and the phase 0 criteria and results
  ([docs/en/report-studio-mcp-rfp.md](docs/en/report-studio-mcp-rfp.md),
  [docs/en/phase0-criteria.md](docs/en/phase0-criteria.md)).
