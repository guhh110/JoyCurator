package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
)

type heldRunner struct {
	organizer.Runner
	entered, release chan struct{}
	once             sync.Once
}

func (r *heldRunner) Run(ctx context.Context, op *organizer.Operation, save func() error) error {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
	}
	return r.Runner.Run(ctx, op, save)
}
func TestWorkspaceAllowsIndependentEditsAndImportWhileQueueRuns(t *testing.T) {
	a, root := fixture(t)
	first := mediaFile(t, a, root, "first.mkv")
	second := mediaFile(t, a, root, "second.mkv")
	input := previewRequest(t, a, "movie", 1, "copy", "error", first, second)
	task, err := a.store.Workspace(input.RecognitionID)
	if err != nil {
		t.Fatal(err)
	}
	task.Paths = []string{filepath.Join(root, "source")}
	task.Items[1].Selected = false
	if err = a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	c := organizer.NewCoordinator()
	t.Cleanup(c.Close)
	old := a.dependencies.Workspace
	workspace := NewWorkspace(a.store, old.settings, old.factory, old.extract, old.files, c)
	runner := &heldRunner{Runner: &organizer.Engine{FS: testAccess(t, a)}, entered: make(chan struct{}), release: make(chan struct{})}
	service := organizer.New(organizer.Dependencies{Coordinator: c, Store: a.store, Settings: a.dependencies.Config, Previews: organizer.NewPreviewCache(), Access: testAccess(t, a), Workspaces: workspace, Mappings: a.store, Sources: a.dependencies.Files, Factory: old.factory, Runner: func(func(context.Context, string) ([]byte, error)) organizer.Runner { return runner }, Revision: workspace.Revision})
	if err = service.Start(); err != nil {
		t.Fatal(err)
	}
	plan, err := service.BatchPreview(context.Background(), organizer.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy", DuplicatePolicy: "error"})
	if err != nil || len(plan.Issues) > 0 {
		t.Fatal(plan, err)
	}
	batch, err := service.ExecuteBatch(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	current, err := workspace.Read(task.ID)
	if err != nil || current.Items[0].Execution == nil || !current.Items[0].Execution.Locked {
		t.Fatal(current, err)
	}
	edit := matching.Edit{Path: second.Path, Group: "independent", Type: "movie", TMDBID: 1, Season: second.Season, Episodes: second.Episodes, Selected: false}
	updated, err := workspace.Update(context.Background(), task.ID, matching.Update{Revision: task.Revision, Items: []matching.Edit{edit}})
	if err != nil || updated.Items[1].Group != "independent" {
		t.Fatal(updated, err)
	}
	locked := edit
	locked.Path = first.Path
	if _, err = workspace.Update(context.Background(), task.ID, matching.Update{Revision: updated.Revision, Items: []matching.Edit{locked}}); err == nil {
		t.Fatal("edited queued file")
	}
	if _, err = workspace.RemoveSource(task.ID, task.Paths[0], updated.Revision); err == nil {
		t.Fatal("removed source containing locked file")
	}
	path := filepath.Join(root, "source", "new.mkv")
	if err = os.WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) { return emptySearch{}, nil }
	imported, err := workspace.Start([]string{path}, "", "organize")
	if err != nil {
		t.Fatal("organize blocked import", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for c.RecognitionActive(imported.ID) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.RecognitionActive(imported.ID) {
		t.Fatal("import did not finish")
	}
	close(runner.release)
	deadline = time.Now().Add(3 * time.Second)
	for c.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Busy() {
		t.Fatal("queue did not finish")
	}
	current, err = workspace.Read(task.ID)
	if err != nil || current.Revision != updated.Revision || !current.Items[0].Removed || current.Items[1].Group != "independent" {
		t.Fatal("progress changed editable workspace", current, err)
	}
	if record, err := service.Record(batch.Runs[0].ID); err != nil || record.State != "completed" {
		t.Fatal(record, err)
	}
}

func TestQueueAndPaginatedBatchHTTPContracts(t *testing.T) {
	a, _, _, plan := batchFixture(t)
	batch, err := a.dependencies.Organizer.ExecuteBatch(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitBatch(t, a, batch.ID)
	handler := a.Handler(nil)
	for _, path := range []string{"/api/queue", "/api/queue/items?purpose=organize", "/api/batches"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatal(path, w.Code)
		}
	}
	if err = a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	w := get("/api/queue")
	var queue organizer.QueueStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &queue) != nil || queue.Active || queue.Organize.Queued != 0 || queue.Scrape.Current != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	w = get("/api/batches?page=1")
	var batches organizer.BatchPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &batches) != nil || batches.Total != 1 || batches.Page != 1 || len(batches.Items) != 1 || batches.Items[0].ID != batch.ID || batches.Items[0].State != "completed" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, run := range batches.Items[0].Runs {
		if run.Ops != nil || run.Mappings != nil {
			t.Fatal("batch leaked details", run)
		}
	}
	w = get("/api/batches?page=2")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &batches) != nil || batches.Total != 1 || batches.Page != 2 || len(batches.Items) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = get("/api/queue/items?purpose=scrape&page=0")
	var queued organizer.RunPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &queued) != nil || queued.Page != 1 || queued.Items == nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = get("/api/queue/items?purpose=unknown"); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}
