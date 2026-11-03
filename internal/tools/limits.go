package tools

// Limits on one render. The values are phase 0's initial ones
// (docs/ja/phase0-criteria.ja.md, P0-4): a manuscript at every limit was
// predicted to take about 12 s, well inside gem-agent's 60 s call timeout.
const (
	maxManuscriptBytes = 2 << 20
	maxPages           = 500
	maxTableRows       = 2000
	maxTableCols       = 50
	maxTableCells      = 20000
	maxChartPoints     = 10000 // one chart
	maxDocChartPoints  = 50000 // all charts
	maxImageBytes      = 20 << 20
	maxImageSide       = 4096
	maxImages          = 50
	maxDiagrams        = 50
	pixelBudget        = 100_000_000
	maxOutputBytes     = 512 << 20
	defaultMaxWarnings = 50
)
