package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/tmdb"
	"joycurator/internal/trace"
)

type entryResult struct {
	entries []model.Entry
	err     error
	relaxed bool
}
type recognitionMemo struct {
	log      *trace.Recorder
	client   *tmdb.Client
	searches map[string]entryResult
	details  map[string]entryResult
	seasons  map[int][]tmdb.Season
	episodes map[string][]tmdb.Episode
	failures map[string]error
}

func newRecognitionMemo(client *tmdb.Client) *recognitionMemo {
	return &recognitionMemo{client: client, searches: map[string]entryResult{}, details: map[string]entryResult{}, seasons: map[int][]tmdb.Season{}, episodes: map[string][]tmdb.Episode{}, failures: map[string]error{}}
}
func (m *recognitionMemo) detail(ctx context.Context, kind string, id int) (*model.Entry, error) {
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
func (m *recognitionMemo) search(ctx context.Context, kind, title string, year int) entryResult {
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
func (m *recognitionMemo) match(ctx context.Context, item *RecognitionItem) {
	p := item.Parsed
	kinds := []string{"movie", "tv"}
	if p.Type == "tv" {
		kinds = []string{"tv"}
	}
	m.log.Add("match", "media-types", "info", "有剧集线索时仅查询剧集，否则同时查询电影和剧集", map[string]any{"parsedType": p.Type, "queryTypes": kinds, "tmdbId": p.TMDBID, "normalizedQuery": recognition.NormalizeTitle(p.Title)})
	candidates := []model.Entry{}
	exact := []model.Entry{}
	relaxed := false
	consideredCount := 0
	defer func() {
		m.log.Add("match", "match-decision", item.State, item.Reason, map[string]any{"candidateCount": max(consideredCount, len(candidates)), "exactCount": len(exact), "displayedCount": len(item.Candidates), "entry": item.Entry})
	}()
	for _, kind := range kinds {
		if p.TMDBID > 0 {
			entry, err := m.detail(ctx, kind, p.TMDBID)
			if errors.Is(err, tmdb.ErrNotFound) {
				m.log.Add("match", "id-not-found", "skipped", "该类型下不存在此 ID；明确 ID 查询不会回退为片名搜索", map[string]any{"type": kind, "tmdbId": p.TMDBID})
				continue
			}
			if err != nil {
				item.State, item.Reason = "failed", err.Error()
				return
			}
			candidates = append(candidates, *entry)
			exact = append(exact, *entry)
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
			relaxed = relaxed || (result.relaxed && len(result.entries) > 0)
			for _, entry := range result.entries {
				candidates = append(candidates, entry)
				sameTitle := recognition.NormalizeTitle(entry.Title) == recognition.NormalizeTitle(p.Title) || recognition.NormalizeTitle(entry.OriginalTitle) == recognition.NormalizeTitle(p.Title)
				accepted := !result.relaxed && sameTitle && (p.Year == 0 || p.Year == entry.Year)
				candidateStatus := "review"
				if accepted {
					candidateStatus = "matched"
				}
				m.log.Add("match", "candidate-check", candidateStatus, "比较标题、原名、年份及搜索是否放宽条件", map[string]any{"type": entry.Type, "tmdbId": entry.TMDBID, "title": entry.Title, "originalTitle": entry.OriginalTitle, "year": entry.Year, "normalizedTitle": recognition.NormalizeTitle(entry.Title), "normalizedOriginalTitle": recognition.NormalizeTitle(entry.OriginalTitle), "normalizedQuery": recognition.NormalizeTitle(p.Title), "sameTitle": sameTitle, "yearKnown": p.Year > 0, "sameYear": p.Year == 0 || p.Year == entry.Year, "relaxed": result.relaxed, "exact": accepted})
				if accepted {
					exact = append(exact, entry)
				}
			}
		}
	}
	// TMDB IDs are scoped to a media type. When both namespaces contain the ID,
	// use the same exact title/year checks as name searches to disambiguate.
	idDisambiguated := false
	if p.TMDBID > 0 && len(exact) > 1 && recognition.NormalizeTitle(p.Title) != "" {
		qualified := []model.Entry{}
		for _, entry := range exact {
			sameTitle := recognition.NormalizeTitle(entry.Title) == recognition.NormalizeTitle(p.Title) || recognition.NormalizeTitle(entry.OriginalTitle) == recognition.NormalizeTitle(p.Title)
			if sameTitle && (p.Year == 0 || p.Year == entry.Year) {
				qualified = append(qualified, entry)
			}
		}
		status := "review"
		if len(qualified) == 1 {
			exact = qualified
			idDisambiguated, status = true, "matched"
		}
		m.log.Add("match", "id-disambiguation", status, "同一 ID 出现在多个媒体类型中，使用片名或原名与已知年份消歧", map[string]any{"tmdbId": p.TMDBID, "normalizedQuery": recognition.NormalizeTitle(p.Title), "year": p.Year, "qualifiedCount": len(qualified), "qualifiedCandidates": qualified})
	}
	// Preserve all exact candidates first, even when upstream ranks them low.
	sort.SliceStable(candidates, func(i, j int) bool {
		isExact := func(e model.Entry) bool {
			for _, v := range exact {
				if v.Type == e.Type && v.TMDBID == e.TMDBID {
					return true
				}
			}
			return false
		}
		return isExact(candidates[i]) && !isExact(candidates[j])
	})
	consideredCount = len(candidates)
	if len(candidates) > 8 {
		candidates = candidates[:8]
	}
	item.Candidates = candidates
	switch {
	case len(exact) == 1:
		entry := exact[0]
		item.Entry = &entry
		item.State = "ready"
		item.Reason = "片名精确匹配，且没有同等候选"
		if p.Year > 0 {
			item.Reason = "片名与年份精确匹配"
		}
		if p.TMDBID > 0 {
			item.Reason = "TMDB ID 唯一匹配"
			if idDisambiguated {
				item.Reason = "TMDB ID 跨类型重复，已通过片名与已知年份唯一匹配"
			}
		}
	case len(candidates) > 0:
		item.State, item.Reason = "review", "存在同名或模糊候选，请选择作品"
		if relaxed {
			item.Reason = "按年份未找到结果，已放宽年份搜索，请确认"
		}
	default:
		item.State, item.Reason = "unmatched", "未找到匹配作品，可手动搜索"
		if p.Title == "" && p.TMDBID == 0 {
			item.Reason = "无法提取片名或 TMDB ID，请手动选择作品"
		}
	}
}
