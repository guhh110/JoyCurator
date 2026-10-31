package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

func TestWorkspaceRejectsUnknownFieldsWithoutRewritingData(t *testing.T) {
	store := openStore(t)
	raw := `{"id":"invalid","revision":1,"state":"completed","items":[],"unexpectedField":true}`
	if _, err := store.db.Exec("INSERT INTO recognitions(id,value) VALUES(?,?)", "invalid", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Workspace("invalid"); err == nil || !strings.Contains(err.Error(), "unexpectedField") {
		t.Fatal("unsupported data silently accepted", err)
	}
	if _, err := store.LatestWorkspace(); err == nil {
		t.Fatal("unsupported latest workspace silently accepted")
	}
	var stored string
	if err := store.db.QueryRow("SELECT value FROM recognitions WHERE id='invalid'").Scan(&stored); err != nil || stored != raw {
		t.Fatal("invalid data was rewritten", stored, err)
	}
}

func openStore(t *testing.T) *Store {
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

func TestBatchTransactionRollsBackEveryQueuedRecord(t *testing.T) {
	store := openStore(t)
	runs := []organizer.Run{{ID: "duplicate", State: "queued", BatchID: "batch"}, {ID: "duplicate", State: "queued", BatchID: "batch"}}
	if err := store.SaveBatch(runs); err == nil {
		t.Fatal("duplicate primary key accepted")
	}
	all, err := store.AllRuns()
	if err != nil || len(all) != 0 {
		t.Fatalf("partial transaction persisted: %+v %v", all, err)
	}
}

func TestTraceSnapshotsPaginationAndCascade(t *testing.T) {
	store := openStore(t)
	rule := recognition.DefaultRules()[0]
	for i := 0; i < 21; i++ {
		id := model.Key()
		if err := store.StartTrace(recognition.Trace{ID: id, Path: "/source/file.mkv", Root: "/source", Status: "running"}); err != nil {
			t.Fatal(err)
		}
		if err := store.AppendStep(id, 1, recognition.Step{RuleID: rule.ID, RuleConfig: rule, Status: "matched"}); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishTrace(recognition.Trace{ID: id, Status: "completed", Metadata: recognition.Metadata{Episodes: []int{}}}); err != nil {
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
	if err := store.SaveRules([]recognition.Rule{}); err != nil {
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
	if err := store.db.QueryRow("SELECT COUNT(*) FROM extraction_steps WHERE trace_id=?", page[0].ID).Scan(&count); err != nil || count != 0 {
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
	w := matching.Workspace{ID: "one", Revision: 3, Items: []matching.Item{}}
	if err := store.SaveWorkspace(&w); err != nil || w.Revision != 4 {
		t.Fatal(w, err)
	}
	latest, err := store.LatestWorkspace()
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
