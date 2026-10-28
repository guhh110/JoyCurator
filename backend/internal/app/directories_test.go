package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
)

func TestDirectoryDefaultsAndMigration(t *testing.T) {
	a, media := fixture(t)
	s, _ := a.settings()
	if s.SourceRoot != filepath.Join(media, "source") || s.TargetRoot != filepath.Join(media, "target") {
		t.Fatal("unexpected defaults")
	}
	raw := map[string]any{}
	_ = json.Unmarshal([]byte(jsonText(s)), &raw)
	delete(raw, "sourceRoot")
	delete(raw, "targetRoot")
	raw["version"], raw["sources"], raw["movieRoot"], raw["tvRoot"] = 7, []string{"/previous/source"}, "/previous/movies", "/previous/tv"
	oldRules := s.Rules
	oldRules.Categories = []model.Category{{Name: "科幻", Type: "movie"}, {Name: "纪录片", Genres: []string{"Documentary"}}}
	raw["rules"] = oldRules
	if _, err := a.db.Exec("UPDATE config SET value=?", jsonText(raw)); err != nil {
		t.Fatal(err)
	}
	if err := a.save(Run{ID: "history", State: "undone", Title: "旧记录"}); err != nil {
		t.Fatal(err)
	}
	_ = a.Close()
	roots := []string{filepath.Join(media, "source"), filepath.Join(media, "target")}
	next, err := Open(filepath.Join(filepath.Dir(media), "data"), roots, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	migrated, _ := next.settings()
	if migrated.SourceRoot != roots[0] || migrated.TargetRoot != roots[1] || migrated.Version != 8 || migrated.Token != "fixture-secret" {
		t.Fatal("settings or credential not preserved")
	}
	if len(migrated.Rules.Categories) != 3 || migrated.Rules.Categories[0].Name != "电影/科幻" || migrated.Rules.Categories[2].Name != "剧集/纪录片" {
		t.Fatal(migrated.Rules.Categories)
	}
	record, err := next.record("history")
	if err != nil || record.State != "undone" {
		t.Fatal("operation history lost")
	}
	if err := migrateDirectories(next.db, model.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	repeated, _ := next.settings()
	if jsonText(repeated) != jsonText(migrated) || repeated.Token != migrated.Token {
		t.Fatal("migration is not idempotent")
	}
	var version int
	_ = next.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 2 {
		t.Fatal(version)
	}
}

func TestSourceBoundaryAndNestedTargetExclusion(t *testing.T) {
	a, media := fixture(t)
	input := mediaFile(t, a, media, "原片.mkv")
	outside := filepath.Join(media, "outside.mkv")
	_ = os.WriteFile(outside, []byte("outside"), 0644)
	for _, path := range []string{outside, filepath.Join(media, "target")} {
		if _, err := a.scan(context.Background(), []string{path}); err == nil {
			t.Fatal("source boundary bypassed", path)
		}
	}
	if _, err := a.preview(context.Background(), PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []File{{Path: outside}}}); err == nil {
		t.Fatal("preview accepted file outside source")
	}
	s, _ := a.settings()
	s.TargetRoot = filepath.Join(s.SourceRoot, "organized")
	_ = os.MkdirAll(s.TargetRoot, 0755)
	_ = os.WriteFile(filepath.Join(s.TargetRoot, "output.mkv"), []byte("output"), 0644)
	request := httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))
	if err := a.updateSettings(httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
	files, err := a.scan(context.Background(), []string{s.SourceRoot})
	if err != nil || len(files) != 1 || files[0].Path != input.Path {
		t.Fatal(files, err)
	}
	if _, err := a.scan(context.Background(), []string{filepath.Join(s.TargetRoot, "output.mkv")}); err == nil {
		t.Fatal("target accepted as direct source")
	}
	s, _ = a.settings()
	s.TargetRoot = s.SourceRoot
	if err := a.updateSettings(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))); err == nil {
		t.Fatal("identical source/target accepted")
	}
	alias := filepath.Join(s.SourceRoot, "target-alias")
	if err := os.Symlink(s.SourceRoot, alias); err != nil {
		t.Fatal(err)
	}
	s.TargetRoot = alias
	if err := a.updateSettings(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))); err == nil {
		t.Fatal("symlink alias bypassed directory overlap check")
	}
}

func TestBrowseDefaultsToSource(t *testing.T) {
	a, media := fixture(t)
	if err := a.auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	handler := a.Handler(nil)
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("Cookie", "joycurator_rewrite="+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	root := get("/api/browse")
	if root.Code != 200 || !strings.Contains(root.Body.String(), filepath.Join(media, "source")) || strings.Contains(root.Body.String(), "target") {
		t.Fatal(root.Body.String())
	}
	if get("/api/browse?path="+filepath.Join(media, "target")).Code != 400 {
		t.Fatal("file browser exposed target")
	}
	if get("/api/browse?scope=roots&path="+filepath.Join(media, "target")).Code != 200 {
		t.Fatal("directory picker cannot access target")
	}
}
