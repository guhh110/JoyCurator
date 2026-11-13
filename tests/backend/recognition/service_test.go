package recognition

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	rules              Pipeline
	trace              Trace
	snapshot           Snapshot
	saveErr, finishErr error
	afterStart         func()
}

func (s *memoryStore) LoadRules() (Pipeline, error) { return s.rules, nil }
func (s *memoryStore) SaveRules(rules Pipeline) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rules = rules
	return nil
}
func (s *memoryStore) StartTrace(value Trace) error {
	s.trace = value
	s.snapshot = Snapshot{}
	if s.afterStart != nil {
		f := s.afterStart
		s.afterStart = nil
		f()
	}
	return nil
}
func (s *memoryStore) FinishTrace(value Trace, snapshot Snapshot) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	s.trace = value
	s.snapshot = snapshot
	return nil
}
func (s *memoryStore) TraceHistory(int, string, string) ([]Trace, int, error) {
	return []Trace{s.trace}, 1, nil
}
func (s *memoryStore) TraceDetail(string) (Trace, Snapshot, error) {
	return s.trace, s.snapshot, nil
}
func (s *memoryStore) DeleteTrace(string) error { return nil }
func extractionFixture(t *testing.T) (*Service, *memoryStore) {
	t.Helper()
	store := &memoryStore{rules: DefaultPipeline()}
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
	rules.Tokenizer.Kind = "invalid"
	if err := service.Save(rules); err == nil {
		t.Fatal("invalid rules accepted")
	}
	if service.Current() != before {
		t.Fatal("invalid chain became active")
	}
	store.saveErr = errors.New("disk full")
	if err := service.Save(DefaultPipeline()); err == nil {
		t.Fatal("storage failure ignored")
	}
	if service.Current() != before {
		t.Fatal("unpersisted rules became active")
	}
}

func TestFileUsesOneSnapshotWhileNextUsesSavedPipeline(t *testing.T) {
	service, store := extractionFixture(t)
	count := len(service.Current().Rules().Rules())
	store.afterStart = func() {
		next := DefaultPipeline()
		next.Annotators = []Rule{}
		if err := service.Save(next); err != nil {
			t.Fatal(err)
		}
	}
	result, id, err := service.ExtractFile(context.Background(), "/source/Show.S02E03.mkv", "/source", "workspace")
	if err != nil || id == "" || len(result.Steps) != 0 || len(store.snapshot.Pipeline.Rules()) != count || result.Metadata.Season == nil || *result.Metadata.Season != 2 {
		t.Fatal(result.Metadata, len(result.Steps), err)
	}
	if store.trace.Status != "completed" || store.trace.WorkspaceID != "workspace" {
		t.Fatal(store.trace)
	}
	next, _, err := service.ExtractFile(context.Background(), "/source/Show.S02E03.mkv", "/source", "workspace")
	if err != nil || len(next.Steps) != 0 || len(store.snapshot.Pipeline.Rules()) != 2 || next.Metadata.Title != nil {
		t.Fatal(next, err)
	}
}

func TestCancellationAndStorageFailurePreserveRetryableSnapshotState(t *testing.T) {
	service, store := extractionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	store.afterStart = cancel
	result, _, err := service.ExtractFile(ctx, "/source/Show.S02E03.mkv", "/source", "workspace")
	if !errors.Is(err, context.Canceled) || store.trace.Status != "cancelled" || len(store.snapshot.Diagnostics) != 0 || len(result.Steps) != 0 {
		t.Fatal(store.trace.Status, len(store.snapshot.Documents), err)
	}
	store.finishErr = errors.New("cannot append trace")
	_, _, err = service.ExtractFile(context.Background(), "/source/file.mkv", "/source", "")
	if !errors.Is(err, store.finishErr) || store.trace.Status != "running" {
		t.Fatal(store.trace.Status, err)
	}
}
