package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/tmdb"
	"joycurator/internal/model"
)

var fixtureDatabases sync.Map

func openTest(t *testing.T, dir, source, target string, secure bool) (*App, error) {
	t.Helper()
	a, err := Open(dir, source, target, secure)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "joycurator.db"))
	if err != nil {
		a.Close()
		return nil, err
	}
	fixtureDatabases.Store(a, db)
	t.Cleanup(func() { fixtureDatabases.Delete(a); db.Close() })
	return a, nil
}
func testDB(t *testing.T, a *App) *sql.DB {
	t.Helper()
	v, ok := fixtureDatabases.Load(a)
	if !ok {
		t.Fatal("missing test database")
	}
	return v.(*sql.DB)
}

func testClient(t *testing.T, a *App, s model.Settings) (*tmdb.Client, error) {
	return tmdb.New(a.store, s.Token, s.ProxyURL, s.Language)
}
func testUpdateSettings(t *testing.T, a *App, w *httptest.ResponseRecorder, r *http.Request) error {
	t.Helper()
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil && !strings.Contains(err.Error(), "already initialized") {
		return err
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		return err
	}
	r.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
	a.Handler(nil).ServeHTTP(w, r)
	if w.Code != 200 {
		return fmt.Errorf("settings: %s", w.Body.String())
	}
	return nil
}

func testAccess(t *testing.T, a *App) *fsops.FS {
	t.Helper()
	s, err := a.dependencies.Config.Load()
	if err != nil {
		t.Fatal(err)
	}
	access, err := fsops.New([]string{s.SourceRoot, s.TargetRoot})
	if err != nil {
		t.Fatal(err)
	}
	return access
}
