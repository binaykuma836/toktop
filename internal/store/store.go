// Package store aggregates de-duplicated usage Events into the live views the
// TUI renders: per-session, per-model and per-project totals, time-bucketed
// spend for sparklines, and today/week/all-time roll-ups. It is safe for
// concurrent use (the watcher writes, the UI reads).
package store

import (
	"sort"
	"sync"
	"time"

	"github.com/furkanalp41/toktop/internal/ingest"
	"github.com/furkanalp41/toktop/internal/pricing"
)

// bucketSeconds is the width of a spend bucket (one minute), used for the
// today/week roll-ups.
const bucketSeconds int64 = 60

// recentCap bounds the per-session and global rings of recent per-response
// costs that drive the live sparklines.
const recentCap = 64

// PriceFn computes (cost, estimated) for a model + usage.
type PriceFn func(model string, u pricing.Usage) (float64, bool)

type modelAgg struct {
	model string
	cost  float64
	usage ingest.Usage
}

type session struct {
	id          string
	agent       string
	project     string
	projectPath string
	first       time.Time
	last        time.Time
	cost        float64
	usage       ingest.Usage
	estimated   bool
	models      map[string]*modelAgg
	recent      []float64 // ring of recent per-response costs (live sparkline)
}

// Store is the concurrent aggregation root.
type Store struct {
	mu         sync.Mutex
	price      PriceFn
	seen       map[string]struct{}
	sessions   map[string]*session
	order      []string
	buckets    map[int64]float64
	recent     []float64
	total      float64
	totalUsage ingest.Usage
	hasEst     bool
	updated    time.Time
	events     int64
}

// New creates an empty store using price for cost computation.
func New(price PriceFn) *Store {
	return &Store{
		price:    price,
		seen:     make(map[string]struct{}),
		sessions: make(map[string]*session),
		buckets:  make(map[int64]float64),
	}
}

func toPricingUsage(u ingest.Usage) pricing.Usage {
	return pricing.Usage{
		Input:        u.Input,
		Output:       u.Output,
		CacheRead:    u.CacheRead,
		CacheWrite5m: u.CacheWrite5m,
		CacheWrite1h: u.CacheWrite1h,
	}
}

func addUsage(dst *ingest.Usage, s ingest.Usage) {
	dst.Input += s.Input
	dst.Output += s.Output
	dst.CacheRead += s.CacheRead
	dst.CacheWrite5m += s.CacheWrite5m
	dst.CacheWrite1h += s.CacheWrite1h
}

// Add ingests a batch of events, ignoring any whose ID was already seen.
// It returns how many were newly counted.
func (s *Store) Add(events []ingest.Event) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Only events from roughly the last week+ affect the today/week roll-ups,
	// so we don't store minute-buckets for older history (the bulk of a startup
	// scan) — they still count toward all-time via s.total.
	cutoff := time.Now().Add(-8 * 24 * time.Hour)

	added := 0
	for _, e := range events {
		if e.ID == "" {
			continue
		}
		if _, dup := s.seen[e.ID]; dup {
			continue
		}
		s.seen[e.ID] = struct{}{}
		added++

		cost, est := s.price(e.Model, toPricingUsage(e.Usage))

		sk := e.SessionID
		if sk == "" {
			sk = "(" + e.Agent + ")"
		}
		sess := s.sessions[sk]
		if sess == nil {
			sess = &session{
				id:          sk,
				agent:       e.Agent,
				project:     e.Project,
				projectPath: e.ProjectPath,
				first:       e.Time,
				last:        e.Time,
				models:      make(map[string]*modelAgg),
			}
			s.sessions[sk] = sess
			s.order = append(s.order, sk)
		}
		if sess.first.IsZero() || e.Time.Before(sess.first) {
			sess.first = e.Time
		}
		if e.Time.After(sess.last) {
			sess.last = e.Time
		}
		if sess.project == "" && e.Project != "" {
			sess.project = e.Project
			sess.projectPath = e.ProjectPath
		}
		sess.cost += cost
		addUsage(&sess.usage, e.Usage)
		if est {
			sess.estimated = true
			s.hasEst = true
		}

		ma := sess.models[e.Model]
		if ma == nil {
			ma = &modelAgg{model: e.Model}
			sess.models[e.Model] = ma
		}
		ma.cost += cost
		addUsage(&ma.usage, e.Usage)

		if e.Time.After(cutoff) {
			s.buckets[bucketOf(e.Time)] += cost
		}
		s.total += cost
		addUsage(&s.totalUsage, e.Usage)

		sess.recent = appendCapped(sess.recent, cost)
		s.recent = appendCapped(s.recent, cost)
	}
	if added > 0 {
		s.events += int64(added)
		s.updated = time.Now()
	}
	return added
}

func bucketOf(t time.Time) int64 { return t.Unix() / bucketSeconds }

// ---- Snapshot views ----

// SessionView is a read-only projection of one session for rendering.
type SessionView struct {
	ID        string
	ShortID   string
	Agent     string
	Project   string
	Cost      float64
	Usage     ingest.Usage
	Last      time.Time
	Estimated bool
	TopModel  string
	Spark     []float64
}

// AggView is a named cost/usage aggregate (per model or per project).
type AggView struct {
	Name  string
	Cost  float64
	Usage ingest.Usage
	Share float64 // fraction of total cost
}

// Snapshot is an immutable consistent view for one render frame.
type Snapshot struct {
	Sessions     []SessionView
	Models       []AggView
	Projects     []AggView
	Total        float64
	Today        float64
	Week         float64
	TotalUsage   ingest.Usage
	HasEstimated bool
	Spark        []float64
	Updated      time.Time
	SessionCount int
	EventCount   int64
}

// Snapshot builds a consistent view as of now. sortByCost orders sessions by
// spend (otherwise by most-recent activity); sparkN is the number of recent
// one-minute buckets to include in each sparkline.
func (s *Store) Snapshot(now time.Time, sortByCost bool, sparkN int) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	// WEEK is the last 7 local days, so it equals the sum of seven daily figures
	// rather than a rolling 168h instant that fuzzes at the minute boundary.
	weekStart := midnight - 6*86400
	var today, week float64
	for idx, c := range s.buckets {
		bt := idx * bucketSeconds
		if bt >= midnight {
			today += c
		}
		if bt >= weekStart {
			week += c
		}
	}

	views := make([]SessionView, 0, len(s.sessions))
	modelTotals := make(map[string]*AggView)
	projTotals := make(map[string]*AggView)

	for _, id := range s.order {
		sess := s.sessions[id]
		var top string
		var topCost float64
		for m, ma := range sess.models {
			if ma.cost > topCost || top == "" {
				topCost = ma.cost
				top = m
			}
			mt := modelTotals[m]
			if mt == nil {
				mt = &AggView{Name: m}
				modelTotals[m] = mt
			}
			mt.Cost += ma.cost
			addUsage(&mt.Usage, ma.usage)
		}

		pkey := sess.project
		if pkey == "" {
			pkey = "—"
		}
		pt := projTotals[pkey]
		if pt == nil {
			pt = &AggView{Name: pkey}
			projTotals[pkey] = pt
		}
		pt.Cost += sess.cost
		addUsage(&pt.Usage, sess.usage)

		views = append(views, SessionView{
			ID:        sess.id,
			ShortID:   shortID(sess.id),
			Agent:     sess.agent,
			Project:   sess.project,
			Cost:      sess.cost,
			Usage:     sess.usage,
			Last:      sess.last,
			Estimated: sess.estimated,
			TopModel:  top,
			Spark:     lastN(sess.recent, sparkN),
		})
	}

	sort.SliceStable(views, func(i, j int) bool {
		if sortByCost {
			if views[i].Cost != views[j].Cost {
				return views[i].Cost > views[j].Cost
			}
			return views[i].Last.After(views[j].Last)
		}
		if !views[i].Last.Equal(views[j].Last) {
			return views[i].Last.After(views[j].Last)
		}
		return views[i].Cost > views[j].Cost
	})

	return Snapshot{
		Sessions:     views,
		Models:       aggSlice(modelTotals, s.total),
		Projects:     aggSlice(projTotals, s.total),
		Total:        s.total,
		Today:        today,
		Week:         week,
		TotalUsage:   s.totalUsage,
		HasEstimated: s.hasEst,
		Spark:        lastN(s.recent, sparkN),
		Updated:      s.updated,
		SessionCount: len(s.sessions),
		EventCount:   s.events,
	}
}

func aggSlice(m map[string]*AggView, total float64) []AggView {
	out := make([]AggView, 0, len(m))
	for _, v := range m {
		av := *v
		if total > 0 {
			av.Share = av.Cost / total
		}
		out = append(out, av)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	return out
}

// appendCapped appends v to a ring buffer, dropping the oldest past recentCap.
func appendCapped(s []float64, v float64) []float64 {
	s = append(s, v)
	if len(s) > recentCap {
		s = s[len(s)-recentCap:]
	}
	return s
}

// lastN returns the last n values of s, left-padded with zeros (oldest first).
func lastN(s []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		if si := len(s) - n + i; si >= 0 {
			out[i] = s[si]
		}
	}
	return out
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
