# RFP: report-studio-mcp

> Generated: 2026-11-03
> Revised: 2026-11-03 (independent verification findings applied)
> Status: Draft

## 1. Problem Statement

There is no way to turn the results of an investigation done in gem-agent or lagent into a document
fit to hand to someone. Today the Markdown reply is passed on as it is, or shaped by hand with pandoc
or browser printing, and adding charts or diagrams costs even more effort. report-studio-mcp is an MCP
server that takes a report manuscript written by an agent (prose, chart data, diagram definitions) and
writes a typeset PDF and an editable Word document (.docx) into the work directory. It relies on no
external service and no browser — only on macOS itself. The first user is the operator; the callers
are gem-agent and lagent. Other MCP clients are added once their work directory and the way they write
a manuscript have been measured.

## 2. Functional Specification

### Commands / API Surface

| Tool | Role |
|---|---|
| `get_usage` | The full reference: accepted Markdown, the ` ```chart ` JSON spec, choosing a profile, limits, the error and warning table (stable `code` and recovery) |
| `render` | Renders a manuscript file in the work directory to PDF and .docx. Synchronous |

There are no tools to list workspaces or their contents (as with image-forge, voice-studio and
video-studio: the workspace namespace is shared between MCP servers, and there is no reason to enumerate
another server's workspaces). There is no separate validation tool either: `render` reports manuscript
errors with line numbers.

`render` arguments:

| Argument | Meaning |
|---|---|
| `work_dir` | Required, no default (org ADR-021) |
| `workspace_id` | The workspace the output goes to |
| `source` | Manuscript path, relative to `work_dir` |
| `formats` | `pdf` / `docx`; both by default |
| `profile` | Profile name; falls back to the front matter, then to `standard` |
| `max_warnings` | How many warnings to return. Beyond the default only the count is returned, so a caller who needs them raises it and asks again |

`render` returns: `work_dir`, each output's `path` (relative) and `abs_path` (org ADR-021 §6), page
count (PDF), warnings (line number, `code`, message) with the total count, and how many were omitted.

The `initialize` instructions state the `work_dir` contract, as the existing work-directory servers do.

### Input / Output

**Input: ordinary Markdown**

Specifications followed (pinned in Phase 0: CommonMark 0.31.2, GFM 0.29-gfm, GitHub Docs for footnotes
and callouts, ECMA-376 5th edition Transitional for .docx — sources and dates in P0-5 of
[phase0-criteria.md](phase0-criteria.md)):

- Body: the CommonMark spec. Correctness is checked against that version's official examples (about 650).
- GitHub extensions (tables, strikethrough, task lists, autolinks): the GFM spec.
- Footnotes and callouts (`> [!NOTE]`, `> [!WARNING]` …): not in the GFM spec, so the GitHub Docs
  description is recorded as the source and tests are derived from it.
- Front matter: a single level of `key: value` lines between leading `---` lines (no nesting, no
  arrays). Accepted keys: title, subtitle, author, date, classification (e.g. "Confidential"),
  `profile`, cover page and table of contents on/off, the page break before chapters (`break_before`,
  v0.2.0). All optional; an unknown key is a warning.
  Without front matter, the first heading is the title.

What a manuscript can contain:

- ` ```chart ` blocks: JSON naming the chart type and its data. Types: vertical and horizontal bars
  (grouped, stacked), line, scatter. The server draws; the model never writes drawing code.
- ` ```mermaid ` blocks: the diagrams mermaid-render supports (flowchart, sequence, ER, state, gantt,
  pie, mindmap).
- Images `![caption](relative/path)`: resolved from the manuscript's directory, reachable only within
  `work_dir`, so PNGs data-toolbox-mcp wrote into another workspace can be included. The format is
  decided by content and limited to PNG and JPEG. Nothing is fetched from a URL.
- Links: only `http`, `https` and `mailto` become clickable. Any other scheme is shown as text with a
  warning. Defanged URLs (`hxxp://`, `[.]` …) are never refanged; they are shown as text as written.

**Errors versus warnings**

An error (no output) only when the output would be wrong:

- invalid chart JSON
- an image that is missing, unreadable, outside `work_dir`, not PNG/JPEG, or over a limit
- a manuscript, table and so on over a limit (see "Limits")

Everything else is a warning and the document is still written:

- A mermaid diagram that cannot be drawn (`SyntaxError`, `UnsupportedType`, `UnsupportedConstruct`,
  `LayoutFault`): as mermaid-render's contract says, its source is shown as a code block instead. A
  diagram that would exceed the pixel limit at print scale is drawn at a lower scale.
- Raw HTML is shown as text, except an HTML comment holding only one of the two directives below, which
  is not shown and acts as the directive (v0.2.0, "Page breaks and placement" below). Headings deeper
  than level 3 take the level-3 look.
- A wide table first shrinks its type down to a floor, then wraps inside cells. Unbreakable long lines
  (code, URLs) are force-wrapped.
- Control characters that XML (.docx) cannot carry are removed.

**Page breaks and placement (v0.2.0)**

Rules that work without changing the manuscript come first; manuscript directives are an escape hatch
"for when the intent must be said", and there are only two. Every rule is specified behaviour, and the
tests check that a rule was followed or, where it could not be, that its warning was given (this is apart
from the policy that appearance never decides pass or fail).

- **The start of a block**: the "start" the rules below use is the smallest first part the block's own
  placement allows. A paragraph follows the widow/orphan rule below: the whole of it at three lines or
  fewer, two lines at four or more. A list: the start of its first item's first block. A table: the
  header row and the first row. Code: its padding and two lines. A chart, diagram or image: the whole of it
  (at the smallest size the shrinking below allows). A quote or callout: the callout's label line and the
  start of its first block. The start comes from the same computation that places the block (it is not
  counted separately).
- **Captions**: a paragraph made only of bold text, or one starting `表N：`, `図N：`, `Table N:` or
  `Figure N:` (N a number; full- and half-width colons and minor separator variants accepted), is a
  caption. It is paired with what follows only when, inside the same container (the body, a list item, a
  quote), that is a table, chart, diagram, image (a paragraph of only images) or code block. A paragraph of
  only italics right after the caption (source, note) joins the pair. Captions placed below their body are
  not handled (an image's description stays under the image as before).
- **Keep with next**: a heading of any level, or a caption group, is placed on the same page as the start
  of the next block; if they do not fit, they move to the next page together. Consecutive headings (with
  captions between them, too) form one group. A group is measured once, and measuring stops as soon as it
  exceeds the body height of a fresh page; such a group is placed in order without this rule. Nothing is
  carried past a group (a page-break directive or a chapter break ends it).
- **Headings are not split**: however many lines a heading wraps to, no page break falls inside it.
- **Shrinking charts and figures**: a chart, mermaid diagram or image is first sized as today, to the text
  width and the page-height caps; that is its "original size". When the original size does not fit what is
  left of the page (after the caption group's own lines), it is shrunk to fit in place if the shrunk size
  is at least 70% of the original and at least eight body lines are left; otherwise it moves to the next
  page with its caption group. Whether a group fits a fresh page is also measured with this shrinking
  allowed. 70% and eight lines are initial values, measured on the test data below and recorded.
- **Widows and orphans**: a paragraph of two or more lines never leaves a single line alone on a page (not
  its first line at the foot of a page, not its last line on the next; a paragraph of two or three lines
  is not split), as Word's `widowControl` does. Not applied to code or table cells.
- **Numbers are not broken**: a number in a table cell (digits with `,` and `.`, including a leading sign or
  currency symbol and a trailing `%`) does not wrap mid-way. A column's minimum width is at least the width
  of the longest number in it. When the minimum widths together exceed the text width, the table shrinks
  and wraps as before, and one warning per table says numbers broke. An ASCII word in which a letter comes
  before a digit (hashes, UUIDs, `CVE-2024-12345`, `ID1234`) is an identifier: not covered and not counted
  toward column widths (landscape pages are meant to handle them). Letters after the digits are a unit
  (`100mg`, `+1.5pt`) and the number stays a number, as does one starting with a currency sign after letters
  (`US$1,234`). An IP address is a number too. Whether
  Core Text has break opportunities inside numbers with full-width digits, `−` (U+2212) or signs such as
  `▲` is measured before implementing.
- **Page break before chapters**: the profile's `break_before` (`h1` / `h2` / `none`) breaks the page before
  headings of that level and above at the outermost level of the body. It breaks only when the page already
  holds something other than headings and the title block (so a run of headings breaks only before its
  first; the title block, and a level-1 heading the cover uses as its title, belong to the first chapter;
  never at the top of a page). Not applied to headings inside quotes, lists or footnotes. `formal` defaults
  to `h2`; `standard` and `compact` to `none`. The front matter's `break_before` overrides it per
  manuscript. A bad value is the existing `invalid_front_matter_value` in front matter, and a
  configuration error in the configuration file.
- **Manuscript directives**: a block at the outermost level of the body made only of an HTML comment whose
  content is `pagebreak` or `keep-with-next` (case and the whitespace around it may vary; whitespace inside the
  name may not, so `<!-- page break -->` is a note; anything after `-->` makes it not a directive). A directive raises no `raw_html` warning and is not shown. A directive does not count
  as a block (it is transparent to caption groups and starts). Directives inside containers, and comments
  with any other content, are shown as text as before.
  - `<!-- pagebreak -->`: break the page here. Nothing happens at the top of a page (several in a row make
    no blank pages).
  - `<!-- keep-with-next -->`: keep the next block on the same page as the start of the block after it;
    when the next block fits a page, it moves whole. It applies to paragraphs, headings, code, charts,
    diagrams, images and rules; not to tables, lists, quotes or raw HTML (they may exceed a page; the
    directive is ignored there, and the rules for a heading or caption before them still hold).
- **.docx**: Word decides the page breaks, so paragraphs carry the matching properties. Caption groups get
  `keepNext`; chapter breaks and `pagebreak` set `pageBreakBefore` on the next paragraph (no paragraph made
  only for a break; not on the first heading right after the cover and TOC, nor on the second and later
  headings of a run; a `pagebreak` before anything is written does nothing when the body starts at the top
  of a page; a table, which has no paragraph properties, gets a small break-only paragraph before it). A
  table's header row and a code block's first line always carry `keepNext` (keeping the same start as the
  PDF).
  Set per paragraph, not in the heading styles. `widowControl` is written in the Normal style (it is on by
  default when absent, but is stated). Table column widths take the longest number as their minimum, as in the PDF (measured with
  Core Text in the table fonts). What these properties mean is read in the specification's definitions
  before implementing (source: Discussion Log 17). Word cannot be checked (Discussion Log 13), so a person checks the page
  breaks in Pages and LibreOffice.
- **Warnings** (output is still written; with line numbers):
  - `caption_separated`: a caption and its body ended up on different pages (the group does not fit a page).
  - `cell_token_split`: a number in a table cell broke across lines; one per table, listing the headers of
    the columns where it happened.
  - `mermaid_scaled`: a diagram was shrunk so its text is below 60% of the body size; with the scale (60% is
    an initial value).

Out of scope (later versions; Discussion Log 14): landscape pages, a distinct look for level-4 headings,
explicit caption syntax with automatic numbering, captions below their body, switching a mermaid diagram's
direction automatically, manual line breaks in the cover title.

**Limits**

Manuscript bytes, page count, table rows and columns, chart data points, image count, dimensions,
bytes and total, and total output size are all capped. An image's dimensions are read first and only
images within the limits are decoded. Initial values are set in Phase 0 by measuring the worst case, so
that a call stays well inside gem-agent's default call timeout (60 s) (initial values and measurements in
P0-4 of [phase0-criteria.md](phase0-criteria.md); cost is set almost entirely by image pixels, so image
and mermaid pixels share one budget). mermaid-render's own limits
(node count and so on) are inherited as they are.

**Output**

- Under `out/` in the workspace, keeping the manuscript's path relative to `work_dir`
  (e.g. `a/report.md` → `<workspace>/out/a/report.pdf`), so same-named manuscripts in different
  directories do not collide.
- Placement goes through `os.Root` (the work directory): a temporary name is created new (`O_EXCL`),
  written, and `rename`d to the final name. No link is followed out of the work directory.
- PDF: fonts are embedded as subsets, numeric charts are vector, mermaid diagrams are raster images at
  a resolution sufficient for print. Links are clickable (the schemes above only).
- .docx: the profile is mapped onto Word styles (headings, body, tables, callouts…), so the recipient
  can adjust the layout in Word. Charts are PNG. The table of contents is a Word TOC field. Follows
  Office Open XML (ECMA-376); the edition and conformance class (the Transitional form Word writes) are
  pinned and recorded in Phase 0.
- Metadata: the PDF document info, the .docx `core.xml` and image descriptions carry only the front
  matter's title and author and the captions — never absolute paths, user names or input file names
  (checked by a regression test).

### Configuration

The configuration file is optional. It is looked up at `$XDG_CONFIG_HOME/report-studio-mcp/config.toml`
→ `~/.config/report-studio-mcp/config.toml` →
`~/Library/Application Support/report-studio-mcp/config.toml`, and the path actually read is logged and
shown by `get_usage`. Unknown keys are errors (strict decoding).

- `[server]`: logging.
- `[profiles.<name>]`: output profiles. `extends = "<built-in>"` takes a built-in profile as the base
  and overrides only some fields.

A profile sets: paper (A4 / Letter) and margins; body, heading and code fonts (any installed font);
font sizes and line spacing; colours (headings, emphasis, callouts, chart palette); header and footer
(title, classification, page number); cover page and table of contents; a logo image for the cover; the
page break before chapters (`break_before`, v0.2.0).
Text inside mermaid diagrams stays Hiragino for now (mermaid-render loads fonts through a different
mechanism from Core Text, and some faces cannot be read by it).

A configuration mistake (a font that does not exist, say) does not stop the server. A `render` that uses
that profile returns an error naming the cause (with a `code`); other profiles keep working.

Built-in profiles (compiled in, not editable):

| Name | Use | Look |
|---|---|---|
| `standard` (default) | Everyday investigation reports | A4, Hiragino Sans, no cover or TOC, page number in the footer |
| `formal` | Formal internal documents | Cover and TOC, Hiragino Mincho body, classification in the header, page break before level-2 headings (v0.2.0) |
| `compact` | Drafts and notes | Narrow margins and smaller type to keep the page count down |

The model can only choose a profile by name; what a profile contains is decided by the operator's
configuration file.

### External Dependencies

- macOS Core Text and Core Graphics (through cgo) — components the OS maintains.
- In-house libraries: `nlink-jp/mermaid-render`, `nlink-jp/pathguard`. The `golang.org/x/image` and
  `golang.org/x/text` that mermaid-render brings in carry over the exception declared and approved for
  mermaid-render.
- TOML is decoded by the organisation's library `nlink-jp/toml` (standard library only, TOML 1.1.0,
  unknown keys are errors by default). See Discussion Log item 11.
- The MCP implementation (JSON-RPC, stdio, tool registration, structured errors, work-directory
  resolution) is ported from chrome-pilot-mcp, whose only dependency is pathguard. AGENTS.md records
  which package came from where.
- External services and network: none.

## 3. Design Decisions

| Decision | Reason |
|---|---|
| A Go MCP server, not a skill | Build, signing, tap and release are shared with the existing MCP servers. mermaid-render can be linked in directly. A skill uploaded to claude.ai runs on Linux and cannot use macOS facilities |
| PDF typeset with Core Text and Core Graphics | No dependency on Chrome. Kinsoku and font subsetting are done by the OS (confirmed by a probe; see the Discussion Log). Security fixes arrive with the OS |
| No new dependencies | The organisation does not adopt community-made components (org ADR-024). The .docx writer and the Markdown and TOML readers are in-house |
| A chart is drawn once and emitted twice | Vector in the PDF; a PNG rasterised by the OS in the .docx |
| Only mistakes are errors | An over-fixed format turns every out-of-range construct into a rejection and a rewrite. A diagram that cannot be drawn is replaced by its source. Matters of appearance are left to people |
| Reads and writes go through `os.Root` | A spawned MCP server is not confined by any runtime (org ADR-021 §7), and the work directory is writable from inside the sandbox. Checking a literal path and then opening it is a TOCTOU. pathguard is a floor of places never to touch, not confinement, so it is layered on top of `os.Root` |
| Appearance switched by profile | Three built-ins plus `extends` overrides. Reading Word templates is complex and not adopted |
| Page breaks: automatic rules first, only two manuscript directives (v0.2.0) | Real use separated captions from their bodies and broke numbers mid-way (Discussion Log 14). Rules that need no manuscript change prevent most of it; directives are limited to HTML comments, which ordinary Markdown viewers do not show, so manuscripts stay compatible |
| Where writing guidance lives | How to use the tool lives in `get_usage`. How to write a good report, and the workflow, live in a separate support skill (skills-series, its own RFP). Parameters and error tables are not written into the skill (a source of drift) |

**Complements**: gem-agent and lagent (callers); mermaid-render (diagrams); pathguard (the floor);
data-toolbox-mcp (analysis charts as images); investigation skills such as incident-research (sources
for a clean copy).

**Out of scope**: reading Word templates, editing existing PDF or .docx files, PowerPoint output, HTML
output, fetching images from URLs, vertical Japanese text, mathematical formulas, PDF encryption or
digital signatures, PDF/A and tagged PDF, operating systems other than macOS, claiming support for MCP
clients other than gem-agent and lagent (added after measurement).

**Accepted residuals**:

- mermaid diagrams are raster images inside the PDF (mermaid-render produces images), embedded at a
  resolution sufficient for print.
- Text inside diagrams does not follow the profile's font settings (Hiragino for now).

## 4. Development Plan

### Phase 0: Measure on real hardware (before implementation)

Write and commit the pass criteria first, then measure. A failure sends us back to the design decisions.

1. Calling Core Text and Core Graphics from Go through cgo gives the same font subsetting and kinsoku as
   the Python probe. Also check whether these frameworks write to stdout (if they do, isolate the
   JSON-RPC stdout at the fd level), and state and verify the minimum OS of the cgo-linked binary
   (`LC_BUILD_VERSION` minos).
2. A minimal hand-written .docx (heading styles, a table, an image, a footnote, a TOC field) opens in
   both Word for Mac and Pages with nothing missing. A person opens them (`textutil` is a different
   reader from Word).
3. File size and resolution of a mermaid diagram embedded in a PDF, and the share of diagrams that hit
   the pixel limit at print scale.
4. Measure the worst-case manuscript (tables, images and pages at the limits) and set the initial limits.
5. Pin the specification versions (CommonMark, GFM, ECMA-376) and record them with the sources for
   footnotes and callouts.

### Phase 1: Core (up to PDF output)

- MCP skeleton: the org ADR-021 work-directory contract, `get_usage`, `render`, stable `code`s, schema
  arch tests. Workspace management takes `CheckBeneath` as a required argument (org ADR-022 §5).
- I/O layer: `os.Root`, the pathguard floor, regular files only, temporary name plus `rename`.
- Markdown parser: every official CommonMark example as a test; GitHub extensions, footnotes, callouts,
  front matter.
- PDF typesetting: headings, paragraphs, lists, tables, code, callouts, images, page breaks, header and
  footer, page numbers, links, handling of content that does not fit.
- Charts (bar, line, scatter); mermaid diagrams with the source fallback.
- Limits. Only the `standard` built-in profile. The in-house TOML reader (strict).

### Phase 2: Features (Word and appearance settings)

- .docx output: profile mapped to Word styles; tables, images, footnotes, TOC field; control-character
  removal.
- Profiles from the configuration file with `extends`; built-in `formal` and `compact`.
- Cover page and table of contents (both PDF and .docx).

### Phase 3: Release

- `get_usage` text, README (en/ja), AGENTS.md, CHANGELOG. A `--version` response (required by
  `verify-release`).
- Independent verification before release. A person checks the output in Pages and LibreOffice (Word is
  not available and the check is not performed; stated under Limitations in the README; Discussion Log 13).
- Signing and notarisation, `make brew`, tap, submodule in util-series, the util-series README catalog
  row, one line in util-series CLAUDE.md on the build (cgo, darwin/arm64 only), the org profile README,
  the nlink-web-site catalog and the repository About, knowledge feedback, check-org.
- Registering with gem-agent and lagent changes the operator's machine, so the steps are announced
  beforehand and the registration is removed after verification (org ADR-023).

### Phase 4: v0.2.0 (page breaks and placement)

A revision after a real user's report (Discussion Log 14), implementing "Page breaks and placement" above.

- First, the reported problems are written as tests that fail on v0.1.0 (the split and stranded headings
  were inferred from code, so they must be shown to fail first).
- The reporter's synthetic sample becomes test data. Across every dummy line count, the reported problems
  (separated captions, numbers broken mid-way, a wide diagram shrunk) are shown fixed, or warned about
  where they cannot be. The initial values 70%, eight lines and 60% are measured across all of them and
  recorded.
- Per rule, a test checks positions on the page (heading and the next block's start on one page, no split
  heading, no widow or orphan, no broken number, shrink of at least 70%, breaks where directed, no blank
  page from a title block followed by headings).
- Measuring a start is a side-effect-free function that shares each block's computation (table column
  widths and type size, a paragraph's lines, chart and figure sizes) with its placement. A paragraph's start
  is measured from its first window only. Time is measured on a manuscript at every limit and on one with
  long runs of headings and directives next to the largest table. Added pages count toward the page limit
  (no trial page breaks).
- In the .docx, the XML is checked for the properties, and a person checks the page breaks in Pages and
  LibreOffice.
- A test confirms the feature is wired into `pdf.Options` and the production .docx call.
- Independent verification when the design is settled and before release; README (en/ja), `get_usage` and
  CHANGELOG updated.

### Open issues (handled in other projects; org ADR-025)

Not steps of this plan; an issue is filed in each project:

- skills-series/mcp-tactics: add the new server to the index and the description (both the body and the
  trigger line).
- skills-series: the support skill for writing reports (its own RFP). v0.2.0's manuscript directives and how
  to write captions belong to what the skill teaches (to be added to skills-series#1).

### Test policy

Pass or fail is never decided by how good something looks; tests judge mistakes:

- Usage-shaped test data: have gem-agent, lagent and Claude write investigation reports and collect them,
  to check that "the Markdown a model ordinarily writes goes through". They are written on test subjects
  so that no secrets or personal data are included.
- PDF: extract the text with the OS's PDF facility (PDFKit) and check that every character of the
  manuscript is present, in order; check by coordinates that no text or figure overflows the margins.
  mermaid diagrams are raster, so check that a figure is placed, its size, and the source fallback.
- .docx: reading it back with `textutil` only compares content (it does not show how Word renders it).
  Check that the XML is well-formed and that no forbidden characters remain.
- An end-to-end test with a stub JSON-RPC client, from start-up to `render`'s output.
- Attack-shaped tests: links pointing outside the work directory, `..`, FIFOs, images with huge
  dimensions, links with forbidden schemes, links planted at the output location.
- Mutation checks: break the implementation to confirm the tests catch it.

### Independently reviewable units

The Markdown parser, chart drawing, PDF typesetting, .docx writing and the I/O layer are built as
components that do not depend on each other, so each can be reviewed on its own.

## 5. Required API Scopes / Permissions

None. No external API, service or credential is used. No macOS privacy permission (Full Disk Access and
the like) is needed — using fonts is not a permission-gated resource. Manuscripts and images are read,
and outputs written, only inside the work directory the caller names. The only reads outside it are what
the operator names in the configuration file (the file itself and the logo image).

## 6. Series Placement

Series: util-series
Reason: the same family as image-forge, video-studio-mcp, voice-studio-mcp and data-toolbox-mcp — MCP
servers that produce artefacts in a work directory. It is not a client for a particular external
service, so it does not belong in cli-series. Development starts in `_wip/report-studio-mcp/` and the
repository is added to util-series as a submodule once published.

## 7. External Platform Constraints

No external platform is depended on. Product-side constraints:

- macOS only (Core Text and Core Graphics), darwin/arm64 only. Because it links with cgo, the minimum OS
  is stated and verified (Phase 0) and also checked on the macOS 26 verification environment.
- The same .docx may look slightly different in Word and in Pages. Pass means "opens" and "nothing
  missing"; small differences in appearance are accepted.
- mermaid-render's limits (node count and so on) are inherited; a diagram over them is replaced by its
  source with a warning.
- gem-agent's default call timeout is 60 s; the limits leave a margin under it.
- gem-agent and lagent spill large tool results to a file rather than truncating them. The warning cap
  keeps the response readable and can be raised with `max_warnings`.

---

## Discussion Log

1. **The first proposal was a skill plus Chrome (2026-11-03).** gem-agent and lagent load skills in
   Claude Code's format, run their scripts through `shell_exec` in the sandbox, and the existing skills
   use the Python standard library only. python-docx, reportlab and pandoc are community-made and
   cannot be used. The first proposal was "JSON manuscript → HTML and .docx from the standard library →
   PDF printed by Chrome".
2. **Dependence on Chrome rejected (operator).** OS typesetting was probed instead. From Python's ctypes,
   Japanese text was laid out with Core Text and written to an A4 PDF with Core Graphics: 27 KB,
   Hiragino W3 and W6 embedded as subsets (`CIDFontType0C`, `AAAAAB+` prefix), kinsoku for commas,
   full stops and closing brackets in effect, and a vector bar chart drawn. The result was the same
   under a Seatbelt profile that allows by default and confines writes to the work directory. The probe
   is kept outside the repository.
3. **Switched to an MCP server (operator's suggestion).** The decisive reasons: mermaid-render can be
   linked in as a library (a skill would have only its dev-only CLI, which is not distributed), and a
   skill on claude.ai runs on Linux and cannot use macOS facilities.
4. **Input format.** The first recommendation was "a fixed Markdown subset, anything else an error".
   After the operator pointed out that an over-fixed format is hard to use, it became "CommonMark plus
   the GitHub extensions, with errors only when the output would be wrong". The cost is writing the
   Markdown parser in-house. A JSON-structured manuscript was not adopted: local models have trouble
   putting long Japanese prose into JSON strings without mistakes.
5. **Chart types.** Bar, line, scatter and mermaid's pie are enough (operator).
6. **Appearance.** At the operator's request, output profiles are switchable by configuration, with
   several recommended profiles built in. Reading Word templates is not needed (operator).
7. **Support skill (operator's suggestion).** How to use the tool goes in `get_usage`; how to write and
   the workflow go in a skill — a separate skills-series repository with its own RFP, prose only and no
   scripts, so every runtime can use it.
8. **Scope confirmations.** The operator accepted that mermaid diagrams are raster in the PDF and that
   HTML output is out of scope.
9. **Independent verification (2026-11-03).** A verifier without this conversation's history checked the
   design against the org conventions, ADRs, knowledge, memory and related projects (3 high, 11 medium,
   10 low; all adopted). Main corrections:
   - Using BurntSushi/toml violated the rule in org ADR-024. Only servers created before the rule was
     stated (2026-09-05) use it; every server since is dependency-free (brave-search parses TOML itself).
     → TOML is now parsed in-house.
   - pathguard had been described as confinement; it is a floor, not confinement (its own README says
     so). → Reads and writes through `os.Root`, regular files only, and temporary name plus `rename` are
     now in the design.
   - The warning cap had been justified by "the caller's output limit", which was wrong (large results
     are spilled to a file). → `max_warnings` raises it, and the reason was corrected.
   - Making the mcp-tactics update and the support skill steps of this plan violated org ADR-025.
     → They are open issues.
   - Added: image format and dimension limits, overall limits, link schemes, pinned spec versions,
     metadata, strict configuration reading, content that does not fit, control characters, stdout
     isolation, minimum OS, and the missing release steps.
   - The workspace listing tools were dropped, in line with the image-forge family.
10. **Operator decisions after the verification (2026-11-03, all as recommended).**
    - A mermaid diagram that cannot be drawn is not an error: its source is shown as a code block with a
      warning (a revision of the error policy from item 4 — the causes include things that are not
      manuscript mistakes, such as emoji or engine faults).
    - Image paths are resolved from the manuscript's directory, reachable throughout `work_dir`
      (data-toolbox writes into a different workspace).
    - Text inside diagrams stays Hiragino for now.
    - Callers are limited to gem-agent and lagent. Claude Desktop is added after its work directory and
      the way it writes a manuscript have been measured.
11. **TOML decoding (phase 2, 2026-11-03).** As phase 2 began, the organisation released a TOML decoding
    library, `nlink-jp/toml` (v0.1.1, standard library only, passes toml-test's 1.1.0 suite, unknown keys are
    errors by default; the operator told us). The plan to port chrome-pilot-mcp's subset reader was dropped
    in favour of it. It is the organisation's own, so the rule against community-made components (org
    ADR-024) holds.
12. **Phase 2 output check (2026-11-03).** A `formal` sample (cover, contents, alert, table, grouped bar
    chart, mermaid diagram, image, nested lists, task list, code, footnote, classification) was rendered with
    the real binary; the operator opened the .docx in Pages (Pages Creator Studio 15.4) and LibreOffice
    26.8.2.2 and confirmed it displays correctly. The PDF was checked as images.
13. **The Word check (2026-11-04).** There is no way to obtain Word for Mac, so the operator decided the Word
    check is not performed. The README (en/ja) has a "Limitations" section stating that the .docx has not
    been checked in Word and what was checked instead (part-by-part checks against ECMA-376, display in
    Pages 15.4 and LibreOffice 26.8).
14. **A real user's report and the scope of v0.2.0 (2026-11-04).** A user who set a 42-page Japanese report
    with `formal` sent requests about page breaks and placement (the manuscript is confidential; only its
    shape and a synthetic sample were shared). Ten problems: captions separated from their bodies, a
    heading split across pages, headings stranded at a page foot, numbers in table cells wrapped mid-way,
    IDs in wide tables wrapped, charts moved to the next page leaving space, a wide mermaid diagram shrunk,
    the cover title wrapped, pages more than half empty.
    - **Reproduction (measured on v0.1.0)**: the synthetic sample was set 26 ways, with 20 to 45 dummy
      lines. The table caption was separated in 4 of them and the chart caption in 15. Numbers broke as
      `1,234,5`/`67` and `12.34`/`%`. The wide diagram's text came out at about 4 pt by eye. The split and
      stranded headings were not reproduced, but the code was confirmed to reserve a fixed "one heading line
      plus two body lines" for a heading.
    - **Causes**: a bold paragraph is just a paragraph, with nothing keeping it with what follows. The
      heading reservation does not look at the next block's actual start. A table column's minimum width
      depends only on the type size, not on the numbers in it, so Core Text finds no break opportunity and
      breaks inside the characters. Charts and figures either fit whole or go to the next page.
    - **The operator's decisions (all as recommended)**: the recommended set of automatic rules (column
      widths that keep numbers whole, keep-with-next and unsplit headings, shrinking charts and figures,
      widows and orphans, the warnings, a chapter break in `formal`). Only two manuscript directives,
      `pagebreak` and `keep-with-next`. Landscape pages, a distinct level-4 look, and explicit captions with
      numbering are left for later versions.
    - **Not adopted**: switching a mermaid diagram's direction (LR to TB) automatically would rearrange the
      author's diagram, so only the warning (`mermaid_scaled`) is given.
15. **Independent verification of the design (2026-11-04).** A verifier without the conversation's history
    checked it against the org conventions, ADRs, knowledge base, memory and the current code (2 high,
    8 medium, 10 low). Adopted and fixed:
    - The chapter break contradicted "consecutive headings form one group": `# Title` followed by `## 1.`
      left a page holding only the level-1 heading. → Break only when the page holds something other than
      headings and the title block; outermost level of the body only.
    - "Two lines of a paragraph" as the start disagreed with the widow/orphan rule, stranding a heading
      before a three-line paragraph. → The start is "the smallest first part the placement allows", from the
      same computation.
    - In the .docx, a chapter break right after the cover's and TOC's page breaks would make a blank page. →
      Per paragraph, not on the first heading nor on later headings of a run; no break-only paragraphs.
    - How to measure a start was not said (the layout is full of side effects, trial-and-rollback is
      fragile, and measuring separately doubles the table and long-paragraph work). → Side-effect-free
      functions sharing the computation.
    - Headings at level 4 and deeper fell outside "levels 1-3", a regression. → All levels.
    - Unbroken numbers and widows/orphans were PDF-only. → Column-width minimums and `widowControl` in the
      .docx too.
    - The meaning of the .docx properties, and whether Core Text breaks inside full-width digits or signs,
      were unverified (org ADR-023). → Read the spec clauses and measure before implementing; besides the
      XML check, a person checks Pages and LibreOffice.
    - Gaps in the directives (`raw_html` warning, text after `-->`, inside containers, at the top of a page,
      relation to groups). → Stated under "Manuscript directives" above.
    - The low findings (caption groups only within one container, the definition of "original size" for
      shrinking, one warning per table, handling bad values, a wiring test, the relation to the test policy,
      constants as measured initial values, the addition to the support skill) were applied too.
    - **Withdrawn**: the `short_page` warning. It made up for a side effect of the chapter break with a
      message that names no action the model can take ("a report is not a control").
16. **`formal`'s default chapter break (2026-11-04).** The verifier in item 15 said changing `formal`'s
    default to `h2` ran into the org lesson "do not change a default because of a minority's complaint". The
    operator answered: when the aim is clean output, should a chapter break in the middle of a page be
    acceptable? That lesson is for a behaviour that suits most uses where only the reporter's use differs; it
    does not apply here. `formal` is the profile for formal documents with a cover and a table of contents;
    chapters starting at the top of a page is the convention for such documents, and readers open chapters
    by the page numbers in the contents and hand chapters out separately. A chapter starting mid-page is a
    defect against this profile's own purpose. → `formal` defaults to `h2` (`standard` and `compact` aim at
    compact output and stay `none`). The cost of the change (how many pages are added, that no near-empty
    pages appear) is measured in Phase 4's tests. CHANGELOG and README state it as a default change.
17. **Measurements behind the implementation, and Phase 4's results (2026-11-04, macOS 27.2, Apple M2 Max).**
    - **Core Text and numbers**: ten numbers were set in 13 contexts, 14 line positions and 3 column widths.
      Half-width numbers stay whole when the column is at least as wide as the number with what cannot be
      separated from it (an opening bracket before, closing brackets and full stops after, letters run into
      it, and the character before a leading `-`). Full-width digits broke even with the column at 1.3 times
      the number (there is a break opportunity between them). A WORD JOINER (U+2060) between a number's
      characters stopped that too, 0/42 splits; it has no width and is not in the text PDFKit extracts. → The
      PDF uses both. The .docx does not get U+2060, which would come along when a number is copied from Word
      (README, Limitations).
    - **Word's paragraph properties**: read in the element definitions of ISO/IEC 29500-1 (= ECMA-376 Part 1)
      quoted by Microsoft Learn's Open XML SDK reference (2026-11-04). `keepNext` chains, and a group taller
      than a page starts on a new page and flows; `pageBreakBefore` overrides `keepNext`; `widowControl` is
      on when absent. That source gives no clause numbers, so the element names are recorded.
    - **Reported problems not reproduced**: a real heading wrapping to three lines was not split even on
      v0.1.0 (the height reserved for headings sufficed). The report's #2 was a bold paragraph standing in for
      a heading; it reproduced on v0.1.0 as a two-line paragraph split one and one. → The widow/orphan rule
      covers paragraphs of two or more lines (as Word does).
    - **What keep-with-next means**: keeping "the end of the next block" with the following start would need
      pulling a paragraph's last lines forward. Instead, the next block moves whole when it fits a page, which
      is what Word's `keepNext` on all of that block's paragraphs does.
    - **The synthetic sample**: across 61 variants (0-60 dummy lines) the table and chart captions stay with
      their bodies and no number breaks. The chart was shrunk in place in 8 (74-99%) and moved on with its
      caption in 13. The wide mermaid diagram gave `mermaid_scaled` with its text at 52% of the body size. →
      70%, eight lines and 60% are kept as the initial values.
    - **Cost of `formal`'s chapter break**: on 40 synthetic seven-chapter reports (about 0.7 page a chapter,
      paragraphs, sections and tables mixed at random), 203 pages with `none` became 330 with `h2` (+3.2 a
      document). No page was empty; 49 pages (about 1.2 a document) were under 20% full, a previous
      chapter's end running a few lines onto the next page. The shorter the chapters, the higher the cost;
      these reports are near the worst case.
    - **Time**: a manuscript with 80 groups of headings, directives and captions and a 2,000-row table took
      0.18 s; eight times the groups stayed within proportion.

18. **A person checked the .docx page breaks (2026-11-04).** The operator opened two check documents in
    Pages and LibreOffice and answered "looks fine". The items were: chapters (`h2`) start at the top of a
    page; no blank page between the contents and chapter 1 or anywhere else; captions on the same page as
    their table or chart; a new page after `pagebreak`; numbers in tables not broken mid-way. The answer
    did not break down per item or per application. Word was not checked (Discussion Log 13).

19. **Pre-release independent review (2026-11-04).** Two verifiers without the conversation's history
    reviewed the PDF typesetting, and the .docx, parser, configuration and documents. Every finding was
    adopted and fixed:
    - `keep-with-next` on a table, list or quote dropped the heading or caption group before it, warned
      `caption_separated` every time and moved the chapter break forward (both verifiers). → The shared
      reading never marks those blocks.
    - A quote's first paragraph right after a heading was drawn in the body colour (the plan key had no text
      style).
    - The block after a kept chart or diagram went to the next page (it was measured as if shrunk). → A kept
      figure is measured at its original size.
    - Digit runs inside a hash were read as numbers, the column minimum became the whole hash, and the real
      numbers broke instead (worse than v0.1.1). → Digits in an ASCII word holding a letter are an identifier.
    - The .docx's estimate for number columns (0.55 em a half-width character) was below the measured 0.56-
      0.71 em. → Measure with Core Text in the table fonts.
    - A `pagebreak` right after the cover or contents could make a blank page in the .docx; the break before a
      table could leave an empty line; header rows and first code lines lacked `keepNext`; headings in
      quotes did not count as body; `<!-- page break -->` was a directive; code height was overestimated and
      a diagram shown as its source measured 0; wording in the documents; two tests that did not check what
      their names said, and no test that the .docx gets the settings.
    - **Accepted remainder**: a table's header row is typeset three times and its first row twice (not
      quadratic).

20. **Re-review of the fixes (2026-11-04).** Another verifier reviewed the diff of item 19's fixes.
    - **A regression my fix introduced**: "digits in a word holding a letter are an identifier" also caught
      numbers with a unit or with a currency sign after letters (`1,234,567KB`, `100mg`, `+1.5pt`,
      `US$1,234`), which then broke silently (v0.1.1 kept them whole). → An identifier is a word in which a
      letter comes before a digit; nothing before a sign or currency symbol is looked at.
    - The .docx counted a heading in a list as body (the PDF draws the list marker with the heading and does
      not). → Only headings in quotes and alerts count as body.
    - README and get_usage stated keep-with-next's scope differently from the code and the RFP; a table with
      no body rows kept its header with the next paragraph. → Aligned.
    - The .docx page-break output changed after the check in Pages and LibreOffice (item 18). → A person
      checks the changed cases again with documents made for them.

21. **Re-check of the changed .docx page breaks (2026-11-04).** The operator opened the two documents made
    for item 20's changes in Pages and LibreOffice and answered "no problems in all four" (two documents,
    two applications). The items: no blank page after the cover; chapters start a new page; a table after
    `pagebreak` starts at the top of a page with no empty line above it (two tables in a row too); numbers
    with units or currency signs are not broken; at six positions, caption, header row and first row on one
    page; a code heading with the code's first two lines on one page.
