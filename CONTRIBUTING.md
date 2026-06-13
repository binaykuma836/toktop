# Contributing to toktop

Thanks for helping! `toktop` is intentionally small and dependency-light. The
goal: a single binary that shows what your AI coding agents cost, locally, with
zero setup.

## Dev loop

```sh
go test ./...        # unit tests
go test -race ./...  # the store and UI run concurrently — keep this green
go run . --demo      # live UI with synthetic data
go run . --once      # one-shot render against your real logs
```

## High-value contributions

- **Codex / Gemini CLI log samples.** The most useful thing you can send is a
  redacted real session log so we can make those parsers first-class. Open an
  issue with a few sanitized lines (strip prompts/content — we only need the
  `type`, `model`, `timestamp`, and `usage` fields).
- **Pricing updates.** Prices live in [`internal/pricing/prices.toml`](internal/pricing/prices.toml).
  PRs that correct a rate should link the provider's pricing page.
- **New agents.** Implement the `ingest.Source` interface
  ([`internal/ingest/types.go`](internal/ingest/types.go)) and add it in
  `main.go`. Keep parsers *conservative*: never emit a number you're unsure of.

## Ground rules

- `gofmt` everything; `go vet ./...` must pass.
- Add a test for any cost/parse behavior — a wrong dollar figure is the one bug
  that loses trust.
- No network calls. Ever. `toktop` is local-first by definition.

## Recording the demo GIF

We use [vhs](https://github.com/charmbracelet/vhs):

```sh
vhs docs/demo.tape   # writes docs/demo.gif
```
