package organizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/trace"
)

type memoryRuns struct {
	mu       sync.Mutex
	items    map[string]Run
	order    []string
	queue    []string
	mappings map[string]model.MediaMapping
	batchErr error
}

func snapshot[T any](v T) T {
	raw, _ := json.Marshal(v)
	var result T
	_ = json.Unmarshal(raw, &result)
	return result
}
func (m *memoryRuns) save(r Run) {
	if _, ok := m.items[r.ID]; !ok {
		m.order = append(m.order, r.ID)
	}
	m.items[r.ID] = snapshot(r)
}
func (m *memoryRuns) SaveRun(r Run) error { m.mu.Lock(); defer m.mu.Unlock(); m.save(r); return nil }
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
func (m *memoryRuns) RunsByPurpose(page int, purpose string, filter RunFilter) (RunPage, error) {
	return m.Runs(page)
}
func (m *memoryRuns) BatchRuns(id string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []Run{}
	for _, key := range m.order {
		if r := m.items[key]; r.BatchID == id {
			result = append(result, snapshot(r))
		}
	}
	return result, nil
}
func (m *memoryRuns) BatchSummaries(id string) ([]Run, error) {
	runs, err := m.BatchRuns(id)
	for i := range runs {
		runs[i] = summary(runs[i])
	}
	return runs, err
}
func (m *memoryRuns) Enqueue(runs []Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.batchErr != nil {
		return m.batchErr
	}
	seen := map[string]bool{}
	for _, id := range m.queue {
		seen[id] = true
	}
	for _, r := range runs {
		if seen[r.ID] {
			return errors.New("duplicate queue entry")
		}
		seen[r.ID] = true
	}
	for _, r := range runs {
		m.save(r)
		m.queue = append(m.queue, r.ID)
	}
	return nil
}
func (m *memoryRuns) QueuedRuns() ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Run{}
	for _, id := range m.queue {
		out = append(out, snapshot(m.items[id]))
	}
	return out, nil
}
func (m *memoryRuns) FinishQueued(r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.save(r)
	for i, id := range m.queue {
		if id == r.ID {
			m.queue = append(m.queue[:i], m.queue[i+1:]...)
			return nil
		}
	}
	return errors.New("missing queue entry")
}
func (m *memoryRuns) CancelQueued(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		r := m.items[id]
		r.State = "cancelled"
		m.save(r)
		for i, key := range m.queue {
			if key == id {
				m.queue = append(m.queue[:i], m.queue[i+1:]...)
				break
			}
		}
	}
	return nil
}
func (m *memoryRuns) QueuePage(purpose string, page int) (RunPage, error) {
	runs, _ := m.QueuedRuns()
	result := RunPage{Items: []Run{}, Page: page}
	for _, r := range runs {
		if r.Purpose == purpose && r.State == "queued" {
			result.Items = append(result.Items, summary(r))
			result.Total++
		}
	}
	return result, nil
}
func (m *memoryRuns) Batches(int) (BatchPage, error)    { return BatchPage{}, nil }
func (m *memoryRuns) BatchPosition(string) (int, error) { return 1, nil }
func (m *memoryRuns) FileExecutions(string) (map[string]matching.FileExecution, error) {
	return map[string]matching.FileExecution{}, nil
}
func (m *memoryRuns) SaveCheckpoint(r Run, ops, mappings []int, values []model.MediaMapping) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[r.ID]
	if !ok {
		return errors.New("missing run")
	}
	next := snapshot(r)
	next.Ops = snapshot(current.Ops)
	next.Mappings = snapshot(current.Mappings)
	for _, i := range ops {
		next.Ops[i] = snapshot(r.Ops[i])
	}
	for _, i := range mappings {
		next.Mappings[i] = snapshot(r.Mappings[i])
	}
	m.save(next)
	for _, v := range values {
		if old, ok := m.mappings[v.Path]; ok && old.Identity == v.Identity {
			v.State = old.State
		}
		m.mappings[v.Path] = v
	}
	return nil
}
func (m *memoryRuns) SaveMapping(v model.MediaMapping) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings[v.Path] = v
	return nil
}
func (m *memoryRuns) Mapping(path string) (*model.MediaMapping, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.mappings[path]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (m *memoryRuns) DeleteMapping(v model.MediaMapping) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mappings[v.Path].RunID == v.RunID {
		delete(m.mappings, v.Path)
	}
	return nil
}
func (m *memoryRuns) UpdateMappingState(v model.MediaMapping, state, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.mappings[v.Path]
	if ok && old.RunID == v.RunID {
		old.State = state
		m.mappings[v.Path] = old
	}
	return nil
}
func (m *memoryRuns) Shared(string, string, string) (bool, error) { return false, nil }

type fixedSettings struct{ value model.Settings }

func (s fixedSettings) Load() (model.Settings, error) { return s.value, nil }

type allowedSources struct{ check func() }

func (s allowedSources) SourceAllowed(string, model.Settings) bool {
	if s.check != nil {
		s.check()
	}
	return true
}

type fakePlans struct{ cache *PreviewCache }

func (p *fakePlans) Get(id string) *Plan { return p.cache.get(id) }

type noCatalog struct{ matching.Provider }

func (noCatalog) Image(context.Context, string) ([]byte, error) { return nil, nil }

type operationRunner struct {
	run func(context.Context, *Operation, func() error) error
}

func (r operationRunner) Run(ctx context.Context, op *Operation, save func() error) error {
	checkpoint := func() error {
		if op.Status == "completed" {
			if err := os.WriteFile(op.Target, []byte("video"), 0644); err != nil {
				return err
			}
			info, err := os.Stat(op.Target)
			if err != nil {
				return err
			}
			op.Identity = fsops.Identity(info)
		}
		return save()
	}
	if r.run != nil {
		return r.run(ctx, op, checkpoint)
	}
	op.Status = "completed"
	return checkpoint()
}
func (r operationRunner) Undo(_ context.Context, op *Operation, save func() error) error {
	if err := os.Remove(op.Target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	op.Status = "undone"
	return save()
}
func executionFixture(t *testing.T, run func(context.Context, *Operation, func() error) error) (*Service, *memoryRuns, *fakePlans) {
	t.Helper()
	root := t.TempDir()
	source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
	for _, dir := range []string{source, target} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	access, err := fsops.New([]string{source, target})
	if err != nil {
		t.Fatal(err)
	}
	cache := NewPreviewCache()
	workspace := matching.Workspace{Purpose: "organize", ID: "workspace", Revision: 1, State: "completed"}
	plans := &fakePlans{cache: cache}
	store := &memoryRuns{items: map[string]Run{}, mappings: map[string]model.MediaMapping{}}
	for _, id := range []string{"manual", "batch"} {
		p := &Plan{ID: id, Purpose: "organize", Version: 1, Revision: 1, RecognitionID: "workspace", Expires: time.Now().Add(time.Hour)}
		count := 1
		if id == "batch" {
			count = 2
		}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("%s-%d.mkv", id, i)
			src, dst := filepath.Join(source, name), filepath.Join(target, name)
			if err := os.WriteFile(src, []byte("video"), 0644); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(src)
			op := Work{Operation: Operation{Kind: "video", Source: src, Target: dst, Mode: "copy", Status: "pending", Size: info.Size(), MTime: info.ModTime().UnixNano()}, Owner: src}
			mapping := model.MediaMapping{Path: dst, Source: src, Type: "movie", TMDBID: 1, State: "pending"}
			item := matching.Item{Identity: fsops.Identity(info), File: model.File{Path: src, Size: info.Size(), MTime: info.ModTime().UnixNano()}, Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Selected: true}
			item.Confirm("user", matching.EpisodeBasis{Kind: "manual"})
			workspace.Items = append(workspace.Items, item)
			group := &Plan{Title: fmt.Sprintf("%s-%d", id, i), Ops: []Work{op}, Mappings: []model.MediaMapping{mapping}}
			p.Groups = append(p.Groups, group)
			p.Ops = append(p.Ops, op)
			p.Mappings = append(p.Mappings, mapping)
		}
		cache.put(p)
	}
	service := New(Dependencies{Coordinator: NewCoordinator(), Store: store, Settings: fixedSettings{model.Settings{Version: 1, SourceRoot: source, TargetRoot: target}}, Previews: cache, Access: access, Workspaces: confirmedWorkspaces{workspace}, Mappings: store, Sources: allowedSources{}, Factory: func(model.Settings, *trace.Recorder) (matching.Provider, error) { return noCatalog{}, nil }, Runner: func(func(context.Context, string) ([]byte, error)) Runner { return operationRunner{run} }, Revision: func(string) (int, error) { return 1, nil }})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
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
	t.Fatal("execution remained busy", s.Queue())
}

func TestMultipleBatchesRunFIFO(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	order := []string{}
	var mu sync.Mutex
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		mu.Lock()
		order = append(order, filepath.Base(op.Source))
		mu.Unlock()
		once.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		op.Status = "completed"
		return save()
	})
	first, err := s.ExecuteBatch("manual")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	second, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	if s.Queue().Organize.Queued != 2 {
		t.Fatal(s.Queue())
	}
	close(release)
	awaitIdle(t, s)
	if fmt.Sprint(order) != "[manual-0.mkv batch-0.mkv batch-1.mkv]" {
		t.Fatal(order)
	}
	for _, id := range []string{first.ID, second.ID} {
		runs, _ := store.BatchRuns(id)
		for _, r := range runs {
			if r.State != "completed" {
				t.Fatal(r)
			}
		}
	}
}
func TestBatchFailureContinuesAndIndependentRetryUndo(t *testing.T) {
	calls := 0
	s, store, _ := executionFixture(t, func(_ context.Context, op *Operation, save func() error) error {
		calls++
		if calls == 1 {
			return errors.New("first failed")
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
	if runs[0].State != "failed" || runs[1].State != "completed" {
		t.Fatal(runs)
	}
	r, err := s.Resume(runs[0].ID, false)
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	r, _ = store.Run(r.ID)
	if r.State != "completed" {
		t.Fatal(r)
	}
	if _, err = s.Resume(r.ID, true); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	r, _ = store.Run(r.ID)
	if r.State != "undone" {
		t.Fatal(r)
	}
}
func TestQueuedBatchCancellationLeavesOtherBatchRunning(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	})
	active, err := s.ExecuteBatch("manual")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	waiting, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelBatch(waiting.ID); err != nil {
		t.Fatal(err)
	}
	runs, _ := store.BatchRuns(waiting.ID)
	for _, r := range runs {
		if r.State != "cancelled" {
			t.Fatal(r)
		}
	}
	if s.Queue().Organize.Current == nil {
		t.Fatal("cancelled unrelated active task")
	}
	if err = s.CancelBatch(active.ID); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
}
func TestCloseKeepsPendingTasksForRestart(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ctx.Err()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	s.coordinator.Close()
	runs, _ := store.BatchRuns(b.ID)
	if runs[0].State != "interrupted" || runs[1].State != "queued" {
		t.Fatal(runs)
	}
	if _, err = s.ExecuteBatch("manual"); err == nil {
		t.Fatal("closed service accepted task")
	}
}
func TestFailedQueuePersistenceDoesNotReserveExecution(t *testing.T) {
	s, store, plans := executionFixture(t, nil)
	store.batchErr = errors.New("full")
	if _, err := s.ExecuteBatch("batch"); err == nil {
		t.Fatal("ignored write failure")
	}
	if s.coordinator.Busy() || plans.Get("batch") == nil {
		t.Fatal("consumed failed submission")
	}
}

func (m *memoryRuns) ScrapeWorks(string, string) (ScrapeWorkPage, error) {
	return ScrapeWorkPage{}, nil
}
func (m *memoryRuns) ScrapeWorkMappings(string, int) ([]model.MediaMapping, error) { return nil, nil }

func (m *memoryRuns) ScrapeArtifact(string) (*Work, error) { return nil, nil }

type confirmedWorkspaces struct{ value matching.Workspace }

func (w confirmedWorkspaces) Read(string) (matching.Workspace, error) { return w.value, nil }
