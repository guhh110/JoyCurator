package matching

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"joycurator/internal/infra/tmdb"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
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
func TestRecognitionCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, title string
		year        int
		movie, tv   string
		state       string
		id          int
	}{
		{"unique", "Arrival", 2016, `[{"id":1,"title":"降临","original_title":"Arrival","release_date":"2016-01-01"}]`, `[]`, "ready", 1},
		{"ambiguous", "Same", 0, `[{"id":1,"title":"Same"},{"id":2,"title":"Same"}]`, `[]`, "review", 0},
		{"cross type", "Same", 0, `[{"id":1,"title":"Same"}]`, `[{"id":2,"name":"Same"}]`, "review", 0},
		{"fuzzy", "Film", 0, `[{"id":1,"title":"Film Two"}]`, `[]`, "review", 0},
		{"not found", "Missing", 0, `[]`, `[]`, "unmatched", 0},
		{"wrong year", "Same", 2020, `[{"id":1,"title":"Same","release_date":"2019-01-01"}]`, `[]`, "review", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
				items := tc.movie
				if strings.HasSuffix(r.URL.Path, "/search/tv") {
					items = tc.tv
				}
				fmt.Fprintf(w, `{"results":%s}`, items)
			})
			item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{Title: tc.title, Year: tc.year})}
			NewResolver(client).Match(context.Background(), &item)
			if item.State != tc.state {
				t.Fatalf("%+v", item)
			}
			if tc.id > 0 && (item.Entry == nil || item.Entry.TMDBID != tc.id) {
				t.Fatal(item)
			}
		})
	}
}
func TestRecognitionYearFallbackAndQueryDeduplication(t *testing.T) {
	var calls atomic.Int32
	client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("first_air_date_year") != "" {
			fmt.Fprint(w, `{"results":[]}`)
		} else {
			fmt.Fprint(w, `{"results":[{"id":2,"name":"Show","first_air_date":"2021-01-01"}]}`)
		}
	})
	memo := NewResolver(client)
	for i := 0; i < 3; i++ {
		item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{Title: "Show", Type: "tv", Year: 2020})}
		memo.Match(context.Background(), &item)
		if item.State != "review" || !strings.Contains(item.Reason, "年份") {
			t.Fatal(item)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("queries not deduplicated", calls.Load())
	}
}
func TestRecognitionIDAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		state  string
	}{
		{"id", 200, `{"id":7,"name":"Show","overview":"Description"}`, "ready"},
		{"missing", 404, `{}`, "unmatched"},
		{"credentials", 401, `{"error":"private"}`, "failed"},
		{"network", 503, `{}`, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) })
			item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{Type: "tv", TMDBID: 7})}
			NewResolver(client).Match(context.Background(), &item)
			if item.State != tc.state {
				t.Fatal(item)
			}
			if strings.Contains(item.Reason, "private") {
				t.Fatal("upstream error leaked")
			}
		})
	}
	client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("cancelled request reached server") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{Type: "tv", Title: "Show"})}
	NewResolver(client).Match(ctx, &item)
	if item.State != "failed" {
		t.Fatal(item)
	}
}
func TestRecognitionSeasonValidation(t *testing.T) {
	for _, tc := range []struct {
		name, seasons string
		season        int
		episodes      []int
		state         string
		wantSeason    int
	}{
		{"unique season", `[{"season_number":0},{"season_number":2}]`, -1, []int{1}, "ready", 2},
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
func TestRecognitionIDCollisionNeedsConfirmation(t *testing.T) {
	client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":7,"title":"Film","name":"Show","overview":"Description"}`)
	})
	item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{TMDBID: 7})}
	NewResolver(client).Match(context.Background(), &item)
	if item.State != "review" || item.Entry != nil || len(item.Candidates) != 2 {
		t.Fatal(item)
	}
}

func TestIDDisambiguationRequiresUniqueExactTitleAndYear(t *testing.T) {
	for _, tc := range []struct {
		name                string
		movieTitle, tvTitle string
		movieYear, tvYear   int
		want                string
	}{
		{"both exact", "Same", "Same", 2020, 2020, "review"},
		{"neither title matches", "Other", "Different", 2020, 2020, "review"},
		{"title matches but year conflicts", "Same", "Different", 2019, 2020, "review"},
		{"unique TV candidate", "Different", "Same", 2020, 2020, "ready"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
				title, year := tc.movieTitle, tc.movieYear
				if strings.Contains(r.URL.Path, "/tv/") {
					title, year = tc.tvTitle, tc.tvYear
				}
				fmt.Fprintf(w, `{"id":7,"title":%q,"original_title":%q,"name":%q,"original_name":%q,"release_date":"%d-01-01","first_air_date":"%d-01-01","overview":"fixture"}`, title, title, title, title, year, year)
			})
			item := Item{Metadata: recognition.MetadataFromParsed(model.Parsed{TMDBID: 7, Title: "Same", Year: 2020})}
			NewResolver(client).Match(context.Background(), &item)
			if item.State != tc.want {
				t.Fatal(item)
			}
			if tc.want == "ready" && (item.Entry == nil || item.Entry.Type != "tv") {
				t.Fatal(item)
			}
		})
	}
}
