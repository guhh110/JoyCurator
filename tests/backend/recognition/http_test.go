package recognition_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"joycurator/internal/app"
	r "joycurator/internal/recognition"
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
func TestHTTPConfigurationSimulationPersistenceAndHistory(t *testing.T) {
	s := newService(t)
	library := decodeResponse[struct{ Rules []r.Rule }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	if len(library.Rules) == 0 {
		t.Fatal("defaults not installed")
	}
	path := filepath.Join(s.source, "Show.S00E01-E03.mkv")
	initial := s.history(t).Total
	simulated := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 200))
	if *simulated.Metadata.Season != 0 || !reflect.DeepEqual(simulated.Metadata.Episodes, []int{1, 2, 3}) || s.history(t).Total != initial {
		t.Fatal(simulated.Metadata)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("simulation created a file")
	}
	// Simulations accept an unsaved chain without changing current model.
	custom := regexRule("custom", 1, r.Title, `(?P<value>Show)`)
	custom.Config = r.Config{Pattern: "Show", Value: json.RawMessage(`"Custom title"`)}
	unsaved := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path, "rules": []r.Rule{custom}}, 200))
	if *unsaved.Metadata.Title != "Custom title" {
		t.Fatal(unsaved.Metadata)
	}
	current := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 200))
	if *current.Metadata.Title != "Show" {
		t.Fatal("trial saved rules")
	}
	// Invalid saves are atomic and retain the active chain.
	duplicate := custom
	duplicate.ID = "duplicate"
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{custom, duplicate}}, 400)
	current = decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 200))
	if *current.Metadata.Title != "Show" {
		t.Fatal("invalid save replaced rules")
	}
	custom.Examples = []r.Example{{Path: path, Expected: json.RawMessage(`"Custom title"`)}, {Path: "/Other.mkv", Expected: json.RawMessage(`null`)}}
	examples := decodeResponse[[]struct{ Passed bool }](t, s.call(t, "POST", "/api/extraction/examples", map[string]any{"rules": []r.Rule{custom}}, 200))
	if len(examples) != 2 || !examples[0].Passed || !examples[1].Passed {
		t.Fatal(examples)
	}
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{custom}}, 200)
	if err := os.WriteFile(path, []byte("dummy media"), 0644); err != nil {
		t.Fatal(err)
	}
	// Formal scan and debug share exactly the same engine and produce identical metadata.
	s.call(t, "POST", "/api/scan", map[string]any{"paths": []string{path}}, 200)
	history := s.history(t)
	if history.Total != initial+1 || history.Items[0].StepCount != 1 {
		t.Fatal(history)
	}
	traceID := history.Items[0].ID
	detail := decodeResponse[r.Result](t, s.call(t, "GET", "/api/extraction/traces/"+traceID, nil, 200))
	if !reflect.DeepEqual(detail.Metadata, unsaved.Metadata) {
		t.Fatal(detail.Metadata, unsaved.Metadata)
	}
	// Saving and restarting does not reinterpret historical snapshots.
	custom.Name = "changed name"
	custom.Config.Value = json.RawMessage(`"Next title"`)
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{custom}}, 200)
	if err := s.app.Close(); err != nil {
		t.Fatal(err)
	}
	s.app = nil
	s.open(t)
	after := decodeResponse[r.Result](t, s.call(t, "GET", "/api/extraction/traces/"+traceID, nil, 200))
	if !reflect.DeepEqual(after, detail) {
		t.Fatal("historical trace changed")
	}
	current = decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 200))
	if *current.Metadata.Title != "Next title" {
		t.Fatal("saved chain not restored")
	}
	s.call(t, "DELETE", "/api/extraction/traces/"+traceID, nil, 200)
	s.call(t, "GET", "/api/extraction/traces/"+traceID, nil, 400)
	if s.history(t).Total != initial {
		t.Fatal("history not deleted")
	}
	db, err := sql.Open("sqlite", filepath.Join(s.base, "data", "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var steps int
	if err = db.QueryRow("SELECT COUNT(*) FROM extraction_steps WHERE trace_id=?", traceID).Scan(&steps); err != nil || steps != 0 {
		t.Fatal("orphan steps", steps, err)
	}
}
func TestHTTPAuthorizationBoundaryAndNoToken(t *testing.T) {
	s := newService(t)
	cookie := s.cookie
	s.cookie = nil
	s.call(t, "GET", "/api/extraction/rules", nil, 401)
	s.call(t, "POST", "/api/extraction/test", map[string]any{"path": "/Film.mkv"}, 401)
	s.cookie = cookie
	for _, body := range []map[string]any{{"path": "relative.mkv"}, {"path": "/Film.mkv", "root": "/elsewhere"}, {"path": "/Film.mkv", "root": "/Film.mkv"}} {
		s.call(t, "POST", "/api/extraction/test", body, 400)
	}
	s.call(t, "POST", "/api/extraction/test", map[string]any{"path": "/nonexistent/Film.2024.mkv"}, 200)
	request := httptest.NewRequest("PUT", "/api/extraction/rules", bytes.NewBufferString(`{"rules":[]}`))
	request.AddCookie(cookie)
	request.Header.Set("Origin", "https://evil.invalid")
	response := httptest.NewRecorder()
	s.handler.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal(response.Code)
	}
}
func TestHistoryPaginationRestartInterruptionAndCleanup(t *testing.T) {
	s := newService(t)
	if err := s.app.Close(); err != nil {
		t.Fatal(err)
	}
	s.app = nil
	db, err := sql.Open("sqlite", filepath.Join(s.base, "data", "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 23; i++ {
		state := "completed"
		if i == 22 {
			state = "running"
		}
		_, err = db.Exec("INSERT INTO extraction_traces(id,path,root,workspace_id,status,started_at) VALUES(?,?,?,?,?,?)", time.Unix(int64(i), 0).Format(time.RFC3339), "/Film.mkv", "/", "", state, time.Now().Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	s.open(t)
	history := s.history(t)
	if history.Total != 23 || len(history.Items) != 20 || history.Items[0].Status != "interrupted" {
		t.Fatal(history)
	}
	second := decodeResponse[struct {
		Items       []any
		Total, Page int
	}](t, s.call(t, "GET", "/api/extraction/traces?page=2", nil, 200))
	if len(second.Items) != 3 || second.Page != 2 {
		t.Fatal(second)
	}
	s.call(t, "DELETE", "/api/extraction/traces/"+history.Items[0].ID, nil, 200)
}

func TestSavedLegacyChainMigratesAndHistoryKeepsOriginalSnapshot(t *testing.T) {
	s := newService(t)
	if err := s.app.Close(); err != nil {
		t.Fatal(err)
	}
	s.app = nil
	db, err := sql.Open("sqlite", filepath.Join(s.base, "data", "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	old := []r.Rule{{ID: "my-year", Name: "custom year", Target: r.Year, Source: r.Filename, Order: 17, Enabled: true, Kind: "builtin", Config: r.Config{Handler: "year", Params: json.RawMessage(`{"min":2000,"max":2030}`)}, Overwrite: true, Examples: []r.Example{{Path: "/Film.2024.mkv", Expected: json.RawMessage(`2024`)}}}}
	raw, _ := json.Marshal(old)
	if _, err = db.Exec("UPDATE extraction_rules SET value=? WHERE id=1", string(raw)); err != nil {
		t.Fatal(err)
	}
	stepRaw, _ := json.Marshal(r.Step{RuleID: old[0].ID, RuleName: old[0].Name, RuleConfig: old[0], Target: r.Year, Status: "matched", ExtractedValue: 2024, After: 2024})
	if _, err = db.Exec("INSERT INTO extraction_traces(id,path,root,workspace_id,status,started_at) VALUES('old','/Film.2024.mkv','/','','completed','2024-01-01')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO extraction_steps(trace_id,position,value) VALUES('old',1,?)", string(stepRaw)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s.open(t)
	library := decodeResponse[struct{ Rules []r.Rule }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	if len(library.Rules) != 1 || library.Rules[0].ID != "my-year" || library.Rules[0].Order != 17 || !library.Rules[0].Overwrite || library.Rules[0].Config.Code == "" || library.Rules[0].Kind != "script" {
		t.Fatal(library)
	}
	detail := decodeResponse[r.Result](t, s.call(t, "GET", "/api/extraction/traces/old", nil, 200))
	if len(detail.Steps) != 1 || detail.Steps[0].RuleConfig.Kind != "builtin" || detail.Steps[0].RuleConfig.Config.Code != "" {
		t.Fatal("old trace reinterpreted", detail)
	}
	result := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": "/Film.2024.mkv"}, 200))
	if result.Metadata.Year == nil || *result.Metadata.Year != 2024 || len(result.Steps[0].Attempts[0].Trace) == 0 {
		t.Fatal(result)
	}
	db, err = sql.Open("sqlite", filepath.Join(s.base, "data", "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var saved string
	if err = db.QueryRow("SELECT value FROM extraction_rules WHERE id=1").Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saved, `"kind":"script"`) || strings.Contains(saved, `"handler"`) {
		t.Fatal("migration not saved")
	}
}
func TestScriptHTTPTrialInvalidSaveAndFormalScan(t *testing.T) {
	s := newService(t)
	path := filepath.Join(s.source, "Film.mkv")
	rule := scriptRule("script", r.Title, "def extract(ctx):\n    trace(\"source\", ctx.text)\n    return ctx.params[\"title\"]\n")
	rule.Config.Params = json.RawMessage(`{"title":"Editable"}`)
	before := s.history(t).Total
	trial := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path, "rules": []r.Rule{rule}}, 200))
	if *trial.Metadata.Title != "Editable" || s.history(t).Total != before {
		t.Fatal(trial)
	}
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{rule}}, 200)
	invalid := rule
	invalid.Config.Code = "def extract(:"
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{invalid}}, 400)
	current := decodeResponse[r.Result](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 200))
	if !reflect.DeepEqual(current.Metadata, trial.Metadata) {
		t.Fatal("invalid save changed active chain")
	}
	if err := os.WriteFile(path, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	s.call(t, "POST", "/api/scan", map[string]any{"paths": []string{path}}, 200)
	history := s.history(t)
	formal := decodeResponse[r.Result](t, s.call(t, "GET", "/api/extraction/traces/"+history.Items[0].ID, nil, 200))
	if !reflect.DeepEqual(formal.Metadata, trial.Metadata) || formal.Steps[0].RuleConfig.Config.Code != rule.Config.Code || len(formal.Steps[0].Attempts[0].Trace) != 3 {
		t.Fatal(formal)
	}
	changed := rule
	changed.Config.Code = "def extract(ctx):\n    return \"Changed\""
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []r.Rule{changed}}, 200)
	persisted := decodeResponse[r.Result](t, s.call(t, "GET", "/api/extraction/traces/"+history.Items[0].ID, nil, 200))
	if !reflect.DeepEqual(persisted, formal) {
		t.Fatal("editing changed history")
	}
	s.call(t, "DELETE", "/api/extraction/traces/"+history.Items[0].ID, nil, 200)
}
