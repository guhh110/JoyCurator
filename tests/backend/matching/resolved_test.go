package matching

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/trace"
)

type resolvedSettings struct{}

func (resolvedSettings) Load() (model.Settings, error) { return model.Settings{}, nil }

func TestResolvedCheckEpisodesSharesSeasonDetail(t *testing.T) {
	seasonCalls := 0
	client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/tv/42":
			fmt.Fprint(w, `{"id":42,"name":"剧集","overview":"剧集简介"}`)
		case "/3/tv/42/season/2":
			seasonCalls++
			fmt.Fprint(w, `{"id":902,"season_number":2,"name":"第二季","overview":"季简介","air_date":"2024-05-01","poster_path":"/season.jpg","episodes":[{"episode_number":1,"still_path":"/episode.jpg"},{"episode_number":2}]}`)
		default:
			t.Errorf("unexpected TMDB request: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	service := New(resolvedSettings{}, func(model.Settings, *trace.Recorder) (Provider, error) { return client, nil })
	resolved, err := service.Prepare(context.Background(), model.Settings{}, "tv", 42)
	if err != nil {
		t.Fatal(err)
	}
	for _, numbers := range [][]int{{1, 2}, {2, 3}} {
		season, checks, err := resolved.CheckEpisodes(context.Background(), 2, numbers)
		if err != nil {
			t.Fatal(err)
		}
		if season.TMDBID != 902 || season.Number != 2 || season.Name != "第二季" || season.Overview != "季简介" || season.Date != "2024-05-01" || season.PosterPath != "/season.jpg" || season.Count != 2 {
			t.Fatalf("season %+v", season)
		}
		if len(checks) != 2 || checks[0].Number != numbers[0] || checks[1].Number != numbers[1] || checks[0].Exists != (numbers[0] != 3) || checks[1].Exists != (numbers[1] != 3) {
			t.Fatalf("checks %+v", checks)
		}
	}
	if seasonCalls != 1 {
		t.Fatalf("season fetched %d times instead of once", seasonCalls)
	}
}
