package matching

import (
	"context"
	"fmt"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/trace"
)

type entryResult struct {
	entries []model.Entry
	err     error
}
type Resolver struct {
	log      *trace.Recorder
	client   Provider
	details  map[string]entryResult
	seasons  map[int][]model.Season
	episodes map[string][]model.Episode
	failures map[string]error
}

func NewResolver(client Provider, logs ...*trace.Recorder) *Resolver {
	var log *trace.Recorder
	if len(logs) > 0 {
		log = logs[0]
	}
	return &Resolver{log: log, client: client, details: map[string]entryResult{}, seasons: map[int][]model.Season{}, episodes: map[string][]model.Episode{}, failures: map[string]error{}}
}
func (m *Resolver) Detail(ctx context.Context, kind string, id int) (*model.Entry, error) {
	started := time.Now()
	key := fmt.Sprintf("%s:%d", kind, id)
	result, ok := m.details[key]
	if !ok {
		item, err := m.client.Detail(ctx, kind, id)
		result = entryResult{err: err}
		if err == nil {
			result.entries = []model.Entry{item}
		}
		m.details[key] = result
	}
	if result.err != nil {
		m.log.Add("match", "id-lookup", "failed", result.err.Error(), map[string]any{"type": kind, "tmdbId": id, "memoHit": ok, "durationMs": float64(time.Since(started).Microseconds()) / 1000})
		return nil, result.err
	}
	m.log.Add("match", "id-lookup", "matched", "TMDB ID 查询得到作品详情", map[string]any{"type": kind, "tmdbId": id, "title": result.entries[0].Title, "memoHit": ok, "durationMs": float64(time.Since(started).Microseconds()) / 1000})
	return &result.entries[0], nil
}
