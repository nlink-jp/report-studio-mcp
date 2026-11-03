package pdf

import (
	"github.com/nlink-jp/report-studio-mcp/internal/chart"
	"github.com/nlink-jp/report-studio-mcp/internal/theme"
)

// The typesetter works in the shared theme's terms.
type (
	Theme     = theme.Theme
	RGB       = theme.RGB
	AlertLook = theme.AlertLook
)

// StandardTheme is the built-in "standard" profile.
func StandardTheme() Theme { return theme.Standard() }

func toRGB(c chart.Color) RGB { return theme.ToRGB(c) }
