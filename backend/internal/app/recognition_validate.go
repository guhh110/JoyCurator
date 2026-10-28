package app

import (
	"context"
	"fmt"

	"joycurator/internal/tmdb"
)

func (m *recognitionMemo) validate(ctx context.Context, item *RecognitionItem) {
	if item.Entry == nil {
		return
	}
	defer func() {
		m.log.Add("validate", "validation-result", item.State, item.Reason, map[string]any{"season": item.Season, "episodes": item.Episodes})
	}()
	ready := func() {
		if item.State != "ready" && item.Reason != "已手动选择作品" {
			item.Reason = "作品与季集已校验"
		}
		item.State = "ready"
	}
	if item.Entry.Type == "movie" {
		m.log.Add("validate", "movie-no-episodes", "skipped", "电影不进行季集校验", nil)
		ready()
		return
	}
	id := item.Entry.TMDBID
	seasonKey := fmt.Sprintf("seasons:%d", id)
	if err := m.failures[seasonKey]; err != nil {
		item.State, item.Reason = "failed", err.Error()
		return
	}
	seasons, ok := m.seasons[id]
	if !ok {
		var err error
		seasons, err = m.client.Seasons(ctx, id)
		if err != nil {
			m.failures[seasonKey] = err
			item.State, item.Reason = "failed", err.Error()
			return
		}
		m.seasons[id] = seasons
	}
	m.log.Add("validate", "seasons", "info", "获取作品季列表，季号大于 0 的季均计为常规季", map[string]any{"tmdbId": id, "seasons": seasons, "memoHit": ok})
	if item.Season < 0 {
		regular := []tmdb.Season{}
		for _, season := range seasons {
			if season.Number > 0 {
				regular = append(regular, season)
			}
		}
		if len(regular) == 1 {
			item.Season = regular[0].Number
			item.Reason += "；唯一常规季自动补全"
			m.log.Add("validate", "infer-season", "matched", "缺少季号，使用唯一常规季", map[string]any{"regularSeasonCount": len(regular), "season": item.Season})
		} else {
			m.log.Add("validate", "infer-season", "review", "常规季不唯一，无法自动补全季号", map[string]any{"regularSeasonCount": len(regular), "regularSeasons": regular})
			item.State, item.Reason = "review", "缺少季号，该作品有多季或没有常规季，请选择"
			return
		}
	}
	if item.Parsed.Season >= 0 {
		m.log.Add("validate", "explicit-season", "info", "保留路径中的明确季号", map[string]any{"season": item.Season})
	}
	validSeason := false
	for _, season := range seasons {
		if season.Number == item.Season {
			validSeason = true
		}
	}
	if !validSeason || len(item.Episodes) == 0 || len(item.Episodes) > 1000 {
		m.log.Add("validate", "season-episode-input", "review", "季号必须存在，且需提供 1–1000 个集数", map[string]any{"validSeason": validSeason, "episodeCount": len(item.Episodes), "season": item.Season})
		item.State, item.Reason = "review", "季号或集数缺失、无效，请修正"
		return
	}
	key := fmt.Sprintf("%d:%d", id, item.Season)
	if err := m.failures[key]; err != nil {
		item.State, item.Reason = "failed", err.Error()
		return
	}
	episodes, ok := m.episodes[key]
	if !ok {
		var err error
		episodes, err = m.client.SeasonEpisodes(ctx, id, item.Season)
		if err != nil {
			m.failures[key] = err
			item.State, item.Reason = "failed", err.Error()
			return
		}
		m.episodes[key] = episodes
	}
	m.log.Add("validate", "season-episodes", "info", "获取所选季的单集列表", map[string]any{"tmdbId": id, "season": item.Season, "episodes": episodes, "memoHit": ok})
	seen := map[int]bool{}
	for _, n := range item.Episodes {
		exists := false
		for _, episode := range episodes {
			if episode.Number == n {
				exists = true
				break
			}
		}
		if n < 1 || !exists || seen[n] {
			m.log.Add("validate", "episode-check", "review", "集数无效、不存在或重复", map[string]any{"number": n, "exists": exists, "duplicate": seen[n], "positive": n > 0})
			item.State, item.Reason = "review", "集数在 TMDB 中不存在或重复，请修正"
			return
		}
		m.log.Add("validate", "episode-check", "matched", "集数存在且未重复", map[string]any{"number": n, "exists": true, "duplicate": false})
		seen[n] = true
	}
	ready()
}
