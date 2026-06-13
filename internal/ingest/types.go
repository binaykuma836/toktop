// Package ingest discovers and parses local AI coding-agent session logs into
// a uniform stream of usage Events. It reads existing files on disk read-only
// and never makes a network call.
package ingest

import "time"

// Usage holds the token counts that drive cost, split by billing class.
type Usage struct {
	Input        int64
	Output       int64
	CacheRead    int64
	CacheWrite5m int64
	CacheWrite1h int64
}

// Total is the sum of every token class.
func (u Usage) Total() int64 {
	return u.Input + u.Output + u.CacheRead + u.CacheWrite5m + u.CacheWrite1h
}

// Event is a single billable agent response, already de-duplicated upstream by
// its stable ID (the API message id).
type Event struct {
	ID          string // stable dedup key, e.g. "claude:msg_01..."
	Agent       string // "claude", "codex", "gemini"
	Model       string
	SessionID   string
	Project     string // display name (basename of the working dir)
	ProjectPath string
	Time        time.Time
	Usage       Usage
}

// Source is a pluggable agent log format. Roots are directories to scan and
// watch; Match decides which files belong to this source; ParseLine turns one
// JSONL line into an Event (ok=false to skip the line).
type Source interface {
	Name() string
	Roots() []string
	Match(path string) bool
	ParseLine(line []byte) (Event, bool)
}
