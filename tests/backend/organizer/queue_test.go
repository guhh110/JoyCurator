package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"joycurator/internal/model"
)

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}
func waitRelease(ctx context.Context, release <-chan struct{}) error {
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func scrapeRun(video model.MediaMapping) Run {
	original := video
	video.Original = &original
	return Run{ID: model.Key(), Purpose: "scrape", Mode: "scrape", Title: "scrape", State: "queued", Version: 1, Mappings: []model.MediaMapping{video}, Ops: []Work{{Operation: Operation{Kind: "nfo", Mode: "copy", Target: video.Path + ".nfo", Content: []byte("nfo"), Status: "pending"}, Owner: video.Path}}, Total: 1}
}
func enqueueTest(t *testing.T, s *Service, r Run) {
	t.Helper()
	s.coordinator.Lock()
	err := s.enqueueLocked([]Run{r})
	s.coordinator.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestScrapeConsumesFirstVideoWhileSameWorkStillOrganizes(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "cancel remaining"}[cancel], func(t *testing.T) {
			secondVideo, release, scraping := make(chan struct{}), make(chan struct{}), make(chan struct{})
			s, store, plans := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
				if op.Kind == "video" && strings.Contains(op.Source, "batch-1") {
					close(secondVideo)
					if err := waitRelease(ctx, release); err != nil {
						return err
					}
				}
				if op.Kind == "nfo" {
					close(scraping)
				}
				op.Status = "completed"
				return save()
			})
			p := plans.Get("batch")
			p.Groups[0].Ops = append(p.Groups[0].Ops, p.Groups[1].Ops...)
			p.Groups[0].Mappings = append(p.Groups[0].Mappings, p.Groups[1].Mappings...)
			p.Groups = p.Groups[:1]
			b, err := s.ExecuteBatch("batch")
			if err != nil {
				t.Fatal(err)
			}
			await(t, secondVideo)
			video, err := store.Mapping(p.Mappings[0].Path)
			if err != nil || video == nil || video.RunID != b.Runs[0].ID {
				t.Fatal("first video not registered before next copy", video, err)
			}
			enqueueTest(t, s, scrapeRun(*video))
			await(t, scraping)
			eventually(t, func() bool { return s.Queue().Scrape.Current == nil && s.Queue().Scrape.Queued == 0 })
			if s.Queue().Organize.Current == nil {
				t.Fatal("organize task did not overlap scrape")
			}
			video, _ = store.Mapping(video.Path)
			if video.State != "completed" {
				t.Fatal(video)
			}
			if cancel {
				if err := s.Cancel(b.Runs[0].ID); err != nil {
					t.Fatal(err)
				}
			} else {
				close(release)
			}
			awaitIdle(t, s)
			video, _ = store.Mapping(video.Path)
			if video.State != "completed" {
				t.Fatal("organizer reset scrape result", video)
			}
		})
	}
}

func TestUndoWaitsForScrapeReadClaimWithoutHoldingPartialClaims(t *testing.T) {
	scraping, release := make(chan struct{}), make(chan struct{})
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		if op.Kind == "nfo" {
			close(scraping)
			if err := waitRelease(ctx, release); err != nil {
				return err
			}
		}
		op.Status = "completed"
		return save()
	})
	b, err := s.ExecuteBatch("manual")
	if err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	run, _ := store.Run(b.Runs[0].ID)
	video, _ := store.Mapping(run.Mappings[0].Path)
	enqueueTest(t, s, scrapeRun(*video))
	await(t, scraping)
	if _, err = s.Resume(run.ID, true); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return s.Queue().Organize.WaitReason != "" })
	s.coordinator.Lock()
	claims := len(s.coordinator.lanes["organize"].claims)
	s.coordinator.Unlock()
	if claims != 0 {
		t.Fatal("waiting task held claims")
	}
	close(release)
	awaitIdle(t, s)
	run, _ = store.Run(run.ID)
	if run.State != "undone" {
		t.Fatal(run)
	}
}
func TestRetryAppendsBehindAlreadyWaitingBatches(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	failed := false
	order := []string{}
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		name := filepath.Base(op.Source)
		order = append(order, name)
		if name == "batch-0.mkv" && !failed {
			failed = true
			return errors.New("retry me")
		}
		if name == "batch-1.mkv" {
			close(entered)
			if err := waitRelease(ctx, release); err != nil {
				return err
			}
		}
		op.Status = "completed"
		return save()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err = s.ExecuteBatch("manual"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Resume(b.Runs[0].ID, false); err != nil {
		t.Fatal(err)
	}
	close(release)
	awaitIdle(t, s)
	if strings.Join(order, ",") != "batch-0.mkv,batch-1.mkv,manual-0.mkv,batch-0.mkv" {
		t.Fatal(order)
	}
	r, _ := store.Run(b.Runs[0].ID)
	if r.State != "completed" {
		t.Fatal(r)
	}
}
func TestDuplicateSourceIsRejectedWithoutConsumingPreview(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, _, plans := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		close(entered)
		if err := waitRelease(ctx, release); err != nil {
			return err
		}
		op.Status = "completed"
		return save()
	})
	p := plans.Get("manual")
	copy := *p
	copy.ID = "duplicate"
	s.previews.put(&copy)
	if _, err := s.ExecuteBatch("manual"); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err := s.ExecuteBatch("duplicate"); err == nil {
		t.Fatal("duplicate source admitted")
	}
	if s.previews.get("duplicate") == nil {
		t.Fatal("rejected plan consumed")
	}
	close(release)
	awaitIdle(t, s)
}
func TestQueuedTasksResumeAfterWorkerRestart(t *testing.T) {
	entered := make(chan struct{})
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	b, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	s.coordinator.Close()
	next := New(Dependencies{Coordinator: NewCoordinator(), Store: store, Settings: s.settings, Previews: NewPreviewCache(), Access: s.access, Mappings: store, Sources: s.sources, Factory: s.factory, Runner: func(func(context.Context, string) ([]byte, error)) Runner { return operationRunner{} }, Revision: s.revision})
	t.Cleanup(next.coordinator.Close)
	if err := next.Start(); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, next)
	runs, _ := store.BatchRuns(b.ID)
	if runs[0].State != "interrupted" || runs[1].State != "completed" {
		t.Fatal(runs)
	}
}

func TestPendingSubtitleTargetsAreReservedAcrossBatches(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, _, plans := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		if strings.Contains(op.Source, "manual-0") {
			close(entered)
			if err := waitRelease(ctx, release); err != nil {
				return err
			}
		}
		op.Status = "completed"
		return save()
	})
	manual, other := plans.Get("manual"), plans.Get("batch")
	subtitle := Work{Operation: Operation{Kind: "subtitle", Target: other.Ops[0].Target, Status: "pending", Mode: "copy"}}
	manual.Ops = append(manual.Ops, subtitle)
	manual.Groups[0].Ops = append(manual.Groups[0].Ops, subtitle)
	if _, err := s.ExecuteBatch("manual"); err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if _, err := s.ExecuteBatch("batch"); err == nil {
		t.Fatal("reserved non-video target admitted")
	}
	close(release)
	awaitIdle(t, s)
}

func TestSingleQueuedAndRunningCancelAdvanceOtherTasks(t *testing.T) {
	entered := make(chan struct{})
	s, store, _ := executionFixture(t, func(ctx context.Context, op *Operation, save func() error) error {
		if strings.Contains(op.Source, "manual") {
			close(entered)
			<-ctx.Done()
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
	batch, err := s.ExecuteBatch("batch")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(batch.Runs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(first.Runs[0].ID); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	active, _ := store.Run(first.Runs[0].ID)
	runs, _ := store.BatchRuns(batch.ID)
	if active.State != "cancelled" || runs[0].State != "cancelled" || runs[1].State != "completed" {
		t.Fatal(active, runs)
	}
}

func TestQueuedMoveUndoChecksAggregateSourceSpaceBeforeStarting(t *testing.T) {
	s, _, plans := executionFixture(t, nil)
	p := plans.Get("batch")
	available, _, err := s.access.CheckTarget(p.Ops[0].Source)
	if err != nil {
		t.Fatal(err)
	}
	s.access.DeviceOf = func(os.FileInfo) uint64 { return ^uint64(0) }
	ops := append([]Work(nil), p.Ops...)
	for i := range ops {
		file, err := os.Create(ops[i].Target)
		if err != nil {
			t.Fatal(err)
		}
		err = file.Truncate(available * 3 / 5)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		ops[i].Status = "completed"
		ops[i].Mode = "move"
	}
	if err = s.validateQueued(context.Background(), Run{Purpose: "organize", Version: 1, Undo: true, Ops: ops}); err == nil || !strings.Contains(err.Error(), "空间不足") {
		t.Fatal(err)
	}
}
