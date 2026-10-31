package matching

import (
	"context"

	"joycurator/internal/model"
	"joycurator/internal/trace"
)

type Provider interface {
	Search(context.Context, string, string, int) ([]model.Entry, error)
	Detail(context.Context, string, int) (model.Entry, error)
	Seasons(context.Context, int) ([]model.Season, error)
	SeasonDetail(context.Context, int, int) (model.Season, []model.Episode, error)
	SeasonEpisodes(context.Context, int, int) ([]model.Episode, error)
	Image(context.Context, string) ([]byte, error)
}

// Factory returns a provider configured for this settings snapshot and trace recorder.
type Factory func(model.Settings, *trace.Recorder) (Provider, error)
