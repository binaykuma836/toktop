// Package pricing turns token counts into dollars using an editable,
// cache-aware pricing table. The default table is embedded; users can override
// any model's price with ~/.config/toktop/pricing.toml (or $TOKTOP_PRICING)
// without waiting for a new release, because prices drift.
package pricing

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed prices.toml
var defaultTOML string

// Usage is the set of token classes that affect cost.
type Usage struct {
	Input        int64
	Output       int64
	CacheRead    int64
	CacheWrite5m int64
	CacheWrite1h int64
}

// Rate is the resolved per-class price (USD per 1M tokens) for a model.
type Rate struct {
	Input        float64
	Output       float64
	CacheRead    float64
	CacheWrite5m float64
	CacheWrite1h float64
	Label        string
	Estimated    bool // true when resolved from the fallback (unknown model)
}

// Cost returns the dollar cost of the given usage at this rate.
func (r Rate) Cost(u Usage) float64 {
	const perMillion = 1_000_000.0
	return (float64(u.Input)*r.Input +
		float64(u.Output)*r.Output +
		float64(u.CacheRead)*r.CacheRead +
		float64(u.CacheWrite5m)*r.CacheWrite5m +
		float64(u.CacheWrite1h)*r.CacheWrite1h) / perMillion
}

type entry struct {
	Match        []string `toml:"match"`
	Input        float64  `toml:"input"`
	Output       float64  `toml:"output"`
	CacheRead    *float64 `toml:"cache_read"`
	CacheWrite5m *float64 `toml:"cache_write_5m"`
	CacheWrite1h *float64 `toml:"cache_write_1h"`
}

type fileSchema struct {
	Model   []entry `toml:"model"`
	Default *entry  `toml:"default"`
}

type resolved struct {
	match []string
	rate  Rate
}

// Table is a resolved pricing table. User overrides take precedence over the
// embedded defaults, and unknown models resolve to a flagged fallback rate.
type Table struct {
	user     []resolved
	base     []resolved
	fallback Rate
}

func deriveRate(e entry) Rate {
	r := Rate{Input: e.Input, Output: e.Output}
	if e.CacheRead != nil {
		r.CacheRead = *e.CacheRead
	} else {
		r.CacheRead = e.Input * 0.10
	}
	if e.CacheWrite5m != nil {
		r.CacheWrite5m = *e.CacheWrite5m
	} else {
		r.CacheWrite5m = e.Input * 1.25
	}
	if e.CacheWrite1h != nil {
		r.CacheWrite1h = *e.CacheWrite1h
	} else {
		r.CacheWrite1h = e.Input * 2.0
	}
	if len(e.Match) > 0 {
		r.Label = e.Match[0]
	}
	return r
}

func resolveAll(es []entry) []resolved {
	out := make([]resolved, 0, len(es))
	for _, e := range es {
		lm := make([]string, 0, len(e.Match))
		for _, m := range e.Match {
			lm = append(lm, strings.ToLower(strings.TrimSpace(m)))
		}
		out = append(out, resolved{match: lm, rate: deriveRate(e)})
	}
	return out
}

func parseTable(data string) (fileSchema, error) {
	var f fileSchema
	_, err := toml.Decode(data, &f)
	return f, err
}

// Load builds the table from the embedded defaults plus any user override.
func Load() (*Table, error) {
	base, err := parseTable(defaultTOML)
	if err != nil {
		return nil, err
	}
	t := &Table{base: resolveAll(base.Model)}
	t.fallback = Rate{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite5m: 3.75, CacheWrite1h: 6, Label: "default", Estimated: true}
	if base.Default != nil {
		t.fallback = deriveRate(*base.Default)
		t.fallback.Estimated = true
		if t.fallback.Label == "" {
			t.fallback.Label = "default"
		}
	}
	if p := overridePath(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			if uf, err := parseTable(string(data)); err == nil {
				t.user = resolveAll(uf.Model)
				if uf.Default != nil {
					t.fallback = deriveRate(*uf.Default)
					t.fallback.Estimated = true
				}
			}
		}
	}
	return t, nil
}

func matchIn(rs []resolved, model string) (Rate, bool) {
	best := -1
	var br Rate
	for _, r := range rs {
		for _, sub := range r.match {
			if sub != "" && strings.Contains(model, sub) && len(sub) > best {
				best = len(sub)
				br = r.rate
			}
		}
	}
	if best < 0 {
		return Rate{}, false
	}
	return br, true
}

// Rate resolves the price for a model id (user table first, then defaults,
// then the flagged fallback for unknown models).
func (t *Table) Rate(model string) Rate {
	m := strings.ToLower(model)
	if r, ok := matchIn(t.user, m); ok {
		return r
	}
	if r, ok := matchIn(t.base, m); ok {
		return r
	}
	r := t.fallback
	r.Estimated = true
	return r
}

// Cost returns the dollar cost of usage for a model and whether the rate was
// an estimate (unknown model).
func (t *Table) Cost(model string, u Usage) (float64, bool) {
	r := t.Rate(model)
	return r.Cost(u), r.Estimated
}

func overridePath() string {
	if p := os.Getenv("TOKTOP_PRICING"); p != "" {
		return p
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "toktop", "pricing.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "toktop", "pricing.toml")
}
