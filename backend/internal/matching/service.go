package matching

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"joycurator/internal/model"
)

type SettingsReader interface {
	Load() (model.Settings, error)
}
type Service struct {
	settings SettingsReader
	factory  Factory
}

func New(settings SettingsReader, factory Factory) *Service { return &Service{settings, factory} }
func (a *Service) client() (Provider, error) {
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	return a.factory(s, nil)
}
func (a *Service) Search(ctx context.Context, kind, query string, year int) ([]model.Entry, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	if kind != "movie" && kind != "tv" || strings.TrimSpace(query) == "" {
		return nil, errors.New("请输入片名并选择类型")
	}
	return client.Search(ctx, kind, query, year)
}
func (a *Service) Seasons(ctx context.Context, id int) ([]model.Season, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, errors.New("作品 ID 无效")
	}
	return client.Seasons(ctx, id)
}
func (a *Service) Episodes(ctx context.Context, id, season int) ([]model.Episode, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	if id <= 0 || season < 0 || season > 999 {
		return nil, errors.New("作品或季号无效")
	}
	return client.SeasonEpisodes(ctx, id, season)
}

func EntryGroup(entry model.Entry) string { return fmt.Sprintf("tmdb:%s:%d", entry.Type, entry.TMDBID) }
