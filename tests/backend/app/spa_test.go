package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAPageRefreshAndMissingAssets(t *testing.T) {
	a, _ := fixture(t)
	ui := fstest.MapFS{"index.html": {Data: []byte(`<html><div id="app">SPA entry</div></html>`)}, "assets/app.js": {Data: []byte("export default true")}}
	handler := a.Handler(ui)
	for _, path := range []string{"/", "/organize", "/records", "/settings", "/recognition/rules", "/recognition/history/trace-123?page=2", "/recognition/debug", "/unknown-page"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), "SPA entry") {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/assets/missing.js", "/assets/missing", "/favicon.ico"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 404 || strings.Contains(response.Body.String(), "SPA entry") {
			t.Fatalf("missing asset %s: %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/recognition/debug", nil))
	if response.Code != 200 || response.Body.Len() != 0 {
		t.Fatal("HEAD navigation", response.Code, response.Body.String())
	}
	if err := a.auth.Setup(context.Background(), "test", "test-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.auth.Login(context.Background(), "test", "test-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	request.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 404 || !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API was replaced by SPA", response.Code, response.Body.String())
	}
}
