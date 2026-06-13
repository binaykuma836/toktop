package ui

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// fmtMoney formats a dollar amount with sensible precision for both large
// totals and sub-cent per-session figures.
func fmtMoney(v float64) string {
	if v < 0 {
		v = 0
	}
	switch {
	case v >= 1000:
		return "$" + addThousands(fmt.Sprintf("%.0f", v))
	case v >= 0.01 || v == 0:
		return fmt.Sprintf("$%.2f", v)
	default:
		return fmt.Sprintf("$%.4f", v)
	}
}

// fmtTokens formats a token count compactly (1.2M, 345.0K, 42).
func fmtTokens(n int64) string {
	f := float64(n)
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", f/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", f/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", f/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func addThousands(s string) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	n := len(s)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		if n > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	out := b.String()
	if neg {
		return "-" + out
	}
	return out
}

// sparkline renders a unicode sparkline scaled to the max value. Zeros render
// as blank, tiny non-zero values render as the lowest bar.
func sparkline(vals []float64) string {
	if len(vals) == 0 {
		return ""
	}
	max := 0.0
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range vals {
		if v <= 0 || max <= 0 {
			b.WriteRune(' ')
			continue
		}
		idx := int(math.Round(v / max * float64(len(sparkRunes)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		b.WriteRune(sparkRunes[idx])
	}
	return b.String()
}

// relTime renders a short relative age like "now", "12s", "5m", "3h", "2d".
func relTime(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < 3*time.Second:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// shortModel trims a verbose model id to its meaningful tail for display.
func shortModel(m string) string {
	if m == "" {
		return "—"
	}
	m = strings.TrimPrefix(m, "claude-")
	m = strings.TrimPrefix(m, "models/")
	return m
}

// truncate shortens s to width runes, adding an ellipsis if cut.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

// padRight pads s with spaces to width (no truncation).
func padRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}

// rightAlign pads s on the left with spaces to width (no truncation).
func rightAlign(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(r)) + s
}
