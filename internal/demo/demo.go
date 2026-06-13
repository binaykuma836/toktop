// Package demo streams a synthetic but realistic event flow so `toktop --demo`
// lights up instantly — for the README GIF and for anyone without active
// sessions. It feeds the same store and rendering path as live data.
package demo

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/furkanalp41/toktop/internal/ingest"
)

type demoSession struct {
	id      string
	project string
	models  []string
}

// Run emits batches of fake events until ctx is cancelled.
func Run(ctx context.Context, out chan<- []ingest.Event) {
	defer close(out)
	rng := rand.New(rand.NewSource(1))
	sessions := []demoSession{
		{id: "demo-api-gateway-7f3a21", project: "api-gateway", models: []string{"claude-opus-4-8"}},
		{id: "demo-web-frontend-2b9c44", project: "web-frontend", models: []string{"claude-sonnet-4-6"}},
		{id: "demo-data-pipeline-c14e90", project: "data-pipeline", models: []string{"claude-haiku-4-5", "claude-sonnet-4-6"}},
	}

	ticker := time.NewTicker(280 * time.Millisecond)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n := 1 + rng.Intn(2)
			batch := make([]ingest.Event, 0, n)
			for i := 0; i < n; i++ {
				s := sessions[rng.Intn(len(sessions))]
				model := s.models[rng.Intn(len(s.models))]
				counter++
				batch = append(batch, ingest.Event{
					ID:          fmt.Sprintf("demo:%d", counter),
					Agent:       "claude",
					Model:       model,
					SessionID:   s.id,
					Project:     s.project,
					ProjectPath: "/demo/" + s.project,
					Time:        time.Now(),
					Usage: ingest.Usage{
						Input:        int64(20 + rng.Intn(400)),
						Output:       int64(120 + rng.Intn(1400)),
						CacheRead:    int64(rng.Intn(40000)),
						CacheWrite5m: int64(rng.Intn(6000)),
					},
				})
			}
			select {
			case out <- batch:
			case <-ctx.Done():
				return
			}
		}
	}
}
