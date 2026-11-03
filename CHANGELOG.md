# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [0.1.0] - Unreleased

### Known limitations

- The .docx has not been checked in Microsoft Word; it was checked against
  ECMA-376 part by part and opened in Pages 15.4 and LibreOffice 26.8 (see the
  README's Limitations).

### Added

- `render`: typesets a Markdown manuscript under `work_dir` into a PDF at
  `<work_dir>/<workspace_id>/out/<source>.pdf` with macOS Core Text and Core
  Graphics (Hiragino, Japanese line breaking, subset-embedded fonts), and
  returns the output paths, page count and warnings with manuscript lines
  (`max_warnings` caps how many are returned in full).
- Word output (`.docx`, written by default alongside the PDF; `formats` picks
  one): styles derived from the profile, real footnotes, numbered lists that
  restart per list, tables with a repeating header, the same pictures and
  charts, a cover without header or footer, a table of contents field linked
  to its headings, page numbers and the classification header.
- Profiles: built-in `standard`, `formal` (cover, table of contents, Mincho
  body) and `compact`; operator profiles in `config.toml` extending a
  built-in (page, margins, fonts, sizes, colours, chart palette, cover, table
  of contents and its title, cover logo), read strictly with nlink-jp/toml per
  call; front matter `profile`, `cover` and `toc`.
- PDF cover page and table of contents whose entries link to their headings.
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
- Limits from phase 0's measurements (manuscript, pages, tables, chart points,
  images, a shared pixel budget for images and diagrams, output bytes).
- `get_usage`: the full reference, including the error and warning table.
- Safety: every read and write through `os.Root`, kernel-named paths judged by
  pathguard's floor, regular files only, outputs placed under a fresh name and
  renamed; only http, https and mailto links are clickable; the JSON-RPC stdout
  is isolated at the descriptor level.
- A pre-release independent review (security, parser, output) and its
  fixes: a panicking tool no longer stops the server; charts with degenerate
  ranges finish and huge ones stay within a raster limit; long paragraphs and
  repeated pictures render in linear time, and every call has a 45 s deadline;
  the parser's table and footnote work is bounded; the opened work directory is
  validated by its kernel name, hard links and linked output directories are
  refused; defanged indicators stay unlinked; table cells keep strikes and
  links; deep nesting, long covers, empty headings and right-to-left text keep
  their text; notes referred to only from other notes stay in the .docx.
- Docs: the RFP and the phase 0 criteria and results
  ([docs/en/report-studio-mcp-rfp.md](docs/en/report-studio-mcp-rfp.md),
  [docs/en/phase0-criteria.md](docs/en/phase0-criteria.md)).
