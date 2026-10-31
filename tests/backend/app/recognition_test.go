package app

import (
	"context"

	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
)

func seedRecognitionCache(t *testing.T, a *App, path, body string) {
	t.Helper()
	if _, err := testDB(t, a).Exec("INSERT OR REPLACE INTO tmdb_cache VALUES(?,?,?)", path, body, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
}
func waitRecognition(t *testing.T, a *App, id string) matching.Workspace {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, err := a.dependencies.Workspace.Read(id)
		if err != nil {
			t.Fatal(err)
		}
		active := a.coordinator.RecognitionActive(id)
		if task.State != "running" && !active {
			return task
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("recognition timed out")
	return matching.Workspace{}
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
	task, err := a.dependencies.Workspace.Start([]string{file.Path}, "")
	if err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.State != "completed" || task.Completed != 1 || len(task.Items) != 1 || !task.Items[0].Selected || task.Items[0].Probe.Codec != "" {
		t.Fatalf("%+v", task)
	}
	update := matching.Update{Revision: task.Revision, Items: []matching.Edit{{Path: file.Path, Group: "custom", Type: "movie", TMDBID: 1, Season: -1, Selected: false}}}
	task, err = a.dependencies.Workspace.Update(context.Background(), task.ID, update)
	if err != nil || task.Items[0].Selected || task.Items[0].Group != "custom" {
		t.Fatal(task, err)
	}
	if _, err = a.dependencies.Workspace.Update(context.Background(), task.ID, update); err == nil {
		t.Fatal("stale update accepted")
	}
	restored, err := a.dependencies.Workspace.Read(task.ID)
	if err != nil || restored.Items[0].Group != "custom" {
		t.Fatal(restored, err)
	}
	if _, err = a.dependencies.Workspace.Start([]string{filepath.Join(root, "target")}, ""); err == nil {
		t.Fatal("unauthorized path accepted")
	}
}

func TestWorkspaceSelectionKeepsValidationUntilSeasonChanges(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "series.mkv")
	file.Season, file.Episodes = 2, []int{1}
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{file.Path}, Items: []matching.Item{{File: file, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "ready", Reason: "previous validation"}}}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	// Cached provider data differs from the previous validation. Selection alone
	// must preserve that result; an actual season change must validate again.
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	in := matching.Update{Revision: task.Revision, Items: []matching.Edit{{Path: file.Path, Group: "tv", Type: "tv", TMDBID: 2, Season: 2, Episodes: []int{1}, Selected: true}}}
	updated, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
	if err != nil || updated.Items[0].State != "ready" || !updated.Items[0].Selected {
		t.Fatal(updated, err)
	}
	in.Revision = updated.Revision
	in.Items[0].Season = 3
	updated, err = a.dependencies.Workspace.Update(context.Background(), task.ID, in)
	if err != nil || updated.Items[0].State != "review" || updated.Items[0].Selected {
		t.Fatal(updated, err)
	}
}
func TestRecognitionRetryKeepsManualEdits(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "manual.mkv")
	task := matching.Workspace{ID: model.Key(), State: "cancelled", Paths: []string{file.Path}, Items: []matching.Item{{File: file, Group: "custom", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Reason: "manual", Selected: false}}, Total: 1}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.Start(nil, task.ID); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.Items[0].Group != "custom" || task.Items[0].Reason != "manual" || task.Items[0].Selected {
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
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, _ := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
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
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{file.Path}, Items: []matching.Item{{File: file, Group: "custom", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "failed", Reason: "network", Edited: true, Selected: false}}, Total: 1}
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.Start(nil, task.ID); err != nil {
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
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{first.Path}, Items: []matching.Item{
		{File: first, Group: "custom", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Edited: true},
		{File: second, Group: "unmatched", State: "unmatched", Reason: "未找到匹配作品"},
	}}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.Start([]string{second.Path}, task.ID); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if len(task.Items) != 2 || task.Items[0].Group != "custom" || task.Items[0].Selected || task.Items[1].State != "unmatched" {
		t.Fatal(task)
	}
	// Changing selection on one work must not relabel unrelated unmatched files.
	in := matching.Update{Revision: task.Revision, Items: []matching.Edit{
		{Path: first.Path, Group: "custom", Type: "movie", TMDBID: 1, Season: first.Season, Selected: true},
		{Path: second.Path, Group: "unmatched", Season: second.Season},
	}}
	updated, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
	if err != nil || updated.Items[1].State != "unmatched" {
		t.Fatal(updated, err)
	}
}

func TestRecognitionRemoveSourceKeepsOtherSources(t *testing.T) {
	a, root := fixture(t)
	if err := os.MkdirAll(filepath.Join(root, "source", "4k"), 0755); err != nil {
		t.Fatal(err)
	}
	outer := mediaFile(t, a, root, "outer.mkv")
	inner := mediaFile(t, a, root, filepath.Join("4k", "inner.mkv"))
	outerDir := filepath.Dir(outer.Path)
	innerDir := filepath.Dir(inner.Path)
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{outerDir, innerDir}, Items: []matching.Item{
		{File: outer, Group: "a", State: "review"},
		{File: inner, Group: "b", State: "review"},
	}}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.RemoveSource(task.ID, outerDir, task.Revision-1); err == nil {
		t.Fatal("stale revision accepted")
	}
	got, err := a.dependencies.Workspace.RemoveSource(task.ID, outerDir+"/", task.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// The inner file belongs to the nested source, so it survives.
	if len(got.Paths) != 1 || got.Paths[0] != innerDir || len(got.Items) != 1 || got.Items[0].Path != inner.Path || got.Total != 1 {
		t.Fatal(got)
	}
	if _, err := a.dependencies.Workspace.RemoveSource(task.ID, "/not/added", got.Revision); err == nil {
		t.Fatal("unknown source accepted")
	}
}
