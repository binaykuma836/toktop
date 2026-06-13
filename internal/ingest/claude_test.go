package ingest

import "testing"

func TestParseClaudeLine(t *testing.T) {
	c := NewClaudeSource()
	line := []byte(`{"type":"assistant","timestamp":"2026-06-09T16:20:02.654Z","cwd":"/home/vlad/proj","sessionId":"sess1","requestId":"req1","uuid":"u1","message":{"id":"msg1","model":"claude-opus-4-8","usage":{"input_tokens":100,"output_tokens":200,"cache_read_input_tokens":50,"cache_creation_input_tokens":40,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":30}}}}`)
	e, ok := c.ParseLine(line)
	if !ok {
		t.Fatal("expected the line to parse")
	}
	if e.ID != "claude:msg1" {
		t.Fatalf("dedup id = %q, want claude:msg1", e.ID)
	}
	if e.Usage.Input != 100 || e.Usage.Output != 200 || e.Usage.CacheRead != 50 {
		t.Fatalf("usage = %+v", e.Usage)
	}
	if e.Usage.CacheWrite5m != 10 || e.Usage.CacheWrite1h != 30 {
		t.Fatalf("cache split = %+v", e.Usage)
	}
	if e.Project != "proj" {
		t.Fatalf("project = %q, want proj", e.Project)
	}
	if e.Model != "claude-opus-4-8" {
		t.Fatalf("model = %q", e.Model)
	}
}

func TestParseClaudeSkips(t *testing.T) {
	c := NewClaudeSource()
	if _, ok := c.ParseLine([]byte(`{"type":"user"}`)); ok {
		t.Error("user line should be skipped")
	}
	if _, ok := c.ParseLine([]byte(`not json at all`)); ok {
		t.Error("non-json should be skipped")
	}
	if _, ok := c.ParseLine([]byte(`{"type":"assistant","message":{"id":"m","usage":{"input_tokens":0,"output_tokens":0}}}`)); ok {
		t.Error("zero-usage assistant line should be skipped")
	}
}

func TestParseCacheFallbackTo5m(t *testing.T) {
	c := NewClaudeSource()
	// No cache_creation breakdown: the whole cache_creation_input_tokens count
	// should be attributed to a 5-minute write.
	line := []byte(`{"type":"assistant","timestamp":"2026-06-09T16:20:02Z","message":{"id":"m2","model":"x","usage":{"input_tokens":1,"cache_creation_input_tokens":500}}}`)
	e, ok := c.ParseLine(line)
	if !ok {
		t.Fatal("expected parse")
	}
	if e.Usage.CacheWrite5m != 500 || e.Usage.CacheWrite1h != 0 {
		t.Fatalf("fallback split = %+v, want 5m=500 1h=0", e.Usage)
	}
}

func TestIDFallbackOrder(t *testing.T) {
	c := NewClaudeSource()
	// Missing message.id -> requestId is used.
	line := []byte(`{"type":"assistant","timestamp":"2026-06-09T16:20:02Z","requestId":"req9","uuid":"u9","message":{"model":"x","usage":{"output_tokens":5}}}`)
	e, ok := c.ParseLine(line)
	if !ok {
		t.Fatal("expected parse")
	}
	if e.ID != "claude:req9" {
		t.Fatalf("id = %q, want claude:req9", e.ID)
	}
}

func TestParseRejectsMissingTimestamp(t *testing.T) {
	c := NewClaudeSource()
	// A valid-looking usage line with no timestamp must be dropped rather than
	// dated to now() (which would inflate the "today" figure).
	line := []byte(`{"type":"assistant","message":{"id":"nots","model":"x","usage":{"output_tokens":5}}}`)
	if _, ok := c.ParseLine(line); ok {
		t.Error("line without a parseable timestamp should be dropped")
	}
}
