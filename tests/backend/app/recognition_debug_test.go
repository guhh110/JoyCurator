package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func debugHasRule(result *RecognitionDebugResult, rule string) bool {
	for _, step := range result.Steps {
		if step.Rule == rule {
			return true
		}
	}
	return false
}
func TestParseDebugNeedsNoTokenAndDoesNotPersist(t *testing.T) {
	a, _ := fixture(t)
	_, _ = testDB(t, a).Exec("UPDATE config SET token='' ")
	before := debugPersistenceSnapshot(t, a)
	result, err := a.dependencies.Workspace.Debug(context.Background(), RecognitionDebugRequest{Path: "/nonexistent/Show/Season 2/03.mkv", Mode: "parse"})
	if err != nil || result.State != "parsed" || result.Parsed.Title != "Show" || result.Season != 2 || result.Input.Root != "/" {
		t.Fatal(result, err)
	}
	if !reflect.DeepEqual(before, debugPersistenceSnapshot(t, a)) {
		t.Fatal("debug persisted application state")
	}
	full, err := a.dependencies.Workspace.Debug(context.Background(), RecognitionDebugRequest{Path: "/nonexistent/Show.S01E01.mkv", Mode: "full"})
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
		if err := testDB(t, a).QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, fmt.Sprintf("%s:%d", table, count))
	}
	var config string
	if err := testDB(t, a).QueryRow("SELECT value FROM config").Scan(&config); err != nil {
		t.Fatal(err)
	}
	return strings.Join(parts, ";") + config
}
func TestDebugUsesCacheAndPreservesPartialTraceOnCancel(t *testing.T) {
	a, root := fixture(t)

	q := url.Values{"query": {"山海旅人"}, "include_adult": {"false"}, "language": {"zh-CN"}}
	seedRecognitionCache(t, a, "/search/tv?"+q.Encode(), `{"results":[{"id":2,"name":"山海旅人"}]}`)
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	request := RecognitionDebugRequest{Path: filepath.Join(root, "source", "山海旅人.S01E01.mkv"), Mode: "full"}
	before := debugPersistenceSnapshot(t, a)
	result, err := a.dependencies.Workspace.Debug(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
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
	stopped, err := a.dependencies.Workspace.Debug(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, _ := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
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
	result, err := a.dependencies.Workspace.Debug(context.Background(), RecognitionDebugRequest{Path: path, Mode: "full"})
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
