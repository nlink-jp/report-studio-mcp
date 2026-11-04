# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Page breaks and placement, from a real user's report on a 42-page `formal`
  document (RFP §2, Discussion Log 14-17):
  - captions (a bold-only paragraph, or one starting `表1：`, `図1：`,
    `Table 1:`, `Figure 1:`, with an italic note line) and headings of every
    level stay on the same page as the start of what follows;
  - headings are never split, and a paragraph never leaves a single line
    alone on a page;
  - charts, diagrams and images that do not fit the rest of a page are shrunk
    to fit (not below 70%) or move on with their caption;
  - numbers in table cells are not broken across lines;
  - `break_before` (`h1`, `h2`, `none`) in profiles and the front matter;
  - the directives `<!-- pagebreak -->` and `<!-- keep-with-next -->`;
  - warnings `cell_token_split`, `caption_separated` and `mermaid_scaled`;
  - the .docx carries the same as keep-with-next, page-break-before and
    widow-control properties and number-aware column widths.

### Changed

- **`formal` now starts every top-level `##` chapter on a new page** (a
  default change: documents in `formal` get longer). A document with a cover
  and contents is opened chapter by chapter; set `break_before = "none"` in a
  profile, or `break_before: none` in the front matter, for the old layout.
- An HTML comment holding only `pagebreak` or `keep-with-next` is no longer
  shown as text or warned about as `raw_html`.

## [0.1.1] - 2026-11-04

### Security

- **Checking that `work_dir` is writable no longer writes anything.** The
  check comes from nlink-jp/pathguard, now v0.3.1 (nlink-jp/pathguard#1). It
  used to create and remove a probe file by name after judging the directory
  by name, so a path component swapped for a link in between could put the
  probe inside a protected place. It is now `access(2)` (`W_OK|X_OK`),
  which writes nothing.

## [0.1.0] - 2026-11-04

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
