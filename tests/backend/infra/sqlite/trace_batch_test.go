package sqlite

import (
	"fmt"
	"joycurator/internal/recognition"
	"reflect"
	"testing"
)

func TestBatchedTraceChildrenRemainOrderedAndRollBackOnLateFailure(t *testing.T) {
	s := openStore(t)
	v := recognition.Trace{ID: "many", Status: "running"}
	if err := s.StartTrace(v); err != nil {
		t.Fatal(err)
	}
	step := recognition.Step{RuleConfig: recognition.DefaultPipeline().Tokenizer, Inputs: []string{}, Captures: map[string]string{}, Attempts: []recognition.Attempt{}}
	for i := 0; i < 200; i++ {
		step.Inputs = append(step.Inputs, fmt.Sprint(i))
		step.Attempts = append(step.Attempts, recognition.Attempt{Input: fmt.Sprint(i), Captures: map[string]string{"x": "y"}, Trace: []recognition.ScriptEvent{{Message: fmt.Sprint(i), Data: map[string]any{"x": true}}}})
	}
	v.Status = "completed"
	if _, err := s.db.Exec(`CREATE TRIGGER reject_late_event BEFORE INSERT ON attempt_events WHEN NEW.f_message='199' BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishTrace(v, []recognition.Step{step}); err == nil {
		t.Fatal("late child failure accepted")
	}
	current, steps, err := s.TraceDetail(v.ID)
	if err != nil || current.Status != "running" || len(steps) != 0 {
		t.Fatal(current, steps, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_late_event"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishTrace(v, []recognition.Step{step}); err != nil {
		t.Fatal(err)
	}
	_, steps, err = s.TraceDetail(v.ID)
	if err != nil || len(steps) != 1 || !reflect.DeepEqual(steps[0], step) {
		t.Fatal("batch roundtrip changed order or snapshots", err)
	}
}
