package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func TestScrapeWorkRoutes(t *testing.T) {
	a, root := fixture(t)
	path := filepath.Join(root, "target", "Film", "movie.mkv")
	if err := a.store.SaveMapping(model.MediaMapping{Path: path, Type: "movie", TMDBID: 42, State: "pending", RunID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler(nil)
	for _, route := range []string{"/api/scrape/works", "/api/scrape/works/movie/42"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatal(route, w.Code)
		}
	}
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	get := func(route string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, route, nil)
		request.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		return w
	}
	w := get("/api/scrape/works?q=movie.mkv")
	var page organizer.ScrapeWorkPage
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Items[0].TMDBID != 42 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = get("/api/scrape/works/movie/42")
	var mappings []model.MediaMapping
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &mappings) != nil || len(mappings) != 1 || mappings[0].State != "unavailable" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = get("/api/scrape/works/movie/invalid"); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
	if w = get("/api/scrape/works?cursor=invalid"); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
	if w = get("/api/scrape/queue"); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "接口不存在") {
		t.Fatal(w.Code, w.Body.String())
	}
}
