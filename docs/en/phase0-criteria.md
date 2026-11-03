# Phase 0: Pass criteria (pre-registered)

> Registered: 2026-11-03 (committed before measuring; the criteria are not rewritten afterwards)
> Basis: RFP §4 Phase 0, org ADR-023

Each item is recorded as pass, fail or unmeasurable. A fail sends us back to the RFP's design
decisions. Unmeasurable is never read as pass: the reason and the next step are written down. Probe
code used for measuring is kept outside the repository; only the results and how to reproduce them go
into "Results" below.

## P0-1 PDF from Go through cgo with Core Text and Core Graphics

A Go program built under the same conditions as `make build` (`CGO_ENABLED=1`, darwin/arm64) lays out
a fixed test text on one A4 PDF page: one heading line in Hiragino Sans W6 and one W3 body paragraph
(at least 600 characters, including line-final commas, full stops, closing and opening brackets, and
mixed Latin letters and digits).

Pass when all hold:

1. Fonts are embedded as subsets: each font's `BaseFont` carries a prefix of six capital letters and
   `+`, and `FontFile2` or `FontFile3` is present. The one-page PDF is under 100 KB.
2. No character is lost: the text PDFKit extracts, with spaces and line breaks removed, equals the test
   text with spaces and line breaks removed.
3. Kinsoku holds: the first character of every line Core Text returns (`CTLine`) is not one of the
   characters forbidden at line start (`、。，．）」』】〕〉》！？ー・`).
4. stdout stays clean: with the program's stdout and stderr sent to separate files, the stdout file is
   0 bytes (the frameworks do not write to stdout while laying out).
5. The minimum OS is explicit: `vtool -show-build` reports a `minos` equal to the value set in the
   build. The value is chosen at measurement time and recorded here.

## P0-2 A minimal hand-written .docx in Word and Pages

One .docx written with the standard library only (`archive/zip`, `encoding/xml` or strings), holding
three heading styles (Japanese), a body paragraph, a 3×3 table, one PNG image, one footnote and one
TOC field.

Pass when all hold:

1. Word for Mac opens it without a repair dialog.
2. Pages opens it without a repair or import warning (a Pages notice that some elements were
   converted is recorded and passes if no content is missing).
3. In both, all six elements above are visible (a person checks and records each element).
4. Body text read back with `textutil -convert txt` contains every character of the paragraphs and the
   table.

This machine has neither Word nor Pages (checked 2026-11-03). If an application cannot be provided,
record unmeasurable and agree an alternative with the operator. Pages alone does not count as pass.

## P0-3 mermaid diagrams embedded in a PDF

The 51 real diagrams in mermaid-render's `testdata/real` are drawn at the scale that gives 300 dpi when
shown at the A4 body width (20 mm left and right margins), and embedded in a PDF as PNG.

Record: pixels, PNG size and PDF size increase per diagram; how many hit the pixel limit (mermaid-render's
default) and whether they could be drawn at a lower scale.

Pass when:

1. Every one of the 51 yields a figure or a "cannot draw" result (the engine neither hangs nor crashes).
2. The median PDF size increase is at most 500 KB, the maximum at most 4 MB.
3. Diagrams that hit the limit can be drawn at a lower scale.

## P0-4 Worst-case time

There is no layout engine yet, so the raw cost of Core Text and Core Graphics is measured:
(a) laying out 300 pages of body text, (b) drawing 20,000 rectangles and 20,000 short strings (standing
in for large tables and charts), (c) decoding and embedding fifty 2,000 × 2,000 PNGs.

Pass when: initial limits are set from the measurements such that the predicted time for a manuscript
at every limit is at most 20 s (a third of gem-agent's 60 s default call timeout). The chosen limits are
recorded here.

## P0-5 Specification versions

Record: the CommonMark spec version, the GFM spec version, the sources for footnotes and callouts
(GitHub Docs pages and the date read), and the Office Open XML (ECMA-376) edition and conformance class
(Transitional).

Pass when: all four are recorded here and referenced from RFP §2 "Specifications followed".

## Results

### P0-1: pass (2026-11-03, macOS 27.2, Go 1.27.3, Apple Silicon)

Probe: a Go program (about 130 lines) whose cgo C part draws `CTFramesetter` → `CTFrameDraw` into a
`CGPDFContext` and returns the first character of each `CTLine`, plus ten lines of Swift that extract
the text with PDFKit's `PDFDocument.string`.

1. Subset embedding: `AAAAAB+HiraginoSans-W6` and `AAAAAC+HiraginoSans-W3` (`CIDFontType0`,
   `FontFile3`). The PDF is 32,668 bytes. — pass
2. Lost characters: the heading and the 600-character body, with spaces and line breaks removed, equal
   PDFKit's extraction. — pass
3. Kinsoku: of 13 body lines, 0 start with a character forbidden at line start.
   `CTFrameGetVisibleStringRange` also confirms the whole text fit the frame. — pass
4. stdout: run with stdout and stderr in separate files, stdout is 0 bytes. — pass
5. Minimum OS: set to 26.0 (a macOS 26 verification environment exists). `vtool -show-build` reports
   `minos 26.0`. — pass

Note (build condition found): passing `-mmacosx-version-min=26.0` in `CGO_CFLAGS` and `CGO_LDFLAGS`
alone left Go's runtime cgo objects built for the build machine's OS (27.2), and the linker warned of
objects "built for newer macOS". Setting `MACOSX_DEPLOYMENT_TARGET=26.0` in the environment and
rebuilding everything brought the warnings to 0. The Makefile pins this variable.

### P0-2: unmeasurable (no Word) — pass in Pages and in LibreOffice as the alternative (2026-11-03)

Probe: a .docx written with the standard library only (`archive/zip` and strings). Parts:
`[Content_Types].xml`, `_rels/.rels`, `docProps/core.xml`, `word/{document,styles,settings,footnotes}.xml`,
`word/_rels/document.xml.rels`, `word/media/chart.png`. Font: Hiragino Sans. The TOC field is written
with its entries filled in, so readers that never update fields still show them, and `updateFields` is
set.

| Criterion | Result |
|---|---|
| 1. Word for Mac | **Unmeasurable**: Word is not on this machine. Agreed with the operator (2026-11-03) to check in LibreOffice 26.8.2.2 instead. The Word check remains open |
| 2. Pages (Pages Creator Studio 15.4) | Pass: the operator reported no repair or import warning |
| 3. All six elements visible | Pass in both Pages and LibreOffice: 3-line TOC, three heading levels, Japanese body text, 3×3 table, image, footnote (superscript mark in the body and the note at the page foot). Checked by the operator from screenshots |
| 4. `textutil` read-back | Pass: every character of the paragraphs and the table is present. The footnote text is not read back (a `textutil` limit; outside the criterion) |

Zip integrity (`unzip -t`) and well-formedness of every XML part (`xmllint --noout`) were also checked.

Observed (appearance only; not part of pass/fail):

- Pages regenerated the TOC on opening and put page numbers right after the headings ("調査の概要1"),
  because the TOC styles have no right-aligned tab with a leader. Phase 2 defines tabs in the TOC styles.
- LibreOffice showed the TOC inside a field frame and did not regenerate it.

Note on method: converting to PDF with `soffice --headless` from this terminal used none of the system
fonts, Helvetica included (it fell back to bundled fonts such as LinuxLibertineG). This was judged a
limitation of the execution environment, not of the document, and was not used as evidence. Using
headless conversion in tests needs this solved first.
