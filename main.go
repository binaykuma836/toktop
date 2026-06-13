// Command toktop is a single-binary, zero-config TUI that shows what your local
// AI coding agents (Claude Code, and experimentally Codex / Gemini CLI) are
// costing you — live, per session/project/model — with a daily budget bar that
// turns red before your bill does. It reads local session logs read-only and
// never makes a network call.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/furkanalp41/toktop/internal/demo"
	"github.com/furkanalp41/toktop/internal/ingest"
	"github.com/furkanalp41/toktop/internal/pricing"
	"github.com/furkanalp41/toktop/internal/store"
	"github.com/furkanalp41/toktop/internal/ui"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var (
		demoMode bool
		onceMode bool
		byCost   bool
		budget   float64
		showVer  bool
	)
	flag.BoolVar(&demoMode, "demo", false, "stream synthetic data to preview the UI (used for the README GIF)")
	flag.BoolVar(&onceMode, "once", false, "print a one-shot summary and exit (no TTY needed)")
	flag.BoolVar(&byCost, "by-cost", false, "sort sessions by spend instead of recent activity")
	flag.Float64Var(&budget, "budget", 0, "daily budget cap in USD (default: $TOKTOP_BUDGET or 20)")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if showVer {
		fmt.Printf("toktop %s\n", version)
		return
	}

	if budget <= 0 {
		budget = envFloat("TOKTOP_BUDGET", 20)
	}

	table, err := pricing.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "toktop: pricing:", err)
		os.Exit(1)
	}

	st := store.New(func(model string, u pricing.Usage) (float64, bool) {
		return table.Cost(model, u)
	})

	sources := []ingest.Source{
		ingest.NewClaudeSource(),
		ingest.NewCodexSource(),
		ingest.NewGeminiSource(),
	}

	if onceMode {
		st.Add(ingest.Scan(sources))
		w := envInt("COLUMNS", 100)
		h := envInt("LINES", 32)
		fmt.Println(ui.RenderStatic(st, budget, byCost, w, h))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var events <-chan []ingest.Event
	if demoMode {
		ch := make(chan []ingest.Event, 128)
		go demo.Run(ctx, ch)
		events = ch
	} else {
		w := ingest.NewWatcher(sources)
		go w.Run(ctx)
		events = w.Events()
	}

	p := tea.NewProgram(
		ui.New(st, events, budget, demoMode, byCost),
		tea.WithAltScreen(),
	)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "toktop:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `toktop — btop for your AI coding agents

Usage:
  toktop [flags]

Flags:
  --demo            stream synthetic data to preview the UI
  --once            print a one-shot summary and exit (no TTY needed)
  --by-cost         sort sessions by spend instead of recent activity
  --budget <usd>    daily budget cap (default: $TOKTOP_BUDGET or 20)
  --version         print version and exit

Keys:
  j/k  move    s  toggle sort    +/-  adjust budget    q  quit

toktop reads your local agent session logs read-only and never makes a
network call. Edit prices at ~/.config/toktop/pricing.toml.
`)
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil && f > 0 {
			return f
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}
