package ingest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CodexSource parses OpenAI Codex CLI session logs (EXPERIMENTAL).
//
// The format is best-effort and intentionally conservative: a line only counts
// if it carries an explicit token-usage object, so an unexpected schema simply
// yields nothing rather than a wrong number. Expected shape per JSONL line:
//
//	{"id":"...","model":"gpt-5-codex","timestamp":"...","cwd":"...",
//	 "usage":{"input_tokens":N,"output_tokens":N,"cached_input_tokens":N}}
type CodexSource struct{ root string }

func NewCodexSource() *CodexSource {
	root := os.Getenv("TOKTOP_CODEX_DIR")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".codex", "sessions")
		}
	}
	return &CodexSource{root: root}
}

func (c *CodexSource) Name() string { return "codex" }

func (c *CodexSource) Roots() []string {
	if c.root == "" {
		return nil
	}
	if fi, err := os.Stat(c.root); err != nil || !fi.IsDir() {
		return nil
	}
	return []string{c.root}
}

func (c *CodexSource) Match(path string) bool { return strings.HasSuffix(path, ".jsonl") }

type codexUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CachedTokens int64 `json:"cached_input_tokens"`
	CacheReadAlt int64 `json:"cache_read_input_tokens"`
	PromptTokens int64 `json:"prompt_tokens"`
	OutTokensAlt int64 `json:"completion_tokens"`
}

type codexLine struct {
	ID        string      `json:"id"`
	Model     string      `json:"model"`
	Timestamp string      `json:"timestamp"`
	Cwd       string      `json:"cwd"`
	Session   string      `json:"session_id"`
	Usage     *codexUsage `json:"usage"`
}

func (c *CodexSource) ParseLine(line []byte) (Event, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return Event{}, false
	}
	var l codexLine
	if err := json.Unmarshal(line, &l); err != nil || l.Usage == nil {
		return Event{}, false
	}
	u := l.Usage
	usage := Usage{
		Input:     pick(u.InputTokens, u.PromptTokens),
		Output:    pick(u.OutputTokens, u.OutTokensAlt),
		CacheRead: pick(u.CachedTokens, u.CacheReadAlt),
	}
	if usage.Total() == 0 {
		return Event{}, false
	}
	id := l.ID
	if id == "" {
		return Event{}, false
	}
	ts, err := time.Parse(time.RFC3339, l.Timestamp)
	if err != nil {
		return Event{}, false
	}
	proj, projPath := projectName(l.Cwd)
	return Event{
		ID:          "codex:" + id,
		Agent:       "codex",
		Model:       l.Model,
		SessionID:   l.Session,
		Project:     proj,
		ProjectPath: projPath,
		Time:        ts,
		Usage:       usage,
	}, true
}

func pick(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}
