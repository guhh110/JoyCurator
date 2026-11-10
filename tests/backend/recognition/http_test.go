package recognition_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"joycurator/internal/app"
)

type service struct {
	app                  *app.App
	handler              http.Handler
	cookie               *http.Cookie
	base, source, target string
}

func newService(t *testing.T) *service {
	t.Helper()
	s := &service{base: t.TempDir()}
	s.source = filepath.Join(s.base, "source")
	s.target = filepath.Join(s.base, "target")
	for _, p := range []string{s.source, s.target} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	s.open(t)
	t.Cleanup(func() {
		if s.app != nil {
			_ = s.app.Close()
		}
	})
	response := s.call(t, "POST", "/api/setup", map[string]string{"username": "test", "password": "test-password-2026"}, 200)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session")
	}
	s.cookie = cookies[0]
	return s
}
func (s *service) open(t *testing.T) {
	t.Helper()
	var err error
	s.app, err = app.Open(filepath.Join(s.base, "data"), s.source, s.target, false)
	if err != nil {
		t.Fatal(err)
	}
	s.handler = s.app.Handler(nil)
}
func (s *service) call(t *testing.T, method, path string, body any, code int) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if s.cookie != nil {
		request.AddCookie(s.cookie)
	}
	response := httptest.NewRecorder()
	s.handler.ServeHTTP(response, request)
	if response.Code != code {
		t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
	}
	return response
}
func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func (s *service) history(t *testing.T) struct {
	Items []struct {
		ID, Status string
		StepCount  int
	}
	Total, Page int
} {
	return decodeResponse[struct {
		Items []struct {
			ID, Status string
			StepCount  int
		}
		Total, Page int
	}](t, s.call(t, "GET", "/api/extraction/traces", nil, 200))
}
