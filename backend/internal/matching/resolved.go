package matching

import (
	"context"

	"joycurator/internal/model"
)

// Resolved is the confirmed work supplied to planning. All catalog requests and
// episode comparisons remain in matching; planning only consumes their results.
type Resolved struct {
	Entry    model.Entry
	provider Provider
	seasons  map[int]model.Season
	episodes map[int][]model.Episode
}

func (a *Service) Prepare(ctx context.Context, settings model.Settings, kind string, id int) (*Resolved, error) {
	provider, err := a.factory(settings, nil)
	if err != nil {
		return nil, err
	}
	entry, err := provider.Detail(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	return &Resolved{Entry: entry, provider: provider, seasons: map[int]model.Season{}, episodes: map[int][]model.Episode{}}, nil
}

func (r *Resolved) CheckEpisodes(ctx context.Context, season int, numbers []int) (model.Season, []EpisodeCheck, error) {
	episodes, ok := r.episodes[season]
	if !ok {
		var err error
		var detail model.Season
		detail, episodes, err = r.provider.SeasonDetail(ctx, r.Entry.TMDBID, season)
		if err != nil {
			return model.Season{}, nil, err
		}
		r.seasons[season] = detail
		r.episodes[season] = episodes
	}
	return r.seasons[season], CheckEpisodes(numbers, episodes), nil
}

func (r *Resolved) SeasonEpisodes(ctx context.Context, season int) (model.Season, []model.Episode, error) {
	if _, ok := r.episodes[season]; !ok {
		if _, _, err := r.CheckEpisodes(ctx, season, nil); err != nil {
			return model.Season{}, nil, err
		}
	}
	return r.seasons[season], r.episodes[season], nil
}

func (r *Resolved) Artwork(ctx context.Context, language string) ([]model.Artwork, error) {
	return r.provider.Artwork(ctx, r.Entry.Type, r.Entry.TMDBID, language)
}

func EpisodeInputValid(season int, numbers []int) bool {
	return season >= 0 && season <= 999 && len(numbers) > 0 && len(numbers) <= 1000
}
