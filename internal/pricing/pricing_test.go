package pricing

import "testing"

func TestMatchAndCacheDefaults(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	r := tb.Rate("claude-opus-4-8")
	if r.Input != 5 || r.Output != 25 {
		t.Fatalf("opus base rate = %+v, want input 5 output 25", r)
	}
	if r.Estimated {
		t.Fatal("known model should not be flagged estimated")
	}
	// Derived cache defaults: read 0.1x, 5m write 1.25x, 1h write 2x of input.
	if r.CacheRead != 0.5 {
		t.Fatalf("opus cache_read = %v, want 0.5", r.CacheRead)
	}
	if r.CacheWrite5m != 6.25 {
		t.Fatalf("opus cache_write_5m = %v, want 6.25", r.CacheWrite5m)
	}
	if r.CacheWrite1h != 10 {
		t.Fatalf("opus cache_write_1h = %v, want 10", r.CacheWrite1h)
	}
	if s := tb.Rate("claude-sonnet-4-6"); s.Input != 3 {
		t.Fatalf("sonnet input = %v, want 3", s.Input)
	}
	if h := tb.Rate("claude-haiku-4-5-20251001"); h.Input != 1 {
		t.Fatalf("haiku input = %v, want 1", h.Input)
	}
	// The [1m] context variant still matches "opus".
	if v := tb.Rate("claude-opus-4-8[1m]"); v.Input != 5 {
		t.Fatalf("opus 1m variant input = %v, want 5", v.Input)
	}
}

func TestUnknownModelIsEstimated(t *testing.T) {
	tb, _ := Load()
	u := tb.Rate("some-brand-new-model-2099")
	if !u.Estimated {
		t.Fatal("unknown model should resolve to the flagged fallback")
	}
}

func TestCostMath(t *testing.T) {
	tb, _ := Load()
	cases := []struct {
		name  string
		model string
		usage Usage
		want  float64
	}{
		{"opus output 1M", "claude-opus-4-8", Usage{Output: 1_000_000}, 25},
		{"opus input 1M", "claude-opus-4-8", Usage{Input: 1_000_000}, 5},
		{"opus cache read 1M", "claude-opus-4-8", Usage{CacheRead: 1_000_000}, 0.5},
		{"sonnet mixed", "claude-sonnet-4-6", Usage{Input: 1_000_000, Output: 1_000_000}, 18},
		{"fable output 1M", "claude-fable-5", Usage{Output: 1_000_000}, 50},
		{"gemini-3-pro input 1M", "gemini-3-pro-preview", Usage{Input: 1_000_000}, 2},
	}
	for _, c := range cases {
		got, _ := tb.Cost(c.model, c.usage)
		if got < c.want-0.001 || got > c.want+0.001 {
			t.Errorf("%s: cost = %v, want %v", c.name, got, c.want)
		}
	}
}
