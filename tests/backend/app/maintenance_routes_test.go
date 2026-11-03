package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

func TestMaintenanceRoutesAndRecordFilters(t *testing.T) {
	a, _ := fixture(t)
	h := a.Handler(nil)
	for _, path := range []string{"/api/maintenance/preview", "/api/maintenance/cleanup"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(`{"cache":true}`)))
		if w.Code != http.StatusUnauthorized {
			t.Fatal(path, w.Code)
		}
	}
	if err := a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/api/maintenance/cleanup", `{"cache":true}`, "https://foreign.invalid"); w.Code != http.StatusForbidden {
		t.Fatal(w.Code, w.Body.String())
	}
	v := recognition.Trace{ID: "old", Status: "running"}
	if err := a.store.StartTrace(v); err != nil {
		t.Fatal(err)
	}
	v.Status = "completed"
	v.FinishedAt = "2000-01-01T00:00:00Z"
	if err := a.store.FinishTrace(v, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SaveCache(context.Background(), "expired", []byte("{}"), time.Now().Unix()-100); err != nil {
		t.Fatal(err)
	}
	w := request("POST", "/api/maintenance/preview", `{"before":"2001-01-01T00:00:00Z","traces":true,"cache":true}`, "http://example.com")
	var preview model.CleanupPreview
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || preview.Traces != 1 || preview.Cache != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	body, _ := json.Marshal(preview.Request)
	w = request("POST", "/api/maintenance/cleanup", string(body), "http://example.com")
	var counts model.CleanupCounts
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &counts) != nil || counts != preview.CleanupCounts {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, purpose := range []string{"organize", "scrape"} {
		if err := a.store.SaveRun(organizer.Run{ID: purpose, Title: "山海情", Purpose: purpose, State: "failed"}); err != nil {
			t.Fatal(err)
		}
		path := "/api/runs"
		if purpose == "scrape" {
			path = "/api/scrape/runs"
		}
		w = request("GET", path+"?state=failed&search=山海情", "", "")
		var page organizer.RunPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != 1 || page.Items[0].ID != purpose {
			t.Fatal(w.Code, w.Body.String())
		}
		if w = request("GET", path+"?state=unknown", "", ""); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
}

func TestMaintenanceValidatesSelectionAndCutoffs(t *testing.T) {
	a, _ := fixture(t)
	for _, r := range []model.CleanupRequest{{}, {Traces: true}, {Traces: true, Before: "invalid"}, {Traces: true, Before: time.Now().Add(time.Hour).Format(time.RFC3339Nano)}, {Cache: true, CacheBefore: -1}, {Cache: true, CacheBefore: time.Now().Unix() + 3600}} {
		if _, err := a.dependencies.Maintenance.PreviewCleanup(r); err == nil {
			t.Fatal("invalid preview accepted", r)
		}
		if _, err := a.dependencies.Maintenance.Cleanup(r); err == nil {
			t.Fatal("invalid cleanup accepted", r)
		}
	}
	if p, err := a.dependencies.Maintenance.PreviewCleanup(model.CleanupRequest{Cache: true}); err != nil || p.Request.CacheBefore <= 0 {
		t.Fatal(p, err)
	}
}
