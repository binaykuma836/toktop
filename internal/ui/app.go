package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/furkanalp41/toktop/internal/ingest"
	"github.com/furkanalp41/toktop/internal/store"
)

const (
	sparkSnapshot = 24 // buckets fetched per snapshot
	sessSparkW    = 8  // sparkline width in the session list
	tickEvery     = 350 * time.Millisecond
)

type eventsMsg []ingest.Event
type tickMsg time.Time
type closedMsg struct{}

// Model is the root Bubble Tea model for toktop.
type Model struct {
	store  *store.Store
	events <-chan []ingest.Event
	budget float64
	demo   bool

	width, height int
	ready         bool
	selected      int
	sortByCost    bool

	snap store.Snapshot
	now  time.Time

	prevToday  float64
	flashUntil time.Time
}

// New builds the root model.
func New(st *store.Store, events <-chan []ingest.Event, budget float64, demo, sortByCost bool) Model {
	return Model{store: st, events: events, budget: budget, demo: demo, sortByCost: sortByCost, now: time.Now()}
}

// RenderStatic renders a single non-interactive frame for `toktop --once`.
func RenderStatic(st *store.Store, budget float64, sortByCost bool, width, height int) string {
	m := New(st, nil, budget, false, sortByCost)
	m.width, m.height, m.ready = width, height, true
	m.now = time.Now()
	m.refresh()
	return m.View()
}

// Init starts the event pump and the render ticker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(waitEvents(m.events), tickCmd())
}

func waitEvents(ch <-chan []ingest.Event) tea.Cmd {
	return func() tea.Msg {
		evs, ok := <-ch
		if !ok {
			return closedMsg{}
		}
		return eventsMsg(evs)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func bellCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Fprint(os.Stderr, "\a")
		return nil
	}
}

// Update handles incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case eventsMsg:
		m.store.Add([]ingest.Event(msg))
		m.now = time.Now()
		m.refresh()
		cmd := m.checkBudget()
		return m, tea.Batch(waitEvents(m.events), cmd)

	case closedMsg:
		m.now = time.Now()
		m.refresh()
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		m.refresh()
		return m, tickCmd()
	}
	return m, nil
}

func (m *Model) refresh() {
	m.snap = m.store.Snapshot(m.now, m.sortByCost, sparkSnapshot)
	if m.selected >= len(m.snap.Sessions) {
		m.selected = len(m.snap.Sessions) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// checkBudget fires a one-shot flash + terminal bell the moment today's spend
// crosses the configured daily cap.
func (m *Model) checkBudget() tea.Cmd {
	var cmd tea.Cmd
	if m.budget > 0 && m.prevToday < m.budget && m.snap.Today >= m.budget {
		m.flashUntil = m.now.Add(1300 * time.Millisecond)
		cmd = bellCmd()
	}
	m.prevToday = m.snap.Today
	return cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "j", "down":
		if m.selected < len(m.snap.Sessions)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "g", "home":
		m.selected = 0
	case "G", "end":
		m.selected = len(m.snap.Sessions) - 1
	case "s":
		m.sortByCost = !m.sortByCost
		m.refresh()
	case "+", "=":
		m.budget += budgetStep(m.budget)
	case "-", "_":
		m.budget -= budgetStep(m.budget)
		if m.budget < 0 {
			m.budget = 0
		}
	}
	return m, nil
}

func budgetStep(b float64) float64 {
	if b < 5 {
		return 0.5
	}
	return 1
}

func (m Model) budgetFrac() float64 {
	if m.budget <= 0 {
		return 0
	}
	return m.snap.Today / m.budget
}

// View renders the whole dashboard.
func (m Model) View() string {
	if !m.ready || m.width < 24 || m.height < 10 {
		return "starting toktop…"
	}
	W := m.width

	top := strings.Join([]string{
		m.renderHeader(W),
		hrule(W),
		m.renderSummary(W),
		m.renderBudget(W),
	}, "\n")
	if banner := m.renderBanner(W); banner != "" {
		top += "\n" + banner
	}
	top += "\n" + hrule(W)

	bottom := hrule(W) + "\n" + m.renderFooter(W)

	bodyRows := m.height - lineCount(top) - lineCount(bottom)
	if bodyRows < 3 {
		bodyRows = 3
	}

	// Two columns when there's room, otherwise a single sessions column so we
	// never compute a negative width.
	var body string
	if W >= 64 {
		rightW := clamp(int(0.34*float64(W)), 22, 40)
		leftW := W - rightW - 3
		if leftW < 20 {
			leftW = 20
		}
		left := lipgloss.NewStyle().Width(leftW).Height(bodyRows).MaxHeight(bodyRows).Render(m.renderSessions(leftW, bodyRows))
		right := lipgloss.NewStyle().Width(rightW).Height(bodyRows).MaxHeight(bodyRows).Render(m.renderBreakdown(rightW, bodyRows))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
	} else {
		body = lipgloss.NewStyle().Width(W).Height(bodyRows).MaxHeight(bodyRows).Render(m.renderSessions(W, bodyRows))
	}

	return top + "\n" + body + "\n" + bottom
}

func (m Model) renderHeader(width int) string {
	left := styTitle.Render("toktop") + "  " + styTagline.Render("btop for AI coding agents")
	clock := styValue.Render(m.now.Format("15:04:05"))
	right := styGreen.Render("●") + " " + styLabel.Render("live") + "  " + clock
	if m.demo {
		right += "  " + styAmber.Render("DEMO")
	}
	return spread(left, right, width)
}

func (m Model) renderSummary(width int) string {
	burn := stySpark.Render(sparkline(lastNf(m.snap.Spark, burnWidth(width))))
	left := styLabel.Render("TODAY ") + budgetColor(m.budgetFrac()).Render(fmtMoney(m.snap.Today)) +
		"   " + styFaint.Render("burn ") + burn
	right := styLabel.Render("WEEK ") + styValue.Render(fmtMoney(m.snap.Week)) +
		"    " + styLabel.Render("ALL-TIME ") + styValue.Render(fmtMoney(m.snap.Total))
	return spread(left, right, width)
}

func (m Model) renderBudget(width int) string {
	frac := m.budgetFrac()
	label := styBudgetK.Render("DAILY BUDGET")
	pct := budgetColor(frac).Render(fmt.Sprintf("%3.0f%%", frac*100))
	right := budgetColor(frac).Render(fmtMoney(m.snap.Today)) + styFaint.Render(" / ") +
		styValue.Render(fmtMoney(m.budget)) + "  " + pct
	barW := width - lipgloss.Width(label) - lipgloss.Width(right) - 4
	if barW < 10 {
		return label + "  " + right + "\n" + renderBar(frac, width)
	}
	return label + "  " + renderBar(frac, barW) + "  " + right
}

func (m Model) renderBanner(width int) string {
	if m.budget <= 0 || m.snap.Today < m.budget {
		return ""
	}
	over := m.snap.Today - m.budget
	txt := fmt.Sprintf(" ⚠  DAILY BUDGET EXCEEDED  ·  %s over %s ", fmtMoney(over), fmtMoney(m.budget))
	if m.now.Before(m.flashUntil) {
		flash := lipgloss.NewStyle().Foreground(lipgloss.Color("#0b0f14")).Background(colRed).Bold(true)
		return flash.Render(padRight(txt, width))
	}
	return styRed.Render(txt)
}

func (m Model) renderSessions(width, rows int) string {
	var b strings.Builder
	head := stySection.Render("SESSIONS") + " " + styFaint.Render(sortLabel(m.sortByCost))
	b.WriteString(head + "\n")

	if len(m.snap.Sessions) == 0 {
		b.WriteString(styFaint.Render("  waiting for agent activity…"))
		return b.String()
	}

	moneyW, sparkW, ageW := 9, sessSparkW, 4
	nameW := width - (2 + moneyW + sparkW + ageW + 3)
	if nameW < 8 {
		nameW = 8
	}

	listRows := rows - 1
	if listRows < 1 {
		listRows = 1
	}
	start, end := windowRange(m.selected, len(m.snap.Sessions), listRows)

	for i := start; i < end; i++ {
		s := m.snap.Sessions[i]
		marker := "  "
		nameStyle := styValue
		if i == m.selected {
			marker = stySelMark.Render("▸ ")
			nameStyle = stySel
		}
		name := nameStyle.Render(padRight(truncate(sessionLabel(s), nameW), nameW))
		money := moneyStyle(s.Estimated).Render(rightAlign(fmtMoney(s.Cost), moneyW))
		spark := stySpark.Render(padRight(sparkline(lastNf(s.Spark, sparkW)), sparkW))
		age := styFaint.Render(rightAlign(relTime(s.Last, m.now), ageW))
		b.WriteString(marker + name + " " + money + " " + spark + " " + age)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) renderBreakdown(width, rows int) string {
	var b strings.Builder
	modelRows := (rows - 3) / 2
	if modelRows < 1 {
		modelRows = 1
	}
	projRows := rows - 3 - modelRows
	if projRows < 1 {
		projRows = 1
	}
	b.WriteString(stySection.Render("BY MODEL") + "\n")
	b.WriteString(renderAgg(m.snap.Models, width, modelRows))
	b.WriteString("\n" + stySection.Render("BY PROJECT") + "\n")
	b.WriteString(renderAgg(m.snap.Projects, width, projRows))
	return b.String()
}

func renderAgg(items []store.AggView, width, rows int) string {
	if rows < 1 {
		rows = 1
	}
	if len(items) == 0 {
		return styFaint.Render("  —")
	}
	moneyW, pctW := 8, 4
	nameW := width - moneyW - pctW - 2
	if nameW < 6 {
		nameW = 6
	}
	// Reserve the last row for a "+N more" line when items overflow, so nothing
	// gets clipped by the column's MaxHeight. Always show at least one item.
	shown := len(items)
	overflow := false
	if len(items) > rows {
		shown = rows - 1
		overflow = true
	}
	if shown < 1 {
		shown = 1
		overflow = false
	}
	var b strings.Builder
	wrote := false
	for i := 0; i < shown; i++ {
		it := items[i]
		name := styValue.Render(padRight(truncate(shortModel(it.Name), nameW), nameW))
		money := styAccent.Render(rightAlign(fmtMoney(it.Cost), moneyW))
		pct := styFaint.Render(rightAlign(fmt.Sprintf("%.0f%%", it.Share*100), pctW))
		if wrote {
			b.WriteString("\n")
		}
		b.WriteString(name + " " + money + " " + pct)
		wrote = true
	}
	if overflow {
		if wrote {
			b.WriteString("\n")
		}
		b.WriteString(styFaint.Render(fmt.Sprintf("  +%d more", len(items)-shown)))
	}
	return b.String()
}

func (m Model) renderFooter(width int) string {
	keys := keyHint("j/k", "move") + "  " + keyHint("s", "sort") + "  " +
		keyHint("+/-", "budget") + "  " + keyHint("q", "quit")
	stat := fmt.Sprintf("%d sessions · %s tokens · %d responses",
		m.snap.SessionCount, fmtTokens(m.snap.TotalUsage.Total()), m.snap.EventCount)
	statStr := styFaint.Render(stat)
	if m.snap.HasEstimated {
		statStr = styEstimate.Render("~est ") + statStr
	}
	return spread(keys, statStr, width)
}

// ---- small helpers ----

func keyHint(k, label string) string {
	return styFaint.Render(k) + " " + styLabel.Render(label)
}

func sortLabel(byCost bool) string {
	if byCost {
		return "(by spend)"
	}
	return "(by recent)"
}

func sessionLabel(s store.SessionView) string {
	name := s.Project
	if name == "" {
		name = s.ShortID
	}
	if s.Agent != "" && s.Agent != "claude" {
		name = s.Agent + ":" + name
	}
	return name
}

func moneyStyle(estimated bool) lipgloss.Style {
	if estimated {
		return styAmber
	}
	return styValue
}

func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func windowRange(selected, n, rows int) (int, int) {
	if n <= rows {
		return 0, n
	}
	start := selected - rows/2
	if start < 0 {
		start = 0
	}
	if start > n-rows {
		start = n - rows
	}
	return start, start + rows
}

func lastNf(s []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	if n >= len(s) {
		return s
	}
	return s[len(s)-n:]
}

func burnWidth(width int) int {
	w := width / 5
	if w > sparkSnapshot {
		w = sparkSnapshot
	}
	if w < 6 {
		w = 6
	}
	return w
}

func lineCount(s string) int { return strings.Count(s, "\n") + 1 }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
