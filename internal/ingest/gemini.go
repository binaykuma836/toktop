package ingest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GeminiSource parses Google Gemini CLI session logs (EXPERIMENTAL).
//
// As with Codex, this is conservative: a line only counts when it carries an
// explicit token-usage object. Expected shape per JSONL line:
//
//	{"id":"...","model":"gemini-3-pro","timestamp":"...","cwd":"...",
//	 "usage":{"promptTokenCount":N,"candidatesTokenCount":N,"cachedContentTokenCount":N}}
type GeminiSource struct{ root string }

func NewGeminiSource() *GeminiSource {
	root := os.Getenv("TOKTOP_GEMINI_DIR")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".gemini")
		}
	}
	return &GeminiSource{root: root}
}

func (g *GeminiSource) Name() string { return "gemini" }

func (g *GeminiSource) Roots() []string {
	if g.root == "" {
		return nil
	}
	if fi, err := os.Stat(g.root); err != nil || !fi.IsDir() {
		return nil
	}
	return []string{g.root}
}

func (g *GeminiSource) Match(path string) bool {
	return strings.HasSuffix(path, ".jsonl") || strings.HasSuffix(path, "logs.json")
}

type geminiUsage struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	CachedContentTokens  int64 `json:"cachedContentTokenCount"`
	// snake_case fallbacks
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CachedTokens int64 `json:"cached_tokens"`
}

type geminiLine struct {
	ID        string       `json:"id"`
	Model     string       `json:"model"`
	Timestamp string       `json:"timestamp"`
	Cwd       string       `json:"cwd"`
	Session   string       `json:"session_id"`
	Usage     *geminiUsage `json:"usage"`
}

func (g *GeminiSource) ParseLine(line []byte) (Event, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return Event{}, false
	}
	var l geminiLine
	if err := json.Unmarshal(line, &l); err != nil || l.Usage == nil {
		return Event{}, false
	}
	u := l.Usage
	usage := Usage{
		Input:     pick(u.PromptTokenCount, u.InputTokens),
		Output:    pick(u.CandidatesTokenCount, u.OutputTokens),
		CacheRead: pick(u.CachedContentTokens, u.CachedTokens),
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
		ID:          "gemini:" + id,
		Agent:       "gemini",
		Model:       l.Model,
		SessionID:   l.Session,
		Project:     proj,
		ProjectPath: projPath,
		Time:        ts,
		Usage:       usage,
	}, true
}
