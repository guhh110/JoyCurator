package matching

import (
	"context"
	"errors"
	"fmt"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

type entryResult struct {
	entries []model.Entry
	err     error
	relaxed bool
}
type Resolver struct {
	log      *trace.Recorder
	client   Provider
	searches map[string]entryResult
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
	return &Resolver{log: log, client: client, searches: map[string]entryResult{}, details: map[string]entryResult{}, seasons: map[int][]model.Season{}, episodes: map[string][]model.Episode{}, failures: map[string]error{}}
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
func (m *Resolver) search(ctx context.Context, kind, title string, year int) entryResult {
	key := fmt.Sprintf("%s:%s:%d", kind, title, year)
	if result, ok := m.searches[key]; ok {
		m.log.Add("match", "search-memo", "info", "复用本次识别中的相同搜索", map[string]any{"type": kind, "query": title, "year": year, "count": len(result.entries), "relaxed": result.relaxed})
		return result
	}
	started := time.Now()
	entries, err := m.client.Search(ctx, kind, title, year)
	status, message := "info", "TMDB 搜索完成"
	if err != nil {
		status, message = "failed", err.Error()
	}
	m.log.Add("match", "search", status, message, map[string]any{"type": kind, "query": title, "year": year, "count": len(entries), "durationMs": float64(time.Since(started).Microseconds()) / 1000})
	result := entryResult{entries: entries, err: err}
	if err == nil && len(entries) == 0 && year > 0 {
		m.log.Add("match", "year-fallback", "review", "按年份未找到结果，去掉年份重试；回退结果不自动确认", map[string]any{"type": kind, "query": title, "previousYear": year})
		result = m.search(ctx, kind, title, 0)
		result.relaxed = true
	}
	m.searches[key] = result
	return result
}
func (m *Resolver) Match(ctx context.Context, item *Item) {
	p := values(item.Metadata)
	kinds := []string{"movie", "tv"}
	if p.Type == "tv" {
		kinds = []string{"tv"}
	}
	m.log.Add("match", "media-types", "info", "有剧集线索时仅查询剧集，否则同时查询电影和剧集", map[string]any{"parsedType": p.Type, "queryTypes": kinds, "tmdbId": p.TMDBID, "normalizedQuery": recognition.NormalizeTitle(p.Title)})
	facts := []Candidate{}
	exactCount := 0
	defer func() {
		m.log.Add("match", "match-decision", item.State, item.Reason, map[string]any{"candidateCount": len(facts), "exactCount": exactCount, "displayedCount": len(item.Candidates), "entry": item.Entry})
	}()
	for _, kind := range kinds {
		if p.TMDBID > 0 {
			entry, err := m.Detail(ctx, kind, p.TMDBID)
			if errors.Is(err, model.ErrNotFound) {
				m.log.Add("match", "id-not-found", "skipped", "该类型下不存在此 ID；明确 ID 查询不会回退为片名搜索", map[string]any{"type": kind, "tmdbId": p.TMDBID})
				continue
			}
			if err != nil {
				item.State, item.Reason = "failed", err.Error()
				return
			}
			facts = append(facts, Candidate{Entry: *entry})
		} else {
			if recognition.NormalizeTitle(p.Title) == "" {
				m.log.Add("match", "missing-query", "skipped", "没有可用片名或 TMDB ID，跳过搜索", map[string]any{"type": kind})
				continue
			}
			result := m.search(ctx, kind, p.Title, p.Year)
			if result.err != nil {
				item.State, item.Reason = "failed", result.err.Error()
				return
			}
			for _, entry := range result.entries {
				fact := Candidate{Entry: entry, Relaxed: result.relaxed}
				facts = append(facts, fact)
				comparison := CompareCandidate(item.Metadata, fact)
				status := "review"
				if comparison.Exact {
					status = "matched"
				}
				m.log.Add("match", "candidate-check", status, "比较标题、原名、年份及搜索是否放宽条件", map[string]any{"type": entry.Type, "tmdbId": entry.TMDBID, "title": entry.Title, "originalTitle": entry.OriginalTitle, "year": entry.Year, "normalizedTitle": recognition.NormalizeTitle(entry.Title), "normalizedOriginalTitle": recognition.NormalizeTitle(entry.OriginalTitle), "normalizedQuery": recognition.NormalizeTitle(p.Title), "sameTitle": comparison.SameTitle, "yearKnown": p.Year > 0, "sameYear": comparison.SameYear, "relaxed": fact.Relaxed, "exact": comparison.Exact})
			}
		}
	}
	decision := DecideMatch(item.Metadata, facts)
	exactCount = decision.ExactCount
	if decision.IDChecked {
		status := "review"
		if decision.IDDisambiguated {
			status = "matched"
		}
		m.log.Add("match", "id-disambiguation", status, "同一 ID 出现在多个媒体类型中，使用片名或原名与已知年份消歧", map[string]any{"tmdbId": p.TMDBID, "normalizedQuery": recognition.NormalizeTitle(p.Title), "year": p.Year, "qualifiedCount": len(decision.Qualified), "qualifiedCandidates": decision.Qualified})
	}
	item.Entry, item.Candidates, item.State, item.Reason = decision.Entry, decision.Candidates, decision.State, decision.Reason
}
