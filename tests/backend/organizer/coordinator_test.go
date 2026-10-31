package organizer

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/planning"
	"joycurator/internal/trace"
)

// Ports are simulated here so admission and worker tests don't depend on SQL,
// network, file sizes or timing-sensitive real copies.
type memoryRuns struct {
	mu       sync.Mutex
	items    map[string]Run
	order    []string
	batchErr error
}

func snapshot[T any](value T) T {
	raw, _ := json.Marshal(value)
	var result T
	_ = json.Unmarshal(raw, &result)
	return result
}
func (m *memoryRuns) SaveRun(r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[r.ID]; !ok {
		m.order = append(m.order, r.ID)
	}
	m.items[r.ID] = snapshot(r)
	return nil
}
func (m *memoryRuns) Run(id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.items[id]
	if !ok {
		return r, errors.New("missing run")
	}
	return snapshot(r), nil
}
func (m *memoryRuns) Runs(int) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]any{"total": len(m.items)}, nil
}
func (m *memoryRuns) BatchRuns(id string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Run{}
	for _, key := range m.order {
		if r := m.items[key]; r.BatchID == id {
			out = append(out, snapshot(r))
		}
	}
	return out, nil
}
func (m *memoryRuns) LatestBatchID() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.order) - 1; i >= 0; i-- {
		if id := m.items[m.order[i]].BatchID; id != "" {
			return id, nil
		}
	}
	return "", nil
}
func (m *memoryRuns) SaveBatch(runs []Run) error {
	if m.batchErr != nil {
		return m.batchErr
	}
	for _, r := range runs {
		if err := m.SaveRun(r); err != nil {
			return err
		}
	}
	return nil
}
func (m *memoryRuns) Shared(string, string, string) (bool, error)                 { return false, nil }
func (m *memoryRuns) SaveMapping(model.MediaMapping) error                        { return nil }
func (m *memoryRuns) DeleteMapping(model.MediaMapping) error                      { return nil }
func (m *memoryRuns) UpdateMappingState(model.MediaMapping, string, string) error { return nil }
func (m *memoryRuns) Mapping(string) (*model.MediaMapping, error) {
	return nil, nil
}
func (m *memoryRuns) RunsByPurpose(page int, purpose string) (map[string]any, error) {
	return m.Runs(page)
}

type fixedSettings struct{}

func (fixedSettings) Load() (model.Settings, error) {
	return model.Settings{Version: 1}, nil
}

type allowedSources struct{}

func (allowedSources) SourceAllowed(string, model.Settings) bool { return true }

type fakePlans struct {
	values map[string]*planning.Plan
	check  func([]model.Work) error
}

func (p *fakePlans) Get(id string) *planning.Plan { return p.values[id] }
func (p *fakePlans) Remove(id string)             { delete(p.values, id) }
func (p *fakePlans) ValidateOps(ops []model.Work) error {
	if p.check != nil {
		return p.check(ops)
	}
	return nil
}

type noCatalog struct{}

func (noCatalog) Search(context.Context, string, string, int) ([]model.Entry, error) {
	return nil, nil
}
func (noCatalog) Detail(context.Context, string, int) (model.Entry, error) {
	return model.Entry{}, nil
}
func (noCatalog) Seasons(context.Context, int) ([]model.Season, error) { return nil, nil }
func (noCatalog) SeasonDetail(context.Context, int, int) (model.Season, []model.Episode, error) {
	return model.Season{}, nil, nil
}
func (noCatalog) SeasonEpisodes(context.Context, int, int) ([]model.Episode, error) {
	return nil, nil
}
func (noCatalog) Artwork(context.Context, string, int, string) ([]model.Artwork, error) {
	return nil, nil
}
func (noCatalog) Image(context.Context, string) ([]byte, error) { return nil, nil }

type operationRunner struct {
	run func(context.Context, *model.Operation, func() error) error
}

func (r operationRunner) Run(ctx context.Context, op *model.Operation, save func() error) error {
	if r.run != nil {
		return r.run(ctx, op, save)
	}
	op.Status = "completed"
	return save()
}
func (r operationRunner) Undo(_ context.Context, op *model.Operation, save func() error) error {
	op.Status = "undone"
	return save()
}
func executionFixture(t *testing.T, run func(context.Context, *model.Operation, func() error) error) (*Service, *memoryRuns, *fakePlans) {
	t.Helper()
	store := &memoryRuns{items: map[string]Run{}}
	plans := &fakePlans{values: map[string]*planning.Plan{}}
	op := model.Work{Operation: model.Operation{Kind: "video", Source: "source", Target: "target"}}
	plans.values["manual"] = &planning.Plan{ID: "manual", Version: 1, Expires: time.Now().Add(time.Hour), Ops: []model.Work{op}}
	plans.values["batch"] = &planning.Plan{ID: "batch", Version: 1, Revision: 1, RecognitionID: "workspace", Expires: time.Now().Add(time.Hour), Ops: []model.Work{op, op}, Groups: []*planning.Plan{{Title: "first", Ops: []model.Work{op}}, {Title: "second", Ops: []model.Work{op}}}}
	factory := matching.Factory(func(model.Settings, *trace.Recorder) (matching.Provider, error) { return noCatalog{}, nil })
	service := New(NewCoordinator(), store, fixedSettings{}, plans, allowedSources{}, factory, func(func(context.Context, string) ([]byte, error)) Runner { return operationRunner{run} }, func(string) (int, error) { return 1, nil }, func(Run) {})
	t.Cleanup(service.coordinator.Close)
	return service, store, plans
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not reach expected state")
	}
}
func awaitIdle(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !s.coordinator.Busy() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("execution remained busy")
}

func TestExecutionCompetitionDoesNotWriteExtraRecord(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, op *model.Operation, save func() error) error {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			op.Status = "completed"
			return save()
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if _, err := s.Execute("manual"); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err := s.ExecuteBatch("batch"); !errors.Is(err, ErrBusy) {
		t.Fatalf("queue bypassed task: %v", err)
	}
	records, _ := store.Runs(1)
	if records["total"] != 1 {
		t.Fatal(records)
	}
	close(release)
	awaitIdle(t, s)
}

func TestBatchFailureContinuesAndIndependentRetryUndo(t *testing.T) {
	calls := 0
	s, store, _ := executionFixture(t, func(_ context.Context, op *model.Operation, save func() error) error {
		calls++
		if calls == 1 {
			return errors.New("first work failed")
		}
		op.Status = "completed"
		return save()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	runs, _ := store.BatchRuns(b.ID)
	if len(runs) != 2 || runs[0].State != "failed" || runs[1].State != "completed" {
		t.Fatal(runs)
	}
	retry, err := s.Resume(runs[0].ID, false)
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	r, _ := store.Run(retry.ID)
	if r.State != "completed" {
		t.Fatal(r)
	}
	undo, err := s.Resume(r.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	r, _ = store.Run(undo.ID)
	if r.State != "undone" {
		t.Fatal(r)
	}
	other, _ := store.Run(runs[1].ID)
	if other.State != "completed" {
		t.Fatal(other)
	}
}

func TestBatchCancelStopsCurrentAndPending(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, _ *model.Operation, _ func() error) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err = s.Execute("manual"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if err = s.CancelBatch(b.ID); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	runs, _ := store.BatchRuns(b.ID)
	for _, r := range runs {
		if r.State != "cancelled" {
			t.Fatal(r)
		}
	}
}

func TestQueueRemainsReservedBetweenWorks(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	checks := 0
	s, _, plans := executionFixture(t, nil)
	plans.check = func(ops []model.Work) error {
		checks++
		if checks == 3 {
			close(entered)
			<-release
		}
		return nil
	}
	if _, err := s.ExecuteBatch("batch"); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	// The first worker has released its current-run slot, but batch retains it.
	s.coordinator.mu.Lock()
	running, batch := s.coordinator.running, s.coordinator.batch
	s.coordinator.mu.Unlock()
	if running != "" || batch == "" {
		t.Fatalf("running=%q batch=%q", running, batch)
	}
	if _, err := s.Execute("manual"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	close(release)
	awaitIdle(t, s)
}

func TestCloseCancelsAndWaitsForWorkers(t *testing.T) {
	entered, observed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, _ *model.Operation, _ func() error) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		close(observed)
		<-release
		return ctx.Err()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	closed := make(chan struct{})
	go func() { s.coordinator.Close(); close(closed) }()
	await(t, observed)
	select {
	case <-closed:
		t.Fatal("closed before worker persisted interruption")
	default:
	}
	close(release)
	await(t, closed)
	runs, _ := store.BatchRuns(b.ID)
	for _, r := range runs {
		if r.State != "interrupted" {
			t.Fatal(r)
		}
	}
	if _, err = s.Execute("manual"); err == nil {
		t.Fatal("closed application accepted work")
	}
}

func TestFailedQueuePersistenceDoesNotReserveExecution(t *testing.T) {
	s, store, plans := executionFixture(t, nil)
	store.batchErr = errors.New("transaction failed")
	if _, err := s.ExecuteBatch("batch"); err == nil {
		t.Fatal("ignored persistence failure")
	}
	if s.coordinator.Busy() || plans.Get("batch") == nil {
		t.Fatal("failed queue consumed preview or reserved execution")
	}
	records, _ := store.Runs(1)
	if records["total"] != 0 {
		t.Fatal(records)
	}
}
