package matching

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"joycurator/internal/infra/tmdb"
	"joycurator/internal/model"
)

type recognitionTransport func(*http.Request) (*http.Response, error)

func (f recognitionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockRecognitionClient(t *testing.T, handler http.HandlerFunc) *tmdb.Client {
	t.Helper()

	client, _ := tmdb.New(nil, "test-token", "", "zh-CN")
	client.HTTP = &http.Client{Transport: recognitionTransport(func(request *http.Request) (*http.Response, error) {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		return recorder.Result(), nil
	})}
	client.RetryWait = time.Millisecond
	return client
}
func TestRecognitionSeasonValidation(t *testing.T) {
	for _, tc := range []struct {
		name, seasons string
		season        int
		episodes      []int
		state         string
		wantSeason    int
	}{
		{"unique season stays unconfirmed", `[{"season_number":0},{"season_number":2}]`, -1, []int{1}, "review", -1},
		{"multiple seasons", `[{"season_number":1},{"season_number":2}]`, -1, []int{1}, "review", -1},
		{"specials", `[{"season_number":0},{"season_number":1}]`, 0, []int{1}, "ready", 0},
		{"invalid episode", `[{"season_number":1}]`, 1, []int{3}, "review", 1},
		{"merged episodes", `[{"season_number":1}]`, 1, []int{1, 2}, "ready", 1},
		{"duplicate", `[{"season_number":1}]`, 1, []int{1, 1}, "review", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/season/") {
					fmt.Fprint(w, `{"episodes":[{"episode_number":1},{"episode_number":2}]}`)
				} else {
					fmt.Fprintf(w, `{"seasons":%s}`, tc.seasons)
				}
			})
			item := Item{File: model.File{Season: tc.season, Episodes: tc.episodes}, Entry: &model.Entry{Type: "tv", TMDBID: 2}}
			NewResolver(client).Validate(context.Background(), &item)
			if item.State != tc.state || item.Season != tc.wantSeason {
				t.Fatal(item)
			}
		})
	}
}
