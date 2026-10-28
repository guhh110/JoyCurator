package app

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"joycurator/internal/model"
)

func batchFixture(t *testing.T) (*App, string, Recognition, *Plan) {
	t.Helper()
	a, root := fixture(t)
	movie := mediaFile(t, a, root, "film.mkv")
	tv := mediaFile(t, a, root, "Show.S01E01.mkv")
	task := Recognition{ID: key(), State: "completed", Paths: []string{filepath.Join(root, "source")}, Items: []RecognitionItem{
		{File: movie, Group: "movie", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Selected: true},
		{File: tv, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "ready", Selected: true},
	}}
	if err := a.saveRecognition(&task); err != nil {
		t.Fatal(err)
	}
	plan, err := a.batchPreview(context.Background(), BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
	if err != nil || len(plan.Issues) > 0 {
		t.Fatal(plan, err)
	}
	return a, root, task, plan
}
func waitBatch(t *testing.T, a *App, id string) Batch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		batch, err := a.readBatch(id)
		if err != nil {
			t.Fatal(err)
		}
		if !batch.Active {
			return batch
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("batch timed out")
	return Batch{}
}
func TestBatchExecuteSeparateRecordsAndUndo(t *testing.T) {
	a, _, task, plan := batchFixture(t)
	if len(plan.Groups) != 2 || len(plan.Rows) != 2 {
		t.Fatal(plan)
	}
	if _, err := a.execute(plan.ID); err == nil {
		t.Fatal("batch preview accepted by manual execute")
	}
	batch, err := a.executeBatch(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	batch = waitBatch(t, a, batch.ID)
	if len(batch.Runs) != 2 {
		t.Fatal(batch)
	}
	for _, run := range batch.Runs {
		if run.State != "completed" || run.BatchID != batch.ID || run.Completed != run.Total {
			t.Fatal(run)
		}
	}
	task, err = a.recognition(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range task.Items {
		if !item.Removed || item.Selected {
			t.Fatal("completed file remains selected", item)
		}
	}
	first := batch.Runs[0]
	undo, err := a.resume(first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, undo).State != "undone" {
		t.Fatal("undo failed")
	}
	second, err := a.record(batch.Runs[1].ID)
	if err != nil || second.State != "completed" {
		t.Fatal("other work affected", err)
	}
	for _, op := range second.Ops {
		if _, err := os.Stat(op.Target); err != nil {
			t.Fatal(err)
		}
	}
}
func TestBatchRejectsStaleEditsAndGlobalConflicts(t *testing.T) {
	t.Run("revision", func(t *testing.T) {
		a, _, task, plan := batchFixture(t)
		_ = a.saveRecognition(&task)
		if _, err := a.executeBatch(plan.ID); err == nil {
			t.Fatal("stale workspace executed")
		}
	})
	t.Run("source changed", func(t *testing.T) {
		a, _, task, plan := batchFixture(t)
		_ = os.WriteFile(task.Items[0].Path, []byte("changed"), 0644)
		if _, err := a.executeBatch(plan.ID); err == nil {
			t.Fatal("changed source executed")
		}
	})
	t.Run("global collision", func(t *testing.T) {
		a, _, task, _ := batchFixture(t)
		s, _ := a.settings()
		s.NFO = false
		s.Rules.MovieFolder = "collision"
		s.Rules.MovieFile = "video.{ext}"
		s.Rules.TVFolder = "collision"
		s.Rules.TVFile = "video.{ext}"
		s.Version++
		_, _ = a.db.Exec("UPDATE config SET value=?", jsonText(s))
		plan, err := a.batchPreview(context.Background(), BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(plan.Issues, " "), "作品之间目标重复") {
			t.Fatal(plan.Issues)
		}
		if _, err = a.executeBatch(plan.ID); err == nil {
			t.Fatal("conflicting batch executed")
		}
	})
	t.Run("aggregate disk space", func(t *testing.T) {
		a, _, task, _ := batchFixture(t)
		available, _, err := a.access.CheckTarget(filepath.Join(a.access.Roots[1], "space.mkv"))
		if err != nil {
			t.Fatal(err)
		}
		// Sparse files exercise real stat sizes and real free-space accounting
		// without reserving their logical size on disk. Each work fits alone.
		for i := range task.Items {
			file, err := os.OpenFile(task.Items[i].Path, os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				t.Fatal(err)
			}
			err = file.Truncate(available * 3 / 5)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(task.Items[i].Path)
			if err != nil {
				t.Fatal(err)
			}
			task.Items[i].Size, task.Items[i].MTime = info.Size(), info.ModTime().UnixNano()
		}
		if err := a.saveRecognition(&task); err != nil {
			t.Fatal(err)
		}
		plan, err := a.batchPreview(context.Background(), BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
		if err != nil {
			t.Fatal(err)
		}
		for _, group := range plan.Groups {
			if len(group.Issues) > 0 {
				t.Fatal("individual work should fit", group.Issues)
			}
		}
		if !strings.Contains(strings.Join(plan.Issues, " "), "空间不足") {
			t.Fatal("aggregate space not checked", plan.Issues)
		}
	})
}
func prepareQueuedBatch(t *testing.T, a *App, plan *Plan) Batch {
	t.Helper()
	batch := Batch{ID: key(), Runs: []Run{}, Active: true}
	for _, group := range plan.Groups {
		run := Run{ID: key(), Title: group.Title, Mode: plan.Mode, State: "queued", Ops: group.Ops, Total: len(group.Ops), BatchID: batch.ID, Version: plan.Version}
		if err := a.save(run); err != nil {
			t.Fatal(err)
		}
		batch.Runs = append(batch.Runs, run)
	}
	return batch
}
func TestBatchFailureContinuesAndCanRetry(t *testing.T) {
	a, _, _, plan := batchFixture(t)
	batch := prepareQueuedBatch(t, a, plan)
	source := batch.Runs[0].Ops[0].Source
	target := source + ".saved"
	if err := os.Rename(source, target); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.batch, a.batchCancel = batch.ID, cancel
	a.wg.Add(1)
	a.mu.Unlock()
	go a.runBatch(ctx, cancel, batch)
	batch = waitBatch(t, a, batch.ID)
	if batch.Runs[0].State != "failed" || batch.Runs[1].State != "completed" {
		t.Fatal(batch)
	}
	if err := os.Rename(target, source); err != nil {
		t.Fatal(err)
	}
	retry, err := a.resume(batch.Runs[0].ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, retry).State != "completed" {
		t.Fatal("retry failed")
	}
}
func TestBatchCancellationAndExecutionReservation(t *testing.T) {
	a, _, _, plan := batchFixture(t)
	batch := prepareQueuedBatch(t, a, plan)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.mu.Lock()
	a.batch, a.batchCancel = batch.ID, cancel
	a.mu.Unlock()
	if _, err := a.launch(Run{ID: key()}); err == nil {
		t.Fatal("manual task bypassed reserved queue")
	}
	s, _ := a.settings()
	if err := a.updateSettings(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/settings", strings.NewReader(jsonText(s)))); err == nil {
		t.Fatal("settings bypassed queue reservation")
	}
	a.wg.Add(1)
	go a.runBatch(ctx, cancel, batch)
	batch = waitBatch(t, a, batch.ID)
	for _, run := range batch.Runs {
		if run.State != "cancelled" {
			t.Fatal(run)
		}
	}
}
func TestRestartInterruptsQueuedRunsAndRecognition(t *testing.T) {
	a, root, _, plan := batchFixture(t)
	batch := prepareQueuedBatch(t, a, plan)
	task := Recognition{ID: key(), State: "running", Items: []RecognitionItem{}}
	_ = a.saveRecognition(&task)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(filepath.Join(filepath.Dir(root), "data"), filepath.Join(root, "source"), filepath.Join(root, "target"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	batch, err = restarted.readBatch(batch.ID)
	if err != nil || batch.Active {
		t.Fatal(batch, err)
	}
	for _, run := range batch.Runs {
		if run.State != "interrupted" {
			t.Fatal(run)
		}
	}
	task, err = restarted.recognition(task.ID)
	if err != nil || task.State != "interrupted" {
		t.Fatal(task, err)
	}
}

func TestShutdownInterruptsPendingBatch(t *testing.T) {
	a, _, _, plan := batchFixture(t)
	batch := prepareQueuedBatch(t, a, plan)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.mu.Lock()
	a.closed = true
	a.batch, a.batchCancel = batch.ID, cancel
	a.mu.Unlock()
	a.wg.Add(1)
	a.runBatch(ctx, cancel, batch)
	restored, err := a.readBatch(batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range restored.Runs {
		if run.State != "interrupted" {
			t.Fatal(run)
		}
	}
}
