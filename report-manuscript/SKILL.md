---
name: report-manuscript
description: Write a Markdown report manuscript that report-studio typesets well into a PDF and a Word document — choosing the profile and front matter, writing captions, tables, charts and diagrams the typesetter keeps together, grouping chapters, and revising from its warnings. Use when an investigation, analysis or review should end as a report file, and whenever calling report-studio's render. Also for 報告書・調査報告書・レポートを PDF にする・Word で出す・社外向けの提出資料・原稿の組版・表紙と目次つきの文書・report-studio. Needs the report-studio MCP server.
argument-hint: "<what the report is about, or the manuscript to revise> [profile: standard | formal | compact]"
---

# report-manuscript

report-studio typesets an ordinary Markdown file into a PDF (macOS's own
typesetting) and an editable .docx. The quality of the result is decided almost
entirely by the manuscript: where captions sit, how wide tables are, how long
chapters are, what a diagram's layout is. This skill is how to write that
manuscript and how to act on what report-studio tells you.

Call `get_usage` before the first `render`. It is the contract: the arguments,
the Markdown it accepts, the chart JSON, the profiles, the limits, and every
error and warning with its meaning. This skill does not repeat it. Nor does it
say what a good report should conclude — that is the investigation's, not the
typesetter's.

## The run

1. **Gather** with whatever the work needs (the investigation servers and the
   `mcp-tactics` skill, data analysis, files). Work out the numbers first; the
   manuscript holds results, not raw tool output.
2. **Write the manuscript** as a file under `work_dir`, with its images beside
   it (images from another server's workspace under the same `work_dir` are
   fine; paths are relative to the manuscript).
3. **`render`** with `work_dir`, a `workspace_id` and `source`.
4. **Read every warning.** Each names a manuscript line. Change the manuscript,
   not your expectations, and render again; it overwrites.
5. **Hand over** the paths with the page count, and say what you could not
   check (below).

## 1. Choose the profile and write the front matter first

| Profile | For |
|---|---|
| `standard` | Working documents, drafts, internal notes: no cover, no table of contents |
| `formal` | A deliverable — submitted, sent outside, signed off: cover, table of contents, a new page for each chapter |
| `compact` | Dense reference material where page count matters more than air |

The operator may have added profiles; `get_usage` lists this machine's under
THIS MACHINE. You choose a profile by name, never edit one.

Front matter goes on the very first line:

~~~markdown
---
title: 不審なログイン試行に関する調査報告
subtitle: 2026 年 10 月の事案
author: セキュリティ運用チーム
date: 2026-11-04
classification: 社外秘
profile: formal
---
~~~

- **`classification`** is printed at the top of every page. Set it whenever the
  content is restricted; it is the one marking that survives a page being
  printed alone.
- Without `title`, the first heading becomes the PDF's title. A formal report
  states it.
- `cover`, `toc` and `break_before` override the profile for this document
  only. Prefer choosing the profile that fits; override when one document
  differs.

## 2. Structure for pages, not only for reading

- **Three heading levels.** A fourth looks like the third (`deep_heading`); if
  you need it, the section wants splitting.
- **Chapters cost pages under `formal`.** Every top-level `#` or `##` after body
  text starts a new page. Five chapters of three lines each are five mostly
  empty pages. Group short sections as `###` under one chapter, or render that
  document with `break_before: h1` (only `#` breaks) or `none`.
- **Summary first, and within one page.** A reader who stops after the first
  page should have the conclusion.
- **Put a table or figure right after the paragraph that refers to it**, not
  gathered at the end of a chapter; the reader then never turns a page to find
  it.
- **An appendix chapter last** for the reproduction steps, full queries and long
  listings. Code blocks that long split across pages anyway; in the body they
  break the argument.

## 3. Captions — the form the typesetter recognises

A caption is kept on the same page as what it captions **only if it is written
in one of two forms, directly before** a table, chart, mermaid diagram, image or
code block:

- a paragraph that is **only bold text**, or
- a paragraph starting `表1：`, `図1：`, `Table 1:` or `Figure 1:`.

An italic note line between the caption and its body (a source line) joins the
caption. Numbering is yours: there is no automatic numbering, so number in
order and refer to the numbers in the text.

~~~markdown
本期間の試行は 3 回の急増に集中していた（表1）。

表1：送信元 ASN 別のログイン試行数

*出典: 認証ログ（2026-10-01〜10-31）*

| ASN | 事業者 | 試行数 | 成功 |
|---|---|---:|---:|
| AS64500 | Example Hosting | 12,431 | 0 |
| AS64511 | Example Cloud | 3,208 | 2 |
~~~

A caption written any other way — a plain sentence, a caption after
the table — is ordinary text, and the typesetter may leave it at the foot of one
page with its table on the next. `caption_separated` means the caption and its
body together are taller than a page: shorten or split the body.

## 4. Tables, charts, diagrams, images

**Tables.** Numbers in cells are never broken across lines, so a table's width
is set by its widest numbers.

- `cell_token_split` names the columns that could not keep their numbers whole:
  drop columns, or split the table into two that share a key column.
- `table_wrapped` means the table was set small with wrapped cells. Readable, but
  a narrower table reads better.
- A table longer than a page reads best split by category, each part with its own
  caption.
- Identifiers (hashes, UUIDs, CVE IDs) may wrap; that is expected. Keep
  defanged indicators (`hxxp://`, `example[.]com`) defanged — they are printed
  exactly as written.

**Charts** are drawn by the server from the JSON of a `chart` block (the format is
in `get_usage`). Use one when the shape matters — a trend, a comparison, a
spike; use a table when the reader needs the values. One y axis: two measures
of different scale are two charts. A vertical bar or line chart with more
categories than its axis has room for labels only every second or third one
(`chart_labels_thinned`); a horizontal bar chart labels every category, wrapping
long names, at the cost of height — past a page it is drawn smaller
(`chart_scaled`).

**Mermaid diagrams.** A wide `flowchart LR` with long labels is shrunk until its
text is unreadable (`mermaid_scaled`). Write `flowchart TB`, or shorten the
labels. A diagram type or construct the server cannot draw is shown as its
source (`mermaid_fallback`); the document is still written, so read the warning
rather than assume the figure is there.

**Images** go on a line of their own, with nothing else in the paragraph. An
image inside a paragraph is shown as its caption only (`inline_image`).

## 5. The two directives — escape hatches

Both are an HTML comment alone on its own lines, at the top level of the body:

- `<!-- pagebreak -->` starts a new page there — for example before the appendix
  under `standard`, which has no chapter breaks.
- `<!-- keep-with-next -->` keeps the next block on the same page as the start of
  the one after it — for a sentence that introduces a figure. It does nothing on
  a table, list, quote, alert or raw HTML.

The automatic rules already keep headings and recognised captions with what
follows, and never leave a single line alone on a page. Reach for a directive
only after a render shows a break that the rules did not prevent; a manuscript
sprinkled with them breaks badly as soon as its text changes. Any other HTML is
printed as text (`raw_html`).

## 6. Acting on warnings

The PDF is written even with warnings; each is still worth reading, because it
says the output differs from what you wrote.

| Warning | Change in the manuscript |
|---|---|
| `caption_separated` | Shorten or split what the caption introduces |
| `cell_token_split`, `table_wrapped` | Fewer columns, or two tables |
| `mermaid_scaled` | `flowchart TB`, or shorter labels |
| `mermaid_fallback` | Use a supported diagram type or construct, or replace the diagram with a table |
| `chart_labels_thinned` | Fewer categories, or horizontal bars |
| `chart_scaled` | Fewer categories, or two charts |
| `deep_heading` | Restructure into three levels |
| `inline_image` | Put the image on its own line |
| `raw_html` | Remove it, or write it as Markdown |
| `unclosed_front_matter` | Close the front matter with a `---` line. Until then the whole block is read as text, and the title, profile and classification are all lost |
| `unknown_front_matter_key`, `invalid_front_matter_value`, `invalid_front_matter_line`, `duplicate_front_matter_key` | Fix that front-matter line; until then it is ignored (for a duplicate, the later value wins) |
| `link_scheme` | Only http, https and mailto links are clickable; write anything else as plain text |

An error means the call failed; `get_usage` has what to do for each. A format
written before the failure, or a file from an earlier render, may still be in
the output directory — hand over files only from a call that succeeded.

## 7. Hand over

Give the user the PDF and .docx paths and the page count, and say:

- the page count is the PDF's — Word paginates the .docx itself, so its pages
  can fall differently;
- in the .docx, the table of contents has no page numbers until the field is
  updated in Word;
- any warning you left in place, and why.
