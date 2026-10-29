package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	s := model.DefaultSettings()
	s.SourceRoot = "/source"
	s.TargetRoot = "/target"
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

func TestLegacyRowsRecoverWithoutRewritingCompletedHistory(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	// Build pre-refactor tables and JSON directly rather than round-tripping new DTOs.
	_, err = db.Exec(`CREATE TABLE config(id INTEGER PRIMARY KEY,value TEXT,token TEXT);
 CREATE TABLE runs(id TEXT PRIMARY KEY,value TEXT);
 CREATE TABLE recognitions(id TEXT PRIMARY KEY,value TEXT);
 INSERT INTO config VALUES(1,'{"version":4,"sourceRoot":"/source","targetRoot":"/target","proxyURL":"","language":"zh-CN","nfo":false,"images":false,"rules":{"movieFolder":"movies","movieFile":"{title}.{ext}","tvFolder":"tv","tvFile":"{title}.{ext}","categories":[]}}','stored-token');
 INSERT INTO runs VALUES('old','{"id":"old","state":"completed","ops":[{"kind":"video","source":"/source/file.mkv","target":"/target/file.mkv","status":"completed","hash":"sha","identity":"identity","temp":"/target/.part"}]}');
 INSERT INTO runs VALUES('queue','{"id":"queue","state":"queued","batchId":"batch","version":4}');
 INSERT INTO recognitions VALUES('workspace','{"id":"workspace","revision":9,"state":"running","items":[{"path":"/source/file.mkv","group":"manual","edited":true,"selected":false,"season":0,"episodes":[1]}]}');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := Open(dir, model.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config, err := store.LoadSettings()
	if err != nil || config.Version != 4 || config.Token != "stored-token" {
		t.Fatal(config, err)
	}
	old, err := store.Run("old")
	if err != nil || old.State != "completed" || old.Ops[0].Hash != "sha" || old.Ops[0].Temp != "/target/.part" {
		t.Fatal(old, err)
	}
	queued, err := store.Run("queue")
	if err != nil || queued.State != "interrupted" || queued.BatchID != "batch" {
		t.Fatal(queued, err)
	}
	workspace, err := store.Workspace("workspace")
	if err != nil || workspace.State != "interrupted" || workspace.Revision != 9 || !workspace.Items[0].Edited || workspace.Items[0].Season != 0 {
		t.Fatal(workspace, err)
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
