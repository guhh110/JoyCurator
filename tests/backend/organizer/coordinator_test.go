package organizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"joycurator/internal/infra/fsops"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
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
func (m *memoryRuns) Runs(int) (RunPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return RunPage{Total: len(m.items)}, nil
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
func (m *memoryRuns) BatchSummaries(id string) ([]Run, error) {
	runs, err := m.BatchRuns(id)
	for i := range runs {
		runs[i].Ops = nil
		runs[i].Mappings = nil
	}
	return runs, err
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
func (m *memoryRuns) RunsByPurpose(page int, purpose string) (RunPage, error) {
	return m.Runs(page)
}

type fixedSettings struct{ value model.Settings }

func (s fixedSettings) Load() (model.Settings, error) { return s.value, nil }

type allowedSources struct{ check func() }

func (s allowedSources) SourceAllowed(string, model.Settings) bool {
	if s.check != nil {
		s.check()
	}
	return true
}

// This harness seeds immutable previews while exercising real file validation.
type fakePlans struct {
	cache *PreviewCache
	check func([]Work) error
}

func (p *fakePlans) Get(id string) *Plan { return p.cache.get(id) }

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
	run func(context.Context, *Operation, func() error) error
}

func (r operationRunner) Run(ctx context.Context, op *Operation, save func() error) error {
	if r.run != nil {
		return r.run(ctx, op, save)
	}
	op.Status = "completed"
	return save()
}
func (r operationRunner) Undo(_ context.Context, op *Operation, save func() error) error {
	op.Status = "undone"
	return save()
}
func executionFixture(t *testing.T, run func(context.Context, *Operation, func() error) error) (*Service, *memoryRuns, *fakePlans) {
	t.Helper()
	store := &memoryRuns{items: map[string]Run{}}
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(source, "video.mkv")
	if err := os.WriteFile(sourcePath, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	access, err := fsops.New([]string{source, target})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewPreviewCache()
	plans := &fakePlans{cache: cache}
	for _, id := range []string{"manual", "batch"} {
		groups := []*Plan{}
		count := 1
		if id == "batch" {
			count = 2
		}
		for i := 0; i < count; i++ {
			op := Work{Operation: Operation{Kind: "video", Mode: "copy", Source: sourcePath, Target: filepath.Join(target, fmt.Sprint(i)+".mkv"), Status: "pending", Size: info.Size(), MTime: info.ModTime().UnixNano()}}
			// Mapping lifecycle is covered by integration tests; the fake runner writes no files.
			mapping := model.MediaMapping{Path: filepath.Join(target, "registered"), Source: sourcePath, Type: "movie", TMDBID: 1}
			groups = append(groups, &Plan{Title: []string{"first", "second"}[i], Ops: []Work{op}, Mappings: []model.MediaMapping{mapping}})
		}
		p := &Plan{ID: id, Purpose: "organize", Version: 1, Revision: 1, RecognitionID: "workspace", Expires: time.Now().Add(time.Hour), Groups: groups}
		for _, g := range groups {
			p.Ops = append(p.Ops, g.Ops...)
			p.Mappings = append(p.Mappings, g.Mappings...)
		}
		cache.put(p)
	}
	factory := matching.Factory(func(model.Settings, *trace.Recorder) (matching.Provider, error) { return noCatalog{}, nil })
	service := New(Dependencies{Coordinator: NewCoordinator(), Store: store, Settings: fixedSettings{model.Settings{Version: 1, SourceRoot: source, TargetRoot: target}}, Previews: cache, Access: access,
		Sources: allowedSources{check: func() {
			if plans.check != nil {
				_ = plans.check(nil)
			}
		}}, Factory: factory,
		Runner: func(func(context.Context, string) ([]byte, error)) Runner { return operationRunner{run} }, Revision: func(string) (int, error) { return 1, nil }, Finish: func(Run) {}})
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

func TestUnknownFileStateIsNotRetried(t *testing.T) {
	s, store, _ := executionFixture(t, nil)
	r := Run{ID: "uncertain", Purpose: "organize", State: "running", Ops: []Work{
		{Operation: Operation{Kind: "video", Status: "published", Target: "unknown"}, Owner: "first"},
		{Operation: Operation{Kind: "video", Status: "pending", Target: "safe"}, Owner: "second"},
	}}
	s.coordinator.wg.Add(1)
	s.work(context.Background(), func() {}, r)
	got, err := store.Run(r.ID)
	if err != nil || got.State != "failed" || got.Ops[0].Status != "manual_review" || got.Ops[1].Status != "completed" {
		t.Fatal("uncertain file was retried or safe file was blocked", got, err)
	}
}

func TestExecutionCompetitionDoesNotWriteExtraRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		once.Do(func() { close(entered) })
		select {
		case <-release:
			op.Status = "completed"
			return save()
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if _, err := s.ExecuteBatch("manual"); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err := s.ExecuteBatch("batch"); !errors.Is(err, ErrBusy) {
		t.Fatalf("queue bypassed task: %v", err)
	}
	records, _ := store.Runs(1)
	if records.Total != 1 {
		t.Fatal(records)
	}
	close(release)
	awaitIdle(t, s)
}

func TestBatchFailureContinuesAndIndependentRetryUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	calls := 0
	s, store, _ := executionFixture(t, func(_ context.Context, op *Operation, save func() error) error {
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	entered := make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, _ *Operation, _ func() error) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err = s.ExecuteBatch("manual"); !errors.Is(err, ErrBusy) {
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	checks := 0
	s, _, plans := executionFixture(t, nil)
	plans.check = func(ops []Work) error {
		checks++
		if checks == 2 {
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
	if _, err := s.ExecuteBatch("manual"); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	close(release)
	awaitIdle(t, s)
}

func TestCloseCancelsAndWaitsForWorkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	entered, observed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, _ *Operation, _ func() error) error {
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
	if _, err = s.ExecuteBatch("manual"); err == nil {
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
	if records.Total != 0 {
		t.Fatal(records)
	}
}
