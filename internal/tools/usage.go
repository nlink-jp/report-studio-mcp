package tools

// usage is get_usage's text: how to use the tool, not how to write a good
// report (that is the support skill's).
const usage = `report-studio-mcp — reference

WHAT IT DOES
  Typesets a Markdown manuscript into a PDF with macOS's own typesetting (Hiragino fonts,
  Japanese line-breaking rules) and writes the same content as an editable Word document
  (.docx) whose styles follow the profile. No browser, no network. Charts and mermaid diagrams are drawn
  by the server from what you write; you never write drawing code.

WORKFLOW
  1. Write the manuscript as a Markdown file under work_dir (any name, e.g. report/report.md).
     Put images it uses under work_dir too (another server's workspace is fine).
  2. Call render {work_dir, workspace_id, source}.
  3. Read the warnings; fix the manuscript if one matters; render again (it overwrites).

TOOLS
  render
    work_dir      required. Absolute directory you can read back.
    workspace_id  required. Name of the workspace for the output: letters, digits, . _ -
    source        required. Manuscript path relative to work_dir.
    formats       optional. ["pdf", "docx"] (default both), or either alone.
    profile       optional. A profile name (default: front matter "profile", else "standard");
                  the list is under THIS MACHINE below.
    max_warnings  optional. Warnings returned in full (default 50); the rest are counted.
  Result: {work_dir, outputs:[{format, path, abs_path, bytes}], pages, warnings:[{line, code,
  message}], warnings_total, warnings_omitted}. Outputs are <workspace_id>/out/<source without
  extension>.pdf and .docx, so a/report.md and b/report.md do not collide. pages counts the
  PDF; Word paginates the .docx itself. In the .docx, the table of contents lists the
  headings without page numbers (Word fills them in when the field is updated).

MANUSCRIPT
  Ordinary Markdown: CommonMark 0.31.2 plus GitHub's tables, strikethrough, task lists and
  autolinks; footnotes ([^1] and "[^1]: text"); alerts ("> [!NOTE]", TIP, IMPORTANT, WARNING,
  CAUTION). Optional front matter, first line exactly "---", then "key: value" lines, then
  "---". Keys: title, subtitle, author, date, classification (shown at the top of every page,
  e.g. 社外秘), profile, cover and toc (true/false: override the profile's cover page and
  table of contents). Without a title, the first heading is the PDF's title.
  Headings: levels 1-3 (deeper ones look like 3). Images: ![caption](relative/path.png) on a
  line of their own; PNG or JPEG; the path is relative to the manuscript. Links: only http,
  https and mailto become clickable; other schemes are shown as text. Defanged indicators
  (hxxp://, example[.]com) are shown exactly as written — keep them defanged.

CHARTS  (a fenced block whose info string is "chart"; its content is JSON)
  {"type": "bar" | "line" | "scatter",
   "title": "...",                          optional
   "orientation": "vertical" | "horizontal", bar only, default vertical
   "stacked": true,                          bar only; values must not be negative
   "x": {"label": "...", "categories": ["...", ...]},   bar and line: one category per value
   "y": {"label": "...", "min": 0, "max": 100},          min/max optional; values outside are errors
   "series": [{"name": "...", "values": [1, null, 3]}]}  null leaves a gap
  Scatter: no categories; series take "points": [[x, y], ...]; x may have min/max too.
  Series: up to 8 (bar, line) or 3 (scatter) — more colours cannot be told apart; split the
  chart. Values and fixed axis ends must lie within ±1e15 (scale large counts or timestamps);
  labels, categories and series names are at most 200 characters, a title 300. With two or more series each needs a distinct name (the legend shows it). One y axis
  only: two measures of different scale belong in two charts.

MERMAID  (a fenced block whose info string is "mermaid")
  flowchart/graph, sequenceDiagram, erDiagram, stateDiagram(-v2), gantt, pie, mindmap. A diagram
  that cannot be drawn (another type, emoji in labels, a construct not supported) is shown as
  its source with a warning — the document is still written.

PROFILES
  Built in: standard (A4, Hiragino Sans, no cover or contents), formal (cover, table of
  contents, Hiragino Mincho body, wider margins), compact (narrow margins, smaller type).
  The operator may add profiles in config.toml ($XDG_CONFIG_HOME, ~/.config or
  ~/Library/Application Support, under report-studio-mcp/): a [profiles.NAME] table with
  extends = "standard" | "formal" | "compact" and any of paper ("A4"/"Letter"), margin_mm,
  margin_top_mm/bottom/left/right, body_font, body_bold_font, heading_font, code_font
  (PostScript names), body_size, code_size, table_size, table_min_size, note_size,
  title_size, heading_sizes (three), line_gap, text_color, muted_color, link_color,
  rule_color, code_background, chart_palette (#rrggbb), cover, toc, toc_title, logo
  (absolute path of a PNG/JPEG for the cover). You choose a profile by name; you cannot
  change one.

LIMITS
  manuscript 2 MiB; 500 pages; a table 2,000 rows and 50 columns, 20,000 cells in all; a chart
  10,000 data points, 50,000 in all; 50 images, each at most 4096x4096 and 20 MiB; 50 mermaid
  diagrams; 100,000,000 pixels of images and diagrams together; 512 MiB of
  output; 45 seconds per call.

ERRORS  (code — meaning — what to do)
  work_dir_required / work_dir_invalid / work_dir_not_found / work_dir_not_writable /
  work_dir_denied — the work directory cannot be used — pass an absolute directory you own.
  invalid_arguments — an argument is malformed or unknown — fix it as the message says.
  missing_argument — source is missing — pass the manuscript path.
  file_not_found — the manuscript or an image is not there — check the path (relative to
  work_dir for source, to the manuscript for images).
  path_not_allowed — a path leaves work_dir, is a protected place, has a second name (a hard
  link) or an output directory on its path is a link (details.reason) — use a plain file and
  plain directories under work_dir.
  image_invalid — not a PNG/JPEG, unreadable, or a URL — use a PNG or JPEG file.
  chart_invalid — the chart JSON cannot be drawn faithfully; details.line is the manuscript
  line — fix the JSON as the message says.
  limit_exceeded — over a limit (details.limit, details.max; "time" when the call would
  outlast its 45 s) — split or shrink the content.
  internal_error — the server failed unexpectedly on this input and is still running — report
  it to the operator with the manuscript.
  profile_unknown — no such profile — use one listed under THIS MACHINE.
  config_invalid — the operator's config.toml cannot be used — tell the operator; the
  message names the file and the problem.
  logo_invalid — the profile's cover logo cannot be used — tell the operator, or render
  with cover: false.
  font_missing — a profile names a font that is not installed.

WARNINGS  (the PDF is written; each names its manuscript line)
  raw_html — HTML is shown as text. deep_heading — a level 4-6 heading looks like level 3.
  unknown_front_matter_key / invalid_front_matter_line / invalid_front_matter_value — that
  front matter line is ignored.
  link_scheme — a link that is not http/https/mailto is shown as text.
  inline_image — an image inside a paragraph is shown as its caption; give it its own line.
  mermaid_fallback — the diagram is shown as its source.
  chart_labels_thinned — too many categories to label each one.
  chart_scaled — a chart taller than a page is drawn smaller to fit.
  unclosed_front_matter — the first line is --- but no closing --- follows; it is read as text.
  duplicate_front_matter_key — a key is given twice; the later value is used.
  control_characters — characters a .docx cannot carry were removed from it.
  table_wrapped — a wide table is set small and its cells wrap.
`
