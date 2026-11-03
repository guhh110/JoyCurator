package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func fixture(t testing.TB) (*App, string) {
	t.Helper()
	base := t.TempDir()
	media := filepath.Join(base, "media")
	for _, dir := range []string{"source", "target"} {
		if err := os.MkdirAll(filepath.Join(media, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	a, err := openTest(t, filepath.Join(base, "data"), filepath.Join(media, "source"), filepath.Join(media, "target"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	s, _ := a.dependencies.Config.Load()
	s.Token = "fixture-secret"
	if err = a.store.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	seed := func(path, value string) {
		_, err := testDB(t, a).Exec("INSERT INTO tmdb_cache VALUES(?,?,?)", path, value, time.Now().Add(time.Hour).Unix())
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("/movie/1?append_to_response=credits&language=zh-CN", `{"id":1,"title":"长夜旅途","overview":"测试电影","release_date":"2024-01-01","genres":[{"name":"剧情"}],"production_countries":[{"iso_3166_1":"CN"}]}`)
	seed("/tv/2?append_to_response=credits&language=zh-CN", `{"id":2,"name":"山海旅人","overview":"测试剧集","first_air_date":"2023-01-01","origin_country":["CN"],"genres":[{"name":"剧情"}]}`)
	seed("/tv/2/season/1?language=zh-CN", `{"episodes":[{"episode_number":1,"name":"初见山海"},{"episode_number":2,"name":"旅途的起点"}]}`)
	seed("/tv/2/season/0?language=zh-CN", `{"episodes":[{"episode_number":1,"name":"特别篇"}]}`)
	return a, media
}
func mediaFile(t testing.TB, a *App, root, name string) model.File {
	t.Helper()
	path := filepath.Join(root, "source", name)
	if err := os.WriteFile(path, bytes.Repeat([]byte("test-video"), 64), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := a.dependencies.Config.Load()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := a.dependencies.Recognition.Parse(context.Background(), path, settings.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	season := -1
	if parsed.Metadata.Season != nil {
		season = *parsed.Metadata.Season
	}
	return model.File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano(), Season: season, Episodes: parsed.Metadata.Episodes}
}
func previewRequest(t testing.TB, a *App, kind string, id int, mode, policy string, files ...model.File) organizer.BatchPreviewRequest {
	t.Helper()
	entry := &model.Entry{Type: kind, TMDBID: id, Title: "测试作品", OriginalTitle: "测试作品", Year: 2024}
	if kind == "tv" {
		entry.Title, entry.Year = "山海旅人", 2023
	}
	if kind == "movie" {
		entry.Title = "长夜旅途"
	}
	task := matching.Workspace{ID: model.Key(), State: "completed", Items: make([]matching.Item, 0, len(files))}
	for _, file := range files {
		task.Items = append(task.Items, matching.Item{File: file, Entry: entry, State: "ready", Selected: true})
	}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	return organizer.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: mode, DuplicatePolicy: policy}
}
func done(t *testing.T, a *App, r organizer.Run) organizer.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		value, err := a.dependencies.Organizer.Record(r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if value.State != "running" {
			running := ""
			if a.coordinator.Busy() {
				running = "busy"
			}
			if running == "" {
				return value
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task timed out")
	return organizer.Run{}
}
func executePlan(t *testing.T, a *App, p *organizer.Plan) organizer.Run {
	t.Helper()
	if len(p.Issues) > 0 {
		t.Fatalf("issues %v", p.Issues)
	}
	batch, err := a.dependencies.Organizer.ExecuteBatch(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	batch = waitBatch(t, a, batch.ID)
	if len(batch.Runs) != 1 {
		t.Fatalf("expected one work, got %d", len(batch.Runs))
	}
	r, err := a.dependencies.Organizer.Record(batch.Runs[0].ID)
	if err != nil || r.State != "completed" {
		t.Fatalf("run: %+v %v", r, err)
	}
	return r
}
func TestTVPreviewExecuteUndoWithSharedArtifacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	a, root := fixture(t)
	f1 := mediaFile(t, a, root, "山海旅人.S01E01.mkv")
	f2 := mediaFile(t, a, root, "山海旅人.S01E02.mkv")
	p, err := a.dependencies.Organizer.BatchPreview(context.Background(), previewRequest(t, a, "tv", 2, "copy", "error", f1, f2))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 2 || len(p.Ops) != 2 {
		t.Fatalf("preview: %+v", p)
	}
	r := executePlan(t, a, p)
	for _, op := range r.Ops {
		if _, err = os.Stat(op.Target); err != nil {
			t.Fatal(err)
		}
	}
	u, err := a.dependencies.Organizer.Resume(r.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	u = done(t, a, u)
	if u.State != "undone" {
		t.Fatalf("undo: %+v", u)
	}
	for _, op := range r.Ops {
		if _, err = os.Stat(op.Target); !os.IsNotExist(err) {
			t.Fatalf("target remains %s", op.Target)
		}
	}
	for _, f := range []model.File{f1, f2} {
		if _, err = os.Stat(f.Path); err != nil {
			t.Fatal("source removed")
		}
	}
}
func TestSeparateOrganizeRunsOnlyRegisterVideoMappings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	a, root := fixture(t)
	f1 := mediaFile(t, a, root, "Show.S01E01.mkv")
	f2 := mediaFile(t, a, root, "Show.S01E02.mkv")
	p, _ := a.dependencies.Organizer.BatchPreview(context.Background(), previewRequest(t, a, "tv", 2, "copy", "error", f1))
	r1 := executePlan(t, a, p)
	p, err := a.dependencies.Organizer.BatchPreview(context.Background(), previewRequest(t, a, "tv", 2, "copy", "error", f2))
	if err != nil {
		t.Fatal(err)
	}
	r2 := executePlan(t, a, p)
	for _, op := range append(r1.Ops, r2.Ops...) {
		if op.Kind != "video" && op.Kind != "subtitle" {
			t.Fatal("organize produced scrape artifact", op.Kind)
		}
	}
	mappings, err := a.store.Mappings()
	if err != nil || len(mappings) != 2 {
		t.Fatalf("expected two registered videos: %+v %v", mappings, err)
	}
	u, err := a.dependencies.Organizer.Resume(r1.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, u).State != "undone" {
		t.Fatal("undo 1")
	}
	mappings, err = a.store.Mappings()
	if err != nil || len(mappings) != 1 || mappings[0].RunID != r2.ID {
		t.Fatalf("first undo affected second mapping: %+v %v", mappings, err)
	}
	u, err = a.dependencies.Organizer.Resume(r2.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, u).State != "undone" {
		t.Fatal("undo 2")
	}
	mappings, err = a.store.Mappings()
	if err != nil || len(mappings) != 0 {
		t.Fatalf("mapping retained after both undos: %+v %v", mappings, err)
	}
}
func TestStalePlanAndSourceChangesRejected(t *testing.T) {
	a, root := fixture(t)
	f := mediaFile(t, a, root, "电影.2024.mkv")
	in := previewRequest(t, a, "movie", 1, "copy", "error", f)
	p, err := a.dependencies.Organizer.BatchPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := a.dependencies.Config.Load()
	s.Version++
	_ = a.store.SaveSettings(s)
	if _, err = a.dependencies.Organizer.ExecuteBatch(p.ID); err == nil {
		t.Fatal("stale settings accepted")
	}
	p, err = a.dependencies.Organizer.BatchPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(f.Path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Organizer.ExecuteBatch(p.ID); err == nil {
		t.Fatal("changed source accepted")
	}
	if _, err = a.dependencies.Organizer.ExecuteBatch("client-controlled-payload"); err == nil {
		t.Fatal("unknown preview accepted")
	}
}
func TestNoOverwriteAndChangedTargetUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	a, root := fixture(t)
	f := mediaFile(t, a, root, "电影.2024.mkv")
	in := previewRequest(t, a, "movie", 1, "copy", "error", f)
	p, _ := a.dependencies.Organizer.BatchPreview(context.Background(), in)
	r := executePlan(t, a, p)
	in = previewRequest(t, a, "movie", 1, "copy", "error", f)
	p, err := a.dependencies.Organizer.BatchPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Issues) == 0 {
		t.Fatal("existing video allowed")
	}
	var target string
	for _, op := range r.Ops {
		if op.Kind == "video" {
			target = op.Target
		}
	}
	if err = os.WriteFile(target, []byte("user-modified"), 0644); err != nil {
		t.Fatal(err)
	}
	u, err := a.dependencies.Organizer.Resume(r.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, u).State != "failed" {
		t.Fatal("undo accepted modified target")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "user-modified" {
		t.Fatal("undo changed external target", err)
	}
}
func TestMoveAndHardlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	for _, mode := range []string{"move", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			a, root := fixture(t)
			f := mediaFile(t, a, root, "电影.2024.mkv")
			p, err := a.dependencies.Organizer.BatchPreview(context.Background(), previewRequest(t, a, "movie", 1, mode, "error", f))
			if err != nil {
				t.Fatal(err)
			}
			r := executePlan(t, a, p)
			if mode == "move" {
				if _, err = os.Stat(f.Path); !os.IsNotExist(err) {
					t.Fatal("move retained source")
				}
			}
			u, err := a.dependencies.Organizer.Resume(r.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			if done(t, a, u).State != "undone" {
				t.Fatal("undo failed")
			}
			if _, err = os.Stat(f.Path); err != nil {
				t.Fatal("source not restored")
			}
		})
	}
}
func TestHTTPAuthenticationAndCredentialRedaction(t *testing.T) {
	a, _ := fixture(t)
	h := a.Handler(nil)
	request := func(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "localhost:8081"
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/settings", "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/api/setup", `{"username":"admin","password":"fixture-password-2026"}`, nil, "https://other.invalid"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := request("POST", "/api/setup", `{"username":"admin","password":"fixture-password-2026"}`, nil, "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("missing session cookie")
	}
	w = request("GET", "/api/settings", "", cookies[0], "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "fixture-secret") {
		t.Fatal("token leaked")
	}
	var value map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &value)
	if value["hasToken"] != true {
		t.Fatal(value)
	}
	request("POST", "/api/logout", `{}`, cookies[0], "")
	if w = request("GET", "/api/settings", "", cookies[0], ""); w.Code != 401 {
		t.Fatal("logout failed")
	}
}
func TestScanExcludesTargetsSymlinksAndPreservesUnknownSeason(t *testing.T) {
	a, root := fixture(t)
	f := mediaFile(t, a, root, "01.mkv")
	if f.Season != -1 {
		t.Fatal("unknown season guessed")
	}
	special := mediaFile(t, a, root, "Show.S00E01.mkv")
	if special.Season != 0 {
		t.Fatal("special lost")
	}
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	_ = os.WriteFile(outside, []byte("test"), 0644)
	_ = os.Symlink(outside, filepath.Join(root, "source", "link.mp4"))
	_ = os.WriteFile(filepath.Join(root, "target", "output.mp4"), []byte("test"), 0644)
	files, err := scanWorkspace(t, a, []string{filepath.Join(root, "source")})
	if err != nil || len(files) != 2 {
		t.Fatalf("%v %v", files, err)
	}
}

func TestInterruptedRunCanResumeAfterRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	a, root := fixture(t)
	f := mediaFile(t, a, root, "电影.2024.mkv")
	p, err := a.dependencies.Organizer.BatchPreview(context.Background(), previewRequest(t, a, "movie", 1, "copy", "error", f))
	if err != nil {
		t.Fatal(err)
	}
	r := organizer.Run{ID: model.Key(), Title: p.Title, Mode: p.Mode, State: "running", Ops: p.Ops, Total: len(p.Ops), Purpose: "organize", Mappings: p.Mappings, Version: p.Version, RecognitionID: p.RecognitionID}
	queued := r
	queued.State = "queued"
	if err = a.store.Enqueue([]organizer.Run{queued}); err != nil {
		t.Fatal(err)
	}
	if err = a.store.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	_ = a.Close()
	next, err := openTest(t, filepath.Join(filepath.Dir(root), "data"), filepath.Join(root, "source"), filepath.Join(root, "target"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	stored, err := next.dependencies.Organizer.Record(r.ID)
	if err != nil || stored.State != "interrupted" {
		t.Fatalf("%+v %v", stored, err)
	}
	resumed, err := next.dependencies.Organizer.Resume(r.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, next, resumed).State != "completed" {
		t.Fatal("resume failed")
	}
}
