package app

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/planning"
)

func TestDirectoryDefaults(t *testing.T) {
	a, media := fixture(t)
	s, err := a.dependencies.Config.Load()
	if err != nil || s.SourceRoot != filepath.Join(media, "source") || s.TargetRoot != filepath.Join(media, "target") {
		t.Fatal("unexpected defaults", err)
	}
}

func TestSettingsRejectUnknownFields(t *testing.T) {
	a, _ := fixture(t)
	s, _ := a.dependencies.Config.Load()
	raw := strings.TrimSuffix(jsonText(s), "}") + `,"unexpectedField":true}`
	if _, err := testDB(t, a).Exec("UPDATE config SET value=?", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Config.Load(); err == nil {
		t.Fatal("unknown settings field accepted")
	}
}

func TestSourceBoundaryAndNestedTargetExclusion(t *testing.T) {
	a, media := fixture(t)
	input := mediaFile(t, a, media, "原片.mkv")
	outside := filepath.Join(media, "outside.mkv")
	_ = os.WriteFile(outside, []byte("outside"), 0644)
	for _, path := range []string{outside, filepath.Join(media, "target")} {
		if _, err := a.dependencies.Files.Scan(context.Background(), []string{path}); err == nil {
			t.Fatal("source boundary bypassed", path)
		}
	}
	if _, err := a.dependencies.Planning.Preview(context.Background(), planning.PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{{Path: outside}}}); err == nil {
		t.Fatal("preview accepted file outside source")
	}
	s, _ := a.dependencies.Config.Load()
	s.TargetRoot = filepath.Join(s.SourceRoot, "organized")
	_ = os.MkdirAll(s.TargetRoot, 0755)
	_ = os.WriteFile(filepath.Join(s.TargetRoot, "output.mkv"), []byte("output"), 0644)
	request := httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))
	if err := testUpdateSettings(t, a, httptest.NewRecorder(), request); err != nil {
		t.Fatal(err)
	}
	files, err := a.dependencies.Files.Scan(context.Background(), []string{s.SourceRoot})
	if err != nil || len(files) != 1 || files[0].Path != input.Path {
		t.Fatal(files, err)
	}
	if _, err := a.dependencies.Files.Scan(context.Background(), []string{filepath.Join(s.TargetRoot, "output.mkv")}); err == nil {
		t.Fatal("target accepted as direct source")
	}
	s, _ = a.dependencies.Config.Load()
	s.TargetRoot = s.SourceRoot
	if err := testUpdateSettings(t, a, httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))); err == nil {
		t.Fatal("identical source/target accepted")
	}
	alias := filepath.Join(s.SourceRoot, "target-alias")
	if err := os.Symlink(s.SourceRoot, alias); err != nil {
		t.Fatal(err)
	}
	s.TargetRoot = alias
	if err := testUpdateSettings(t, a, httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(jsonText(s)))); err == nil {
		t.Fatal("symlink alias bypassed directory overlap check")
	}
}

func TestBrowseDefaultsToSource(t *testing.T) {
	a, media := fixture(t)
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	handler := a.Handler(nil)
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("Cookie", "joycurator_session="+token)
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
