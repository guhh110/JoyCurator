package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"joycurator/internal/model"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestSearchCacheAndToken(t *testing.T) {
	cache := &testCache{}
	client, err := New(cache, "private-token", "", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Fatal("missing token header")
		}
		if r.URL.Query().Get("language") != "zh-CN" || r.URL.Query().Get("year") != "2020" {
			t.Fatal(r.URL.String())
		}
		return response(200, `{"results":[{"id":1,"title":"中文标题","release_date":"2020-01-01"}]}`), nil
	})}
	for i := 0; i < 2; i++ {
		items, err := client.Search(context.Background(), "movie", "标题", 2020)
		if err != nil || len(items) != 1 || items[0].Title != "中文标题" {
			t.Fatalf("%#v %v", items, err)
		}
	}
	if calls != 1 {
		t.Fatal("cache not used", calls)
	}
}
func TestRateLimitAndEmptyResult(t *testing.T) {
	client, _ := New(nil, "token", "", "en-US")
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(429, `{}`), nil
		}
		return response(200, `{"results":[]}`), nil
	})}
	items, err := client.Search(context.Background(), "tv", "Unknown", 0)
	if err != nil || len(items) != 0 || calls != 2 {
		t.Fatalf("%v %v %d", items, err, calls)
	}
}
func TestErrorRedactionAndCancellation(t *testing.T) {
	client, _ := New(nil, "very-secret-token", "", "en-US")
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return response(401, `{"error":"very-secret-token"}`), nil
	})}
	_, err := client.Search(context.Background(), "movie", "title", 0)
	if err == nil || strings.Contains(err.Error(), "very-secret-token") {
		t.Fatal("credentials leaked", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Search(ctx, "movie", "title", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "dial tcp: i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestTimeoutRetriesThenFails covers a slow upstream: three attempts, then a
// stable error that never exposes the transport details.
func TestTimeoutRetriesThenFails(t *testing.T) {
	client, _ := New(nil, "token", "", "en-US")
	client.RetryWait = time.Millisecond
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, timeoutError{}
	})}
	_, err := client.Search(context.Background(), "movie", "title", 0)
	if err == nil || !strings.Contains(err.Error(), "网络连接失败") {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("attempts %d", calls)
	}
}

type testCache struct{ data []byte }

func (c *testCache) LoadCache(context.Context, string, int64) ([]byte, error) { return c.data, nil }
func (c *testCache) SaveCache(_ context.Context, _ string, b []byte, _ int64) error {
	c.data = append([]byte(nil), b...)
	return nil
}

// TestSeasonDetailAndEpisodesShareResponse checks that both API methods decode
// the same season response, including the season's own TMDB ID and artwork.
func TestSeasonDetailAndEpisodesShareResponse(t *testing.T) {
	cache := &testCache{}
	client, _ := New(cache, "token", "", "zh-CN")
	calls := 0
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/3/tv/42/season/2" || r.URL.Query().Get("language") != "zh-CN" {
			t.Fatalf("unexpected TMDB request: %s", r.URL)
		}
		return response(200, `{"id":902,"season_number":2,"name":"第二季","overview":"季简介","air_date":"2024-05-01","poster_path":"/season.jpg","episodes":[{"episode_number":1,"name":"第一集","air_date":"2024-05-02","overview":"首集简介","still_path":"/episode.jpg"},{"episode_number":2,"name":"第二集","air_date":"2024-05-09","overview":"次集简介","still_path":null}]}`), nil
	})}
	season, episodes, err := client.SeasonDetail(context.Background(), 42, 2)
	if err != nil {
		t.Fatal(err)
	}
	if season.TMDBID != 902 || season.Number != 2 || season.Name != "第二季" || season.Overview != "季简介" || season.Date != "2024-05-01" || season.PosterPath != "/season.jpg" || season.Count != 2 {
		t.Fatalf("season metadata: %+v", season)
	}
	if len(episodes) != 2 || episodes[0].Season != 2 || episodes[0].Number != 1 || episodes[0].Name != "第一集" || episodes[0].Overview != "首集简介" || episodes[0].AirDate != "2024-05-02" || episodes[0].Still != "/episode.jpg" || episodes[1].Number != 2 || episodes[1].Still != "" {
		t.Fatalf("episode metadata: %+v", episodes)
	}
	fromDelegate, err := client.SeasonEpisodes(context.Background(), 42, 2)
	if err != nil || len(fromDelegate) != 2 || fromDelegate[0] != episodes[0] || fromDelegate[1] != episodes[1] || calls != 1 {
		t.Fatalf("delegate episodes %+v, calls %d, error %v", fromDelegate, calls, err)
	}
}

func TestSeasonDetailSeasonNumber(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"missing season number", `{"id":902,"episodes":[{"episode_number":1}]}`, false},
		{"special season zero", `{"id":900,"season_number":0,"episodes":[{"episode_number":1}]}`, true},
		{"mismatched season number", `{"id":903,"season_number":3,"episodes":[{"episode_number":1}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := New(nil, "token", "", "zh-CN")
			client.HTTP = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				return response(200, tc.body), nil
			})}
			season, episodes, err := client.SeasonDetail(context.Background(), 42, 2)
			if tc.wantError {
				if err == nil || len(episodes) != 0 || season != (model.Season{}) {
					t.Fatalf("expected mismatched season error; season %+v episodes %+v err %v", season, episodes, err)
				}
				return
			}
			if err != nil || season.Number != 2 || season.TMDBID != 902 || len(episodes) != 1 {
				t.Fatalf("season %+v episodes %+v err %v", season, episodes, err)
			}
		})
	}
}

func TestSeasonsIncludeMetadataAndSort(t *testing.T) {
	client, _ := New(nil, "token", "", "en-US")
	client.HTTP = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/3/tv/42" {
			t.Fatalf("unexpected TMDB request: %s", r.URL)
		}
		return response(200, `{"seasons":[{"id":902,"season_number":2,"name":"第二季","overview":"简介","air_date":"2024-05-01","poster_path":"/season.jpg","episode_count":8},{"id":901,"season_number":1,"name":"第一季","episode_count":4}]}`), nil
	})}
	seasons, err := client.Seasons(context.Background(), 42)
	if err != nil || len(seasons) != 2 || seasons[0].TMDBID != 901 || seasons[0].Number != 1 || seasons[0].Count != 4 || seasons[1].TMDBID != 902 || seasons[1].Number != 2 || seasons[1].Name != "第二季" || seasons[1].Overview != "简介" || seasons[1].Date != "2024-05-01" || seasons[1].PosterPath != "/season.jpg" || seasons[1].Count != 8 {
		t.Fatalf("seasons %+v, error %v", seasons, err)
	}
}
