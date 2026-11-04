# report-studio-mcp

An MCP server that turns a Markdown report manuscript written by an agent —
prose, chart data, diagram definitions — into a typeset PDF and an editable
Word document (.docx), written into the caller's work directory. It relies on no external service and no browser:
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

## Output

- **PDF**: Hiragino (or the profile's fonts), page numbers, the
  classification in the header, and — with a cover or table of contents — a
  title page and a contents list whose entries link to their headings.
- **Word (.docx)**: the same content with Word styles derived from the
  profile (headings, body, tables, quotes, code, captions), real footnotes,
  numbered lists, a table of contents field, and the same pictures and charts.
  The recipient can restyle it in Word.

## Page breaks

Most documents need nothing written for this:

- A heading, and a **caption** with what it captions, stay on the same page as
  the start of what follows. A caption is a paragraph of only bold text, or one
  starting `表1：`, `図1：`, `Table 1:` or `Figure 1:`, written directly before
  a table, chart, diagram, image or code block; an italic note line
  (`*出典: ...*`) between them belongs to it.
- Headings are never split, and a paragraph never leaves a single line alone
  on a page.
- A chart, diagram or image that does not fit the rest of a page is shrunk to
  fit (not below 70%) or moves on with its caption.
- **Numbers in table cells are not broken across lines** (`1,234,567`,
  `12.34%`, `▲１２．３４％`, `192.168.10.25`, `100mg`, `US$1,234`).
  Identifiers where letters come before digits (hashes, UUIDs,
  `CVE-2024-12345`) are not numbers and may wrap. When
  a table is too wide to keep its numbers whole, the warning
  `cell_token_split` names the columns.
- `break_before` starts chapters on a new page: with `h2` (the `formal`
  default) every top-level `#` or `##` heading that follows body text starts a
  new page (a run of headings breaks once, never at the top of a page); `h1`
  does this for `#` only, `none` never. Set it in a profile or in the front
  matter.

Two directives cover the rest, each an HTML comment alone on its own lines at
the top level of the body (other Markdown viewers show nothing).
`keep-with-next` applies to a paragraph, heading, code block, chart, diagram,
image or horizontal rule; on a table, list, quote, alert or raw HTML it is
ignored.

```markdown
<!-- pagebreak -->
<!-- keep-with-next -->
```

## Profiles

Three built-in profiles: `standard` (A4, Hiragino Sans), `formal` (cover,
table of contents, Hiragino Mincho body, wider margins, chapters on a new
page) and `compact` (narrow margins, smaller type). The operator can add profiles in `config.toml`
(`$XDG_CONFIG_HOME`, `~/.config` or `~/Library/Application Support`, under
`report-studio-mcp/`), each extending a built-in and overriding page, margins,
fonts, sizes, colours, chart palette, cover, table of contents, a cover logo
and the page break before chapters:

```toml
[profiles.customer]
extends = "formal"
margin_mm = 22
toc_title = "目次"
logo = "~/Pictures/logo.png"
```

The manuscript picks a profile by name (`profile:` in its front matter, or
the `profile` argument) and can turn the cover and contents on or off
(`cover:`, `toc:`) and set the chapter break (`break_before:`). The file is decoded strictly with
[nlink-jp/toml](https://github.com/nlink-jp/toml): an unknown key is an error.

Errors are reserved for output that would be wrong (invalid chart JSON, a
missing image, a limit exceeded); everything else is a warning carrying its
manuscript line. The full reference is the server's `get_usage` tool.

## Tools

| Tool | Role |
|---|---|
| `get_usage` | The full reference: manuscript format, chart JSON, limits, errors and warnings |
| `render` | `{work_dir, workspace_id, source}` → `<work_dir>/<workspace_id>/out/<source>.pdf` and `.docx` (`formats` picks one), with the page count and warnings |

`work_dir` is required on every call and has no default: an absolute
directory the caller can read back (organization ADR-021).

## Skill (bundled): report-manuscript

The repository bundles the **report-manuscript** skill: how to write a
manuscript this server typesets well. It covers choosing the profile and the
front matter, writing captions in the form the typesetter keeps with their
table or figure, and grouping short chapters under `formal`. It also covers
wide tables, chart and diagram layout, when to use the two directives, and
changing the manuscript in response to each warning. It builds on `get_usage`
rather than repeating it. How to structure an investigation report in general
is not this tool's craft and is not in it.

The server and the skill install separately: Homebrew and the binary zip carry
the server only. Every release attaches the skill as its own asset,
`report-manuscript-vX.Y.Z.zip`, with the skill folder at the zip's root. Take
the one from the release that matches your server.

- **gem-agent / lagent** — unpack it into `~/.config/gem-agent/skills/` or
  `~/.config/lagent/skills/`. Copy, do not link: their sandboxes cannot read
  `~/.claude`, so a link to an install there does not load.
- **Claude (desktop app / claude.ai)** — Customize → Skills → upload the zip,
  and upload the new zip again after each update.
- **Claude Code** — `unzip report-manuscript-vX.Y.Z.zip -d ~/.claude/skills/`

From a clone, `make install-skill` builds the zip and unpacks it into
`~/.claude/skills/` (`SKILLS_DEST=<dir>` for another directory). `make test`
runs the organization's skill validator. `internal/tools/skill_test.go` holds
the skill to the server. Every tool and profile must be named. Every name the
skill teaches must exist in the server, and every directive must parse as one.
The example manuscripts must parse without a warning, with the caption paired
with its table.

## Safety

- Every read and write goes through `os.Root` on the work directory; each
  file opened is named by the kernel and checked against
  [pathguard](https://github.com/nlink-jp/pathguard)'s floor. Only regular
  files are read; outputs are placed under a fresh name and renamed.
- Only `http`, `https` and `mailto` links become clickable. Defanged
  indicators stay as written.
- The .docx's core properties hold the title and author only. The PDF's
  document information holds the title and author, plus what Core Graphics
  always writes: the producer (the macOS version) and the creation date.

## Limitations

- **The .docx has not been checked in Microsoft Word.** It was written to
  ECMA-376 (5th edition, Transitional), checked part by part (well-formed XML,
  content types, relationships, footnotes, numbering, bookmarks) and opened
  without warnings in Pages 15.4 and LibreOffice 26.8, where every element
  showed. Word for Mac or Windows was not available to the project, so whether
  Word opens it without a repair prompt, and how Word renders it, are not
  verified.
- In the .docx, the table of contents lists the headings without page
  numbers; Word fills them in when the field is updated.
- In the .docx, the page-break rules are paragraph properties (keep with next,
  page break before, widow control) that the reader applies; how Word
  paginates is not verified. A person checked the page breaks in Pages and
  LibreOffice.
  Number columns are as wide as Core Text measures their numbers, but
  full-width digits (`１２３`) may still break in a narrow column there.
- Text runs left to right; right-to-left paragraphs are kept but laid out in a
  left-to-right line.

## Requirements

macOS 26 or later on Apple Silicon. No third-party dependencies beyond the
nlink-jp modules named above.

## Build

```bash
make build          # → dist/report-studio-mcp
make test           # go test ./... and the skill validator
make install-skill  # the skill zip, unpacked into ~/.claude/skills
```

## Design

The requirements and design decisions are in the RFP
([docs/en/report-studio-mcp-rfp.md](docs/en/report-studio-mcp-rfp.md)); the
measurements taken before implementing are in
[docs/en/phase0-criteria.md](docs/en/phase0-criteria.md).

## License

MIT — see [LICENSE](LICENSE). The CommonMark and GFM spec examples used as
test data are CC BY-SA 4.0 (see `internal/markdown/testdata/NOTICE`).
