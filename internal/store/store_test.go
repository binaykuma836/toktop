package store

import (
	"testing"
	"time"

	"github.com/furkanalp41/toktop/internal/ingest"
	"github.com/furkanalp41/toktop/internal/pricing"
)

// flatPrice charges $1 per 1M output tokens, nothing else — easy to reason about.
func flatPrice(_ string, u pricing.Usage) (float64, bool) {
	return float64(u.Output) / 1_000_000.0, false
}

func ev(id, sess string, t time.Time, out int64) ingest.Event {
	return ingest.Event{ID: id, Model: "m", SessionID: sess, Time: t, Usage: ingest.Usage{Output: out}}
}

func TestDedupByID(t *testing.T) {
	s := New(flatPrice)
	now := time.Now()
	e := ev("msg1", "s1", now, 1_000_000)
	s.Add([]ingest.Event{e})
	s.Add([]ingest.Event{e}) // same ID arriving again (repeated transcript line)
	snap := s.Snapshot(now, true, 8)
	if snap.EventCount != 1 {
		t.Fatalf("event count = %d, want 1 (dedup)", snap.EventCount)
	}
	if snap.Total < 0.999 || snap.Total > 1.001 {
		t.Fatalf("total = %v, want 1.0", snap.Total)
	}
}

func TestTodayVsAllTime(t *testing.T) {
	s := New(flatPrice)
	now := time.Now()
	s.Add([]ingest.Event{ev("a", "s", now, 1_000_000)})
	s.Add([]ingest.Event{ev("b", "s", now.Add(-48*time.Hour), 1_000_000)})
	snap := s.Snapshot(now, true, 8)
	if snap.Today < 0.999 || snap.Today > 1.001 {
		t.Fatalf("today = %v, want 1.0", snap.Today)
	}
	if snap.Total < 1.999 || snap.Total > 2.001 {
		t.Fatalf("all-time = %v, want 2.0", snap.Total)
	}
}

func TestPerModelAndProjectAggregation(t *testing.T) {
	s := New(func(model string, u pricing.Usage) (float64, bool) {
		return float64(u.Output) / 1_000_000.0, false
	})
	now := time.Now()
	s.Add([]ingest.Event{
		{ID: "1", Model: "opus", SessionID: "s1", Project: "alpha", Time: now, Usage: ingest.Usage{Output: 2_000_000}},
		{ID: "2", Model: "haiku", SessionID: "s2", Project: "beta", Time: now, Usage: ingest.Usage{Output: 1_000_000}},
	})
	snap := s.Snapshot(now, true, 8)
	if len(snap.Models) != 2 || snap.Models[0].Name != "opus" {
		t.Fatalf("models = %+v", snap.Models)
	}
	if snap.Models[0].Cost < 1.99 {
		t.Fatalf("top model cost = %v, want ~2", snap.Models[0].Cost)
	}
	if len(snap.Projects) != 2 || snap.Projects[0].Name != "alpha" {
		t.Fatalf("projects = %+v", snap.Projects)
	}
	if len(snap.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(snap.Sessions))
	}
}
