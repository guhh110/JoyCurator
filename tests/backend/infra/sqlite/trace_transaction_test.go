package sqlite

import (
	"joycurator/internal/matching"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"testing"
)

func TestTraceFinalizationRollsBackStepsAndSummaryTogether(t *testing.T) {
	store := openStore(t)
	trace := recognition.Trace{ID: "atomic", Path: "/file.mkv", Root: "/", Status: "running"}
	if err := store.StartTrace(trace); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_trace BEFORE UPDATE ON extraction_traces BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	trace.Status = "completed"
	steps := []recognition.Step{{RuleID: "one"}, {RuleID: "two"}}
	if err := store.FinishTrace(trace, steps); err == nil {
		t.Fatal("failed commit accepted")
	}
	current, saved, err := store.TraceDetail(trace.ID)
	if err != nil || current.Status != "running" || len(saved) != 0 {
		t.Fatal("partial trace committed", current, saved, err)
	}
	if _, err := store.db.Exec("DROP TRIGGER reject_trace"); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishTrace(trace, steps); err != nil {
		t.Fatal(err)
	}
	current, saved, err = store.TraceDetail(trace.ID)
	if err != nil || current.Status != "completed" || len(saved) != 2 {
		t.Fatal(current, saved, err)
	}
	if err := store.FinishTrace(trace, steps); err == nil {
		t.Fatal("duplicate finalization accepted")
	}
}
func TestRunSummariesSkipOperationDecodingButDetailsRemainStrict(t *testing.T) {
	store := openStore(t)
	r := organizer.Run{ID: "summary", Purpose: "organize", State: "completed", BatchID: "batch", Ops: []organizer.Work{{Operation: organizer.Operation{Content: []byte("blob")}}}}
	if err := store.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	// A broken undo reference affects detail reconstruction, never summary projection.
	if _, err := store.db.Exec("PRAGMA foreign_keys=OFF; UPDATE operations SET undo_parent='missing'; PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}

	page, err := store.RunsByPurpose(1, "organize")
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Ops != nil || page.Items[0].Mappings != nil {
		t.Fatal(page, err)
	}
	batch, err := store.BatchSummaries("batch")
	if err != nil || len(batch) != 1 || batch[0].Ops != nil {
		t.Fatal(batch, err)
	}
	if _, err := store.Run("summary"); err == nil {
		t.Fatal("invalid detail accepted")
	}
	unfinished, err := store.UnfinishedRuns()
	if err != nil || len(unfinished) != 0 {
		t.Fatal(unfinished, err)
	}
	if err := store.SaveRun(organizer.Run{ID: "active", State: "queued", Purpose: "organize"}); err != nil {
		t.Fatal(err)
	}
	unfinished, err = store.UnfinishedRuns()
	if err != nil || len(unfinished) != 1 || unfinished[0].ID != "active" {
		t.Fatal(unfinished, err)
	}
}
func TestWorkspaceSaveFailureDoesNotAdvanceRevision(t *testing.T) {
	store := openStore(t)
	w := matching.Workspace{ID: "workspace", Revision: 2}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_workspace BEFORE INSERT ON recognitions BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveWorkspace(&w); err == nil || w.Revision != 2 {
		t.Fatal(w, err)
	}
}
