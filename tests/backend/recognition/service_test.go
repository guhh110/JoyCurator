package recognition

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	rules              []Rule
	trace              Trace
	steps              []Step
	saveErr, errorStep error
	afterStep          func()
}

func (s *memoryStore) LoadRules() ([]Rule, error) { return s.rules, nil }
func (s *memoryStore) SaveRules(rules []Rule) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rules = rules
	return nil
}
func (s *memoryStore) StartTrace(value Trace) error { s.trace = value; s.steps = nil; return nil }
func (s *memoryStore) AppendStep(_ string, _ int, step Step) error {
	if s.errorStep != nil {
		return s.errorStep
	}
	s.steps = append(s.steps, step)
	if s.afterStep != nil {
		callback := s.afterStep
		s.afterStep = nil
		callback()
	}
	return nil
}
func (s *memoryStore) FinishTrace(value Trace) error { s.trace = value; return nil }
func (s *memoryStore) TraceHistory(int, string, string) ([]Trace, int, error) {
	return []Trace{s.trace}, 1, nil
}
func (s *memoryStore) TraceDetail(string) (Trace, []Step, error) {
	return s.trace, s.steps, nil
}
func (s *memoryStore) DeleteTrace(string) error { return nil }
func extractionFixture(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	store := &memoryStore{rules: DefaultRules()}
	service, err := New(store, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func TestInvalidOrFailedSaveKeepsCurrentChain(t *testing.T) {
	service, store := extractionFixture(t)
	before := service.Current()
	rules := service.Current().Rules()
	rules[0].Kind = "invalid"
	if err := service.Save(rules); err == nil {
		t.Fatal("invalid rules accepted")
	}
	if service.Current() != before {
		t.Fatal("invalid chain became active")
	}
	store.saveErr = errors.New("disk full")
	if err := service.Save([]Rule{}); err == nil {
		t.Fatal("storage failure ignored")
	}
	if service.Current() != before {
		t.Fatal("unpersisted rules became active")
	}
}

func TestFileUsesOneChainWhileNextFileUsesSavedChain(t *testing.T) {
	service, store := extractionFixture(t)
	count := len(service.Current().Rules())
	store.afterStep = func() {
		if err := service.Save([]Rule{}); err != nil {
			t.Fatal(err)
		}
	}
	result, id, err := service.ExtractFile(context.Background(), "/source/Show.S02E03.mkv", "/source", "workspace")
	if err != nil || id == "" || len(result.Steps) != count || len(store.steps) != count || result.Metadata.Season == nil || *result.Metadata.Season != 2 {
		t.Fatal(result.Metadata, len(result.Steps), err)
	}
	if store.trace.Status != "completed" || store.trace.WorkspaceID != "workspace" {
		t.Fatal(store.trace)
	}
	next, _, err := service.ExtractFile(context.Background(), "/source/Show.S02E03.mkv", "/source", "workspace")
	if err != nil || len(next.Steps) != 0 || next.Metadata.Title != nil {
		t.Fatal(next, err)
	}
}

func TestCancellationAndStorageFailurePreserveCompletedSteps(t *testing.T) {
	service, store := extractionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	store.afterStep = cancel
	result, _, err := service.ExtractFile(ctx, "/source/Show.S02E03.mkv", "/source", "workspace")
	if !errors.Is(err, context.Canceled) || store.trace.Status != "cancelled" || len(store.steps) != len(service.Current().Rules()) || result.Steps[1].Status != "skipped" {
		t.Fatal(store.trace.Status, len(store.steps), err)
	}
	store.errorStep = errors.New("cannot append trace")
	_, _, err = service.ExtractFile(context.Background(), "/source/file.mkv", "/source", "")
	if !errors.Is(err, store.errorStep) || store.trace.Status != "failed" {
		t.Fatal(store.trace.Status, err)
	}
}
