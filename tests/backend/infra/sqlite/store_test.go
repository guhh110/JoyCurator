package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

func TestWorkspaceRejectsUnknownColumnsWithoutRewritingData(t *testing.T) {
	store := openStore(t)
	w := matching.Workspace{Purpose: "organize", ID: "strict", State: "completed"}
	if err := store.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE recognitions SET unexpected_field=1 WHERE id='strict'"); err == nil {
		t.Fatal("unknown column accepted")
	}
	current, err := store.Workspace(w.ID)
	if err != nil || current.Revision != w.Revision {
		t.Fatal(current, err)
	}
}

func openStore(t testing.TB) *Store {
	t.Helper()
	s := model.DefaultSettings()
	s.SourceRoot = filepath.Join(t.TempDir(), "source")
	s.TargetRoot = filepath.Join(t.TempDir(), "target")
	store, err := Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestTraceSnapshotsPaginationAndCascade(t *testing.T) {
	store := openStore(t)
	rule := recognition.DefaultPipeline().Tokenizer
	for i := 0; i < 21; i++ {
		id := model.Key()
		if err := store.StartTrace(recognition.Trace{ID: id, Path: "/source/file.mkv", Root: "/source", Status: "running"}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishTrace(recognition.Trace{ID: id, Status: "completed", Metadata: recognition.Metadata{Episodes: []int{}}}, []recognition.Step{{RuleID: rule.ID, RuleConfig: rule, Status: "matched"}}); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := store.TraceHistory(1, "/source/file.mkv", "")
	if err != nil || len(page) != 20 || total != 21 {
		t.Fatal(len(page), total, err)
	}
	next, total, err := store.TraceHistory(2, "", "")
	if err != nil || len(next) != 1 || total != 21 {
		t.Fatal(len(next), total, err)
	}
	if err := store.SaveRules(emptyPipeline()); err != nil {
		t.Fatal(err)
	}
	_, steps, err := store.TraceDetail(page[0].ID)
	if err != nil || len(steps) != 1 || steps[0].RuleConfig.Config.Code != rule.Config.Code {
		t.Fatal(steps, err)
	}
	if err := store.DeleteTrace(page[0].ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM extraction_steps WHERE root=?", page[0].ID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := store.StartTrace(recognition.Trace{ID: "active", Path: "/file", Root: "/", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTrace("active"); err == nil {
		t.Fatal("deleted active trace")
	}
}

func TestWorkspaceRevisionAndCachePort(t *testing.T) {
	store := openStore(t)
	w := matching.Workspace{Purpose: "organize", ID: "one", Revision: 3, Items: []matching.Item{}}
	if err := store.SaveWorkspace(&w); err != nil || w.Revision != 4 {
		t.Fatal(w, err)
	}
	latest, err := store.LatestWorkspace("organize")
	if err != nil || latest.Revision != 4 {
		t.Fatal(latest, err)
	}
	ctx := context.Background()
	body := []byte(`{"id":1}`)
	if err := store.SaveCache(ctx, "key", body, 20); err != nil {
		t.Fatal(err)
	}
	value, err := store.LoadCache(ctx, "key", 10)
	if err != nil || !json.Valid(value) {
		t.Fatal(value, err)
	}
	value, err = store.LoadCache(ctx, "key", 30)
	if err != nil || value != nil {
		t.Fatal(value, err)
	}
}

func emptyPipeline() recognition.Pipeline {
	p := recognition.DefaultPipeline()
	p.Annotators = []recognition.Rule{}
	return p
}
func pipelineWith(r recognition.Rule) recognition.Pipeline {
	p := emptyPipeline()
	p.Tokenizer = r
	return p
}

func TestSharedTraceCanBeFoundByContextFilePath(t *testing.T) {
	store := openStore(t)
	trace := recognition.Trace{ID: "tree", Path: "/source/Show", Status: "running"}
	if err := store.StartTrace(trace); err != nil {
		t.Fatal(err)
	}
	trace.Status = "completed"
	if err := store.FinishTrace(trace, []recognition.Step{{Inputs: []string{"/source/Show/01.mkv"}}}); err != nil {
		t.Fatal(err)
	}
	items, total, err := store.TraceHistory(1, "/source/Show/01.mkv", "")
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != "tree" {
		t.Fatal(items, total, err)
	}
}
