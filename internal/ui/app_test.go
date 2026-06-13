package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/furkanalp41/toktop/internal/ingest"
	"github.com/furkanalp41/toktop/internal/pricing"
	"github.com/furkanalp41/toktop/internal/store"
)

func testModel(t *testing.T, budget float64) Model {
	t.Helper()
	tb, err := pricing.Load()
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(func(m string, u pricing.Usage) (float64, bool) { return tb.Cost(m, u) })
	return New(st, nil, budget, true, true)
}

func sized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return nm.(Model)
}

func TestViewRendersSections(t *testing.T) {
	m := sized(t, testModel(t, 5), 100, 30)
	out := m.View()
	for _, want := range []string{"toktop", "DAILY BUDGET", "BY MODEL", "BY PROJECT", "SESSIONS"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered view is missing %q", want)
		}
	}
}

func TestNotReadyShowsSplash(t *testing.T) {
	m := testModel(t, 5)
	if !strings.Contains(m.View(), "starting") {
		t.Error("before a window size, the view should show the splash")
	}
}

func TestBudgetExceededBanner(t *testing.T) {
	m := sized(t, testModel(t, 0.0001), 100, 30)
	e := ingest.Event{ID: "x", Model: "claude-opus-4-8", SessionID: "s", Time: time.Now(), Usage: ingest.Usage{Output: 1_000_000}}
	nm, _ := m.Update(eventsMsg{e})
	m = nm.(Model)
	out := m.View()
	if !strings.Contains(out, "EXCEEDED") {
		t.Error("crossing the budget should render the EXCEEDED banner")
	}
}

func TestBudgetCrossFiresBell(t *testing.T) {
	m := sized(t, testModel(t, 0.0001), 100, 30)
	e := ingest.Event{ID: "x", Model: "claude-opus-4-8", SessionID: "s", Time: time.Now(), Usage: ingest.Usage{Output: 1_000_000}}
	_, cmd := m.Update(eventsMsg{e})
	if cmd == nil {
		t.Error("crossing the budget should return a (batched) command including the bell")
	}
}

func TestQuitKey(t *testing.T) {
	m := sized(t, testModel(t, 5), 80, 24)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("pressing q should return tea.Quit")
	}
}

func TestSortToggle(t *testing.T) {
	m := sized(t, testModel(t, 5), 80, 24)
	before := m.sortByCost
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if nm.(Model).sortByCost == before {
		t.Error("pressing s should toggle the sort mode")
	}
}

func TestRenderNeverPanicsAtAnySize(t *testing.T) {
	base := testModel(t, 5)
	e := ingest.Event{ID: "x", Model: "claude-opus-4-8", SessionID: "s", Project: "p", Time: time.Now(), Usage: ingest.Usage{Output: 500000}}
	for _, wh := range [][2]int{{10, 5}, {24, 10}, {40, 12}, {63, 20}, {64, 20}, {200, 60}} {
		m := sized(t, base, wh[0], wh[1])
		nm, _ := m.Update(eventsMsg{e})
		m = nm.(Model)
		_ = m.View() // must not panic
	}
}

func TestMoneyFormatting(t *testing.T) {
	cases := map[float64]string{
		0:        "$0.00",
		0.0034:   "$0.0034",
		4.2:      "$4.20",
		1234.4:   "$1,234",
		12345.67: "$12,346",
	}
	for in, want := range cases {
		if got := fmtMoney(in); got != want {
			t.Errorf("fmtMoney(%v) = %q, want %q", in, got, want)
		}
	}
}
