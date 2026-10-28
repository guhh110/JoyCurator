package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/tmdb"
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
			item := RecognitionItem{Parsed: model.Parsed{Title: tc.title, Year: tc.year}}
			newRecognitionMemo(client).match(context.Background(), &item)
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
	memo := newRecognitionMemo(client)
	for i := 0; i < 3; i++ {
		item := RecognitionItem{Parsed: model.Parsed{Title: "Show", Type: "tv", Year: 2020}}
		memo.match(context.Background(), &item)
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
			item := RecognitionItem{Parsed: model.Parsed{Type: "tv", TMDBID: 7}}
			newRecognitionMemo(client).match(context.Background(), &item)
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
	item := RecognitionItem{Parsed: model.Parsed{Type: "tv", Title: "Show"}}
	newRecognitionMemo(client).match(ctx, &item)
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
			item := RecognitionItem{File: File{Season: tc.season, Episodes: tc.episodes}, Entry: &model.Entry{Type: "tv", TMDBID: 2}}
			newRecognitionMemo(client).validate(context.Background(), &item)
			if item.State != tc.state || item.Season != tc.wantSeason {
				t.Fatal(item)
			}
		})
	}
}
func seedRecognitionCache(t *testing.T, a *App, path, body string) {
	t.Helper()
	if _, err := a.db.Exec("INSERT OR REPLACE INTO tmdb_cache VALUES(?,?,?)", path, body, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
}
func waitRecognition(t *testing.T, a *App, id string) Recognition {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := a.recognition(id)
		if err != nil {
			t.Fatal(err)
		}
		a.mu.Lock()
		active := a.recognitionCancels[id] != nil
		a.mu.Unlock()
		if task.State != "running" && !active {
			return task
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("recognition timed out")
	return Recognition{}
}
func TestRecognitionPersistedWorkspace(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "长夜旅途.2024.mkv")
	for _, kind := range []string{"movie", "tv"} {
		q := url.Values{"query": {"长夜旅途"}, "year": {"2024"}, "include_adult": {"false"}, "language": {"zh-CN"}}
		if kind == "tv" {
			q.Del("year")
			q.Set("first_air_date_year", "2024")
		}
		body := `{"results":[]}`
		if kind == "movie" {
			body = `{"results":[{"id":1,"title":"长夜旅途","release_date":"2024-01-01"}]}`
		}
		seedRecognitionCache(t, a, "/search/"+kind+"?"+q.Encode(), body)
		if kind == "tv" {
			q.Del("first_air_date_year")
			seedRecognitionCache(t, a, "/search/tv?"+q.Encode(), body)
		}
	}
	task, err := a.startRecognition([]string{file.Path}, "")
	if err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.State != "completed" || task.Completed != 1 || len(task.Items) != 1 || !task.Items[0].Selected || task.Items[0].Probe.Codec != "" {
		t.Fatalf("%+v", task)
	}
	update := RecognitionUpdate{Revision: task.Revision, Items: []RecognitionEdit{{Path: file.Path, Group: "custom", Type: "movie", TMDBID: 1, Season: -1, Selected: false}}}
	task, err = a.updateRecognition(context.Background(), task.ID, update)
	if err != nil || task.Items[0].Selected || task.Items[0].Group != "custom" {
		t.Fatal(task, err)
	}
	if _, err = a.updateRecognition(context.Background(), task.ID, update); err == nil {
		t.Fatal("stale update accepted")
	}
	restored, err := a.recognition(task.ID)
	if err != nil || restored.Items[0].Group != "custom" {
		t.Fatal(restored, err)
	}
	if _, err = a.startRecognition([]string{filepath.Join(root, "target")}, ""); err == nil {
		t.Fatal("unauthorized path accepted")
	}
}
func TestRecognitionRetryKeepsManualEdits(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "manual.mkv")
	task := Recognition{ID: key(), State: "cancelled", Paths: []string{file.Path}, Items: []RecognitionItem{{File: file, Group: "custom", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Reason: "manual", Selected: false}}, Total: 1}
	if err := a.saveRecognition(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.startRecognition(nil, task.ID); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.Items[0].Group != "custom" || task.Items[0].Reason != "manual" || task.Items[0].Selected {
		t.Fatal(task)
	}
}
func TestRecognitionCancellation(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "cancel.mkv")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := Recognition{ID: key(), State: "running", Paths: []string{file.Path}, Items: []RecognitionItem{}}
	if err := a.saveRecognition(&task); err != nil {
		t.Fatal(err)
	}
	client, _ := a.client(model.DefaultSettings())
	a.wg.Add(1)
	a.recognize(ctx, cancel, task, model.Settings{SourceRoot: filepath.Join(root, "source"), TargetRoot: filepath.Join(root, "target")}, client)
	task, _ = a.recognition(task.ID)
	if task.State != "cancelled" {
		t.Fatal(task)
	}
}
func TestRecognitionRoutesAuthenticationAndOrigin(t *testing.T) {
	a, _ := fixture(t)
	for _, path := range []string{"/api/recognitions", "/api/recognitions/unknown", "/api/batches"} {
		w := httptest.NewRecorder()
		a.Handler(nil).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	if err := a.auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, _ := a.auth.Login(context.Background(), "admin", "fixture-password-2026")
	r := httptest.NewRequest("POST", "/api/recognitions", strings.NewReader(`{"paths":[]}`))
	r.Header.Set("Cookie", "joycurator_session="+token)
	r.Header.Set("Origin", "https://outside.example")
	w := httptest.NewRecorder()
	a.Handler(nil).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestRecognitionRetryPreservesFailedManualMapping(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "unrelated-name.mkv")
	file.Season, file.Episodes = 1, []int{1, 2}
	task := Recognition{ID: key(), State: "completed", Paths: []string{file.Path}, Items: []RecognitionItem{{File: file, Group: "custom", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "failed", Reason: "network", Edited: true, Selected: false}}, Total: 1}
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	if err := a.saveRecognition(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.startRecognition(nil, task.ID); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	item := task.Items[0]
	if item.Group != "custom" || item.Entry == nil || item.Entry.TMDBID != 2 || item.Season != 1 || len(item.Episodes) != 2 || item.Selected || item.State != "ready" {
		t.Fatal(item)
	}
}
func TestRecognitionAddPathsPreservesExistingWorkspace(t *testing.T) {
	a, root := fixture(t)
	first := mediaFile(t, a, root, "first.mkv")
	second := mediaFile(t, a, root, "second.mkv")
	task := Recognition{ID: key(), State: "completed", Paths: []string{first.Path}, Items: []RecognitionItem{
		{File: first, Group: "custom", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Edited: true},
		{File: second, Group: "unmatched", State: "unmatched", Reason: "未找到匹配作品"},
	}}
	if err := a.saveRecognition(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.startRecognition([]string{second.Path}, task.ID); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if len(task.Items) != 2 || task.Items[0].Group != "custom" || task.Items[0].Selected || task.Items[1].State != "unmatched" {
		t.Fatal(task)
	}
	// Changing selection on one work must not relabel unrelated unmatched files.
	in := RecognitionUpdate{Revision: task.Revision, Items: []RecognitionEdit{
		{Path: first.Path, Group: "custom", Type: "movie", TMDBID: 1, Season: first.Season, Selected: true},
		{Path: second.Path, Group: "unmatched", Season: second.Season},
	}}
	updated, err := a.updateRecognition(context.Background(), task.ID, in)
	if err != nil || updated.Items[1].State != "unmatched" {
		t.Fatal(updated, err)
	}
}
func TestRecognitionIDCollisionNeedsConfirmation(t *testing.T) {
	client := mockRecognitionClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":7,"title":"Film","name":"Show","overview":"Description"}`)
	})
	item := RecognitionItem{Parsed: model.Parsed{TMDBID: 7}}
	newRecognitionMemo(client).match(context.Background(), &item)
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
			item := RecognitionItem{Parsed: model.Parsed{TMDBID: 7, Title: "Same", Year: 2020}}
			newRecognitionMemo(client).match(context.Background(), &item)
			if item.State != tc.want {
				t.Fatal(item)
			}
			if tc.want == "ready" && (item.Entry == nil || item.Entry.Type != "tv") {
				t.Fatal(item)
			}
		})
	}
}
