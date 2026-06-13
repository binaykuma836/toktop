package ingest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeSource parses Claude Code session transcripts under ~/.claude/projects.
// This is the first-class, verified integration.
//
// Important: a single API response is written to the transcript on MULTIPLE
// lines (one per streamed content block), each repeating the same
// `message.usage`. We therefore key Events on the API message id so the store
// counts each response exactly once. Summing raw lines would overcount ~3-4x.
type ClaudeSource struct{ root string }

// NewClaudeSource locates the Claude Code projects directory, honouring
// $TOKTOP_CLAUDE_DIR (which should point at the .claude dir).
func NewClaudeSource() *ClaudeSource {
	base := os.Getenv("TOKTOP_CLAUDE_DIR")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".claude")
		}
	}
	return &ClaudeSource{root: filepath.Join(base, "projects")}
}

func (c *ClaudeSource) Name() string { return "claude" }

func (c *ClaudeSource) Roots() []string {
	if c.root == "" {
		return nil
	}
	if fi, err := os.Stat(c.root); err != nil || !fi.IsDir() {
		return nil
	}
	return []string{c.root}
}

func (c *ClaudeSource) Match(path string) bool { return strings.HasSuffix(path, ".jsonl") }

type claudeUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheCreation            *struct {
		Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	UUID      string `json:"uuid"`
	Message   struct {
		ID    string       `json:"id"`
		Model string       `json:"model"`
		Usage *claudeUsage `json:"usage"`
	} `json:"message"`
}

func (c *ClaudeSource) ParseLine(line []byte) (Event, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return Event{}, false
	}
	var l claudeLine
	if err := json.Unmarshal(line, &l); err != nil {
		return Event{}, false
	}
	if l.Type != "assistant" || l.Message.Usage == nil {
		return Event{}, false
	}
	u := l.Message.Usage
	usage := Usage{
		Input:     u.InputTokens,
		Output:    u.OutputTokens,
		CacheRead: u.CacheReadInputTokens,
	}
	// Prefer the explicit 5m/1h split when present; otherwise treat all cache
	// creation as a 5-minute write (the common default).
	if u.CacheCreation != nil && (u.CacheCreation.Ephemeral5m != 0 || u.CacheCreation.Ephemeral1h != 0) {
		usage.CacheWrite5m = u.CacheCreation.Ephemeral5m
		usage.CacheWrite1h = u.CacheCreation.Ephemeral1h
		// If the broken-out classes sum to less than the reported total (an
		// unforeseen creation class), attribute the remainder to a 5m write so
		// no cost is silently dropped.
		if rem := u.CacheCreationInputTokens - usage.CacheWrite5m - usage.CacheWrite1h; rem > 0 {
			usage.CacheWrite5m += rem
		}
	} else {
		usage.CacheWrite5m = u.CacheCreationInputTokens
	}
	if usage.Total() == 0 {
		return Event{}, false
	}

	// Dedup key: API message id is shared across the repeated lines of one
	// response. Fall back to requestId, then the per-line uuid.
	id := l.Message.ID
	if id == "" {
		id = l.RequestID
	}
	if id == "" {
		id = l.UUID
	}
	if id == "" {
		return Event{}, false
	}

	// Require a valid timestamp: dating a backfilled or malformed line to "now"
	// would inflate the today/budget figure the whole tool is built around.
	ts, err := time.Parse(time.RFC3339, l.Timestamp)
	if err != nil {
		return Event{}, false
	}

	proj, projPath := projectName(l.Cwd)
	return Event{
		ID:          "claude:" + id,
		Agent:       "claude",
		Model:       l.Message.Model,
		SessionID:   l.SessionID,
		Project:     proj,
		ProjectPath: projPath,
		Time:        ts,
		Usage:       usage,
	}, true
}

// projectName derives a short display name from a working-directory path.
func projectName(cwd string) (string, string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", ""
	}
	return filepath.Base(cwd), cwd
}
