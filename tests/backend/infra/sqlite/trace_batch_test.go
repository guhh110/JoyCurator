package sqlite

import (
	"fmt"
	"joycurator/internal/recognition"
	"testing"
)

func TestBatchedTraceChildrenRemainOrderedAndRollBackOnLateFailure(t *testing.T) {
	s := openStore(t)
	v := recognition.Trace{ID: "many", Status: "running"}
	if err := s.StartTrace(v); err != nil {
		t.Fatal(err)
	}
	snapshot := sampleSnapshot(t)
	for i := 0; i < 200; i++ {
		snapshot.Diagnostics = append(snapshot.Diagnostics, recognition.Diagnostic{Message: fmt.Sprint(i)})
	}
	v.Status = "completed"
	if _, err := s.db.Exec(`CREATE TRIGGER reject_late_event BEFORE INSERT ON snapshot_diagnostics WHEN NEW.message='199' BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishTrace(v, snapshot); err == nil {
		t.Fatal("late child failure accepted")
	}
	current, steps, err := s.TraceDetail(v.ID)
	if err != nil || current.Status != "running" || len(steps.Documents) != 0 {
		t.Fatal(current, steps, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_late_event"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishTrace(v, snapshot); err != nil {
		t.Fatal(err)
	}
	_, steps, err = s.TraceDetail(v.ID)
	if err != nil || !sameSnapshot(t, steps, snapshot) {
		t.Fatal("batch roundtrip changed order or snapshots", err)
	}
}
