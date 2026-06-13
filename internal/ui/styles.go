package ui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette (GitHub-dark inspired, works on any 256-color terminal).
var (
	colFg     = lipgloss.Color("#e6edf3")
	colMuted  = lipgloss.Color("#7d8590")
	colFaint  = lipgloss.Color("#484f58")
	colGreen  = lipgloss.Color("#3fb950")
	colAmber  = lipgloss.Color("#d29922")
	colRed    = lipgloss.Color("#f85149")
	colBlue   = lipgloss.Color("#58a6ff")
	colViolet = lipgloss.Color("#bc8cff")
	colBorder = lipgloss.Color("#30363d")
)

var (
	styTitle    = lipgloss.NewStyle().Foreground(colViolet).Bold(true)
	styTagline  = lipgloss.NewStyle().Foreground(colMuted)
	styLabel    = lipgloss.NewStyle().Foreground(colMuted)
	styFaint    = lipgloss.NewStyle().Foreground(colFaint)
	styValue    = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	styAccent   = lipgloss.NewStyle().Foreground(colBlue)
	styGreen    = lipgloss.NewStyle().Foreground(colGreen)
	styAmber    = lipgloss.NewStyle().Foreground(colAmber)
	styRed      = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	stySpark    = lipgloss.NewStyle().Foreground(colBlue)
	stySel      = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	stySelMark  = lipgloss.NewStyle().Foreground(colViolet).Bold(true)
	stySection  = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	styBudgetK  = lipgloss.NewStyle().Foreground(colMuted).Bold(true)
	styEstimate = lipgloss.NewStyle().Foreground(colAmber)
)

// budgetColor returns the style appropriate to how close spend is to the cap.
func budgetColor(frac float64) lipgloss.Style {
	switch {
	case frac >= 1.0:
		return styRed
	case frac >= 0.85:
		return styAmber
	case frac >= 0.7:
		return styAmber
	default:
		return styGreen
	}
}

// renderBar draws a fixed-width budget bar coloured by fraction of cap used.
func renderBar(frac float64, width int) string {
	if width < 4 {
		width = 4
	}
	style := budgetColor(frac)
	f := frac
	if f > 1 {
		f = 1
	}
	if f < 0 {
		f = 0
	}
	fill := int(math.Round(f * float64(width)))
	if frac > 0 && fill == 0 {
		fill = 1
	}
	if fill > width {
		fill = width
	}
	filled := style.Render(strings.Repeat("█", fill))
	empty := styFaint.Render(strings.Repeat("░", width-fill))
	return filled + empty
}

// hrule returns a faint horizontal rule of the given width.
func hrule(width int) string {
	if width < 1 {
		return ""
	}
	return styFaint.Render(strings.Repeat("─", width))
}
