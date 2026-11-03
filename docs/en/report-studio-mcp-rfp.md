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
  `profile`, cover page and table of contents on/off. All optional; an unknown key is a warning.
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
- Raw HTML is shown as text; headings deeper than level 3 take the level-3 look.
- A wide table first shrinks its type down to a floor, then wraps inside cells. Unbreakable long lines
  (code, URLs) are force-wrapped.
- Control characters that XML (.docx) cannot carry are removed.

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
(title, classification, page number); cover page and table of contents; a logo image for the cover.
Text inside mermaid diagrams stays Hiragino for now (mermaid-render loads fonts through a different
mechanism from Core Text, and some faces cannot be read by it).

A configuration mistake (a font that does not exist, say) does not stop the server. A `render` that uses
that profile returns an error naming the cause (with a `code`); other profiles keep working.

Built-in profiles (compiled in, not editable):

| Name | Use | Look |
|---|---|---|
| `standard` (default) | Everyday investigation reports | A4, Hiragino Sans, no cover or TOC, page number in the footer |
| `formal` | Formal internal documents | Cover and TOC, Hiragino Mincho body, classification in the header |
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

### Open issues (handled in other projects; org ADR-025)

Not steps of this plan; an issue is filed in each project:

- skills-series/mcp-tactics: add the new server to the index and the description (both the body and the
  trigger line).
- skills-series: the support skill for writing reports (its own RFP).

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
