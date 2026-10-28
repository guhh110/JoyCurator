package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/tmdb"
)

func debugHasRule(result *RecognitionDebugResult, rule string) bool {
	for _, step := range result.Steps {
		if step.Rule == rule {
			return true
		}
	}
	return false
}
func TestDebugPathInput(t *testing.T) {
	settings := model.Settings{SourceRoot: "/source", TargetRoot: "/source/organized"}
	for _, tc := range []struct {
		name, path, root, wantRoot string
		source, target, valid      bool
	}{
		{"missing source file", "/source/Show/01.mkv", "", "/source", true, false, true},
		{"outside source", "/nas/Show/01.mkv", "", "/", false, false, true},
		{"custom boundary", "/nas/Show/01.mkv", "/nas", "/nas", false, false, true},
		{"target scope", "/source/organized/Film.mkv", "", "/source", true, true, true},
		{"normalized", "/source/other/../Film.mkv", "", "/source", true, false, true},
		{"not ancestor", "/source/Film.mkv", "/different", "", false, false, false},
		{"file as boundary", "/source/Film.mkv", "/source/Film.mkv", "", false, false, false},
		{"relative", "Show/01.mkv", "", "", false, false, false},
		{"windows", `C:\Show\01.mkv`, "", "", false, false, false},
		{"directory", "/source/Show/", "", "", false, false, false},
		{"control", "/source/Show\n01.mkv", "", "", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := debugPathInput(RecognitionDebugRequest{Path: tc.path, Root: tc.root, Mode: "parse"}, settings)
			if (err == nil) != tc.valid {
				t.Fatal(input, err)
			}
			if tc.valid && (input.Root != tc.wantRoot || input.WithinSource != tc.source || input.WithinTarget != tc.target || input.FileAccess != "unverified" || input.FileExistence != "unverified") {
				t.Fatal(input)
			}
		})
	}
}
func TestParseDebugNeedsNoTokenAndDoesNotPersist(t *testing.T) {
	a, _ := fixture(t)
	_, _ = a.db.Exec("UPDATE config SET token='' ")
	before := debugPersistenceSnapshot(t, a)
	result, err := a.debugRecognition(context.Background(), RecognitionDebugRequest{Path: "/nonexistent/Show/Season 2/03.mkv", Mode: "parse"})
	if err != nil || result.State != "parsed" || result.Parsed.Title != "Show" || result.Season != 2 || result.Input.Root != "/" {
		t.Fatal(result, err)
	}
	if !reflect.DeepEqual(before, debugPersistenceSnapshot(t, a)) {
		t.Fatal("debug persisted application state")
	}
	full, err := a.debugRecognition(context.Background(), RecognitionDebugRequest{Path: "/nonexistent/Show.S01E01.mkv", Mode: "full"})
	if err != nil || full.State != "failed" || full.Parsed.Title != "Show" || !debugHasRule(full, "credentials") || !debugHasRule(full, "parsed-result") {
		t.Fatal(full, err)
	}
	if !reflect.DeepEqual(before, debugPersistenceSnapshot(t, a)) {
		t.Fatal("missing-token debug persisted state")
	}
}
func debugPersistenceSnapshot(t *testing.T, a *App) string {
	t.Helper()
	parts := []string{}
	for _, table := range []string{"recognitions", "runs", "tmdb_cache"} {
		var count int
		if err := a.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, fmt.Sprintf("%s:%d", table, count))
	}
	var config string
	if err := a.db.QueryRow("SELECT value FROM config").Scan(&config); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	plans := len(a.plans)
	a.mu.Unlock()
	return strings.Join(parts, ";") + config + fmt.Sprint(plans)
}
func TestDebugMatchesProductionAndExplainsDecisions(t *testing.T) {
	for _, tc := range []struct{ name, path, want, rule string }{
		{"unique", "/source/Film.2024.mkv", "ready", "candidate-check"},
		{"ambiguous", "/source/Same.mkv", "review", "match-decision"},
		{"fallback", "/source/Fallback.2020.mkv", "review", "year-fallback"},
		{"id collision", "/source/Film [tmdb-7].mkv", "review", "id-lookup"},
		{"single season", "/source/Show/01.mkv", "ready", "infer-season"},
		{"multiple seasons", "/source/Multi/01.mkv", "review", "infer-season"},
		{"specials", "/source/Show.S00E01.mkv", "ready", "explicit-season"},
		{"invalid episode", "/source/Show.S01E03.mkv", "review", "episode-check"},
		{"unmatched", "/source/Missing.mkv", "unmatched", "match-decision"},
		{"upstream error", "/source/Failure.mkv", "failed", "request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/search/"):
					title := r.URL.Query().Get("query")
					kind := "movie"
					if strings.HasSuffix(r.URL.Path, "/tv") {
						kind = "tv"
					}
					if title == "Failure" {
						w.WriteHeader(401)
						fmt.Fprint(w, `{"private":"sensitive-upstream-body"}`)
						return
					}
					results := "[]"
					if title == "Film" && kind == "movie" {
						results = `[{"id":1,"title":"Film","release_date":"2024-01-01"}]`
					}
					if title == "Same" && kind == "movie" {
						results = `[{"id":1,"title":"Same"},{"id":2,"title":"Same"}]`
					}
					if title == "Fallback" && kind == "movie" && r.URL.Query().Get("year") == "" {
						results = `[{"id":1,"title":"Fallback","release_date":"2019-01-01"}]`
					}
					if (title == "Show" || title == "Multi") && kind == "tv" {
						results = fmt.Sprintf(`[{"id":2,"name":%q}]`, title)
					}
					fmt.Fprintf(w, `{"results":%s}`, results)
				case strings.Contains(r.URL.Path, "/season/"):
					fmt.Fprint(w, `{"episodes":[{"episode_number":1},{"episode_number":2}]}`)
				case strings.HasSuffix(r.URL.Path, "/tv/2"):
					seasons := `[{"season_number":0},{"season_number":1}]`
					if tc.name == "multiple seasons" {
						seasons = `[{"season_number":1},{"season_number":2}]`
					}
					fmt.Fprintf(w, `{"seasons":%s}`, seasons)
				default:
					fmt.Fprint(w, `{"id":7,"title":"Film","name":"Film","overview":"Description"}`)
				}
			}
			factory := func(model.Settings) (*tmdb.Client, error) { return mockRecognitionClient(t, handler), nil }
			settings := model.Settings{SourceRoot: "/source", Token: "private-config-token"}
			input, err := debugPathInput(RecognitionDebugRequest{Path: tc.path, Mode: "full"}, settings)
			if err != nil {
				t.Fatal(err)
			}
			result := recognizeDebug(context.Background(), "full", input, settings, factory)
			if result.State != tc.want || !debugHasRule(result, tc.rule) || !debugHasRule(result, "parsed-result") {
				t.Fatalf("%s: %s; %+v", tc.want, result.State, result.Steps)
			}
			client, _ := factory(settings)
			extracted, _ := recognition.DefaultEngine().Extract(context.Background(), recognition.Input{Path: input.NormalizedPath, Root: input.Root}, nil)
			parsed := parsedMetadata(extracted.Metadata)
			normal := RecognitionItem{Parsed: parsed, File: File{Season: parsed.Season, Episodes: parsed.Episodes}, Candidates: []model.Entry{}}
			memo := newRecognitionMemo(client)
			memo.match(context.Background(), &normal)
			if normal.Entry != nil {
				memo.validate(context.Background(), &normal)
			}
			if !reflect.DeepEqual(result.Parsed, parsed) || result.State != normal.State || result.Reason != normal.Reason || result.Season != normal.Season || !reflect.DeepEqual(result.Episodes, normal.Episodes) || !reflect.DeepEqual(result.Entry, normal.Entry) || !reflect.DeepEqual(result.Candidates, normal.Candidates) {
				t.Fatalf("debug diverged from production: %+v vs %+v", result, normal)
			}
			raw, _ := json.Marshal(result)
			for _, secret := range []string{"private-config-token", "test-token", "sensitive-upstream-body", "Authorization"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("debug leaked", secret)
				}
			}
		})
	}
}
func TestDebugUsesCacheAndPreservesPartialTraceOnCancel(t *testing.T) {
	a, root := fixture(t)
	settings, _ := a.settings()
	q := url.Values{"query": {"山海旅人"}, "include_adult": {"false"}, "language": {"zh-CN"}}
	seedRecognitionCache(t, a, "/search/tv?"+q.Encode(), `{"results":[{"id":2,"name":"山海旅人"}]}`)
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	input, err := debugPathInput(RecognitionDebugRequest{Path: filepath.Join(root, "source", "山海旅人.S01E01.mkv"), Mode: "full"}, settings)
	if err != nil {
		t.Fatal(err)
	}
	factory := func(s model.Settings) (*tmdb.Client, error) {
		client, err := a.client(s)
		client.HTTP = &http.Client{Transport: recognitionTransport(func(*http.Request) (*http.Response, error) {
			t.Error("cached debug attempted a network request")
			return nil, errors.New("network forbidden")
		})}
		return client, err
	}
	before := debugPersistenceSnapshot(t, a)
	result := recognizeDebug(context.Background(), "full", input, settings, factory)
	if result.State != "ready" {
		t.Fatal(result)
	}
	requests := 0
	for _, step := range result.Steps {
		if step.Rule == "request" {
			requests++
			var data map[string]any
			_ = json.Unmarshal(step.Data, &data)
			if data["cacheHit"] != true || data["attempts"] != float64(0) {
				t.Fatal(data)
			}
		}
	}
	if requests != 3 || before != debugPersistenceSnapshot(t, a) {
		t.Fatal("cache or persistence regression", requests)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped := recognizeDebug(ctx, "full", input, settings, factory)
	if stopped.State != "failed" || !debugHasRule(stopped, "parsed-result") || stopped.Steps[len(stopped.Steps)-1].Rule != "final-result" {
		t.Fatal(stopped)
	}
}
func TestDebugEndpointAuthOriginAndValidation(t *testing.T) {
	a, _ := fixture(t)
	handler := a.Handler(nil)
	request := func(token, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/recognitions/debug", strings.NewReader(body))
		if token != "" {
			r.Header.Set("Cookie", "joycurator_session="+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := `{"path":"/absent/Film.2024.mkv","mode":"parse"}`
	if w := request("", "", body); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if err := a.auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, _ := a.auth.Login(context.Background(), "admin", "fixture-password-2026")
	if w := request(token, "https://other.example", body); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, invalid := range []string{`{"path":"relative.mkv","mode":"parse"}`, `{"path":"/Film.mkv","mode":"wrong"}`, `{"path":"/Film.mkv","root":"/other","mode":"parse"}`, `{"path":"/Film.mkv","mode":"parse","unknown":true}`} {
		if w := request(token, "", invalid); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	before := debugPersistenceSnapshot(t, a)
	w := request(token, "", body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"parsed"`) || before != debugPersistenceSnapshot(t, a) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestMatrixTMDBIDIsNotAnEpisodeAndDisambiguatesMediaType(t *testing.T) {
	a, _ := fixture(t)
	path := "/source/inbox/01-movies/The.Matrix.1999.tmdb-603.mkv"
	seedRecognitionCache(t, a, "/movie/603?append_to_response=credits&language=zh-CN", `{"id":603,"title":"黑客帝国","original_title":"The Matrix","release_date":"1999-03-31","overview":"Movie fixture"}`)
	seedRecognitionCache(t, a, "/tv/603?append_to_response=credits&language=zh-CN", `{"id":603,"name":"Veronica's Closet","original_name":"Veronica's Closet","first_air_date":"1997-09-25","overview":"TV fixture"}`)
	result, err := a.debugRecognition(context.Background(), RecognitionDebugRequest{Path: path, Mode: "full"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Parsed.Type != "" || result.Parsed.Title != "The Matrix" || result.Parsed.Year != 1999 || result.Parsed.TMDBID != 603 || len(result.Parsed.Episodes) != 0 {
		t.Fatal(result.Parsed)
	}
	if result.State != "ready" || result.Entry == nil || result.Entry.Type != "movie" || result.Entry.TMDBID != 603 || len(result.Candidates) != 2 {
		t.Fatal(result)
	}
	if !debugHasRule(result, "id-filename") || !debugHasRule(result, "id-disambiguation") || debugHasRule(result, "trailing-episode") {
		t.Fatal(result.Steps)
	}
}
