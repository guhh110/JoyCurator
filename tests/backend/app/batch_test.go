package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/planning"
)

func batchFixture(t *testing.T) (*App, string, matching.Workspace, *planning.Plan) {
	t.Helper()
	a, root := fixture(t)
	movie := mediaFile(t, a, root, "film.mkv")
	tv := mediaFile(t, a, root, "Show.S01E01.mkv")
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{filepath.Join(root, "source")}, Items: []matching.Item{
		{File: movie, Group: "movie", Entry: &model.Entry{Type: "movie", TMDBID: 1}, State: "ready", Selected: true},
		{File: tv, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "ready", Selected: true},
	}}
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{DuplicatePolicy: "error", RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
	if err != nil || len(plan.Issues) > 0 {
		t.Fatal(plan, err)
	}
	return a, root, task, plan
}
func waitBatch(t *testing.T, a *App, id string) organizer.Batch {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		batch, err := a.dependencies.Organizer.ReadBatch(id)
		if err != nil {
			t.Fatal(err)
		}
		if !batch.Active {
			return batch
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("batch timed out")
	return organizer.Batch{}
}
func TestBatchExecuteSeparateRecordsAndUndo(t *testing.T) {
	a, _, task, plan := batchFixture(t)
	if len(plan.Groups) != 2 || len(plan.Rows) != 2 {
		t.Fatal(plan)
	}
	if _, err := a.dependencies.Organizer.Execute(plan.ID); err == nil {
		t.Fatal("batch preview accepted by manual execute")
	}
	batch, err := a.dependencies.Organizer.ExecuteBatch(plan.ID)
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
	task, err = a.dependencies.Workspace.Read(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range task.Items {
		if !item.Removed || item.Selected {
			t.Fatal("completed file remains selected", item)
		}
	}
	first := batch.Runs[0]
	undo, err := a.dependencies.Organizer.Resume(first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, undo).State != "undone" {
		t.Fatal("undo failed")
	}
	second, err := a.dependencies.Organizer.Record(batch.Runs[1].ID)
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
		_ = a.store.SaveWorkspace(&task)
		if _, err := a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
			t.Fatal("stale workspace executed")
		}
	})
	t.Run("source changed", func(t *testing.T) {
		a, _, task, plan := batchFixture(t)
		_ = os.WriteFile(task.Items[0].Path, []byte("changed"), 0644)
		if _, err := a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
			t.Fatal("changed source executed")
		}
	})
	t.Run("global collision", func(t *testing.T) {
		a, _, task, _ := batchFixture(t)
		s, _ := a.dependencies.Config.Load()
		s.NFO = false
		s.Rules.MovieFolder = "collision"
		s.Rules.MovieFile = "video.{ext}"
		s.Rules.TVFolder = "collision"
		s.Rules.TVFile = "video.{ext}"
		s.Version++
		_, _ = testDB(t, a).Exec("UPDATE config SET value=?", jsonText(s))
		plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{DuplicatePolicy: "error", RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(plan.Issues, " "), "作品之间目标重复") {
			t.Fatal(plan.Issues)
		}
		if _, err = a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
			t.Fatal("conflicting batch executed")
		}
	})
	t.Run("aggregate disk space", func(t *testing.T) {
		a, _, task, _ := batchFixture(t)
		available, _, err := testAccess(t, a).CheckTarget(filepath.Join(testAccess(t, a).Roots[1], "space.mkv"))
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
		if err := a.store.SaveWorkspace(&task); err != nil {
			t.Fatal(err)
		}
		plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{DuplicatePolicy: "error", RecognitionID: task.ID, Revision: task.Revision, Mode: "copy"})
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
func prepareQueuedBatch(t *testing.T, a *App, plan *planning.Plan) organizer.Batch {
	t.Helper()
	batch := organizer.Batch{ID: model.Key(), Runs: []organizer.Run{}, Active: true}
	for _, group := range plan.Groups {
		run := organizer.Run{ID: model.Key(), Title: group.Title, Mode: plan.Mode, State: "queued", Ops: group.Ops, Total: len(group.Ops), BatchID: batch.ID, Version: plan.Version}
		if err := a.store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
		batch.Runs = append(batch.Runs, run)
	}
	return batch
}
func TestRestartInterruptsQueuedRunsAndRecognition(t *testing.T) {
	a, root, _, plan := batchFixture(t)
	batch := prepareQueuedBatch(t, a, plan)
	task := matching.Workspace{ID: model.Key(), State: "running", Items: []matching.Item{}}
	_ = a.store.SaveWorkspace(&task)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := openTest(t, filepath.Join(filepath.Dir(root), "data"), filepath.Join(root, "source"), filepath.Join(root, "target"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	batch, err = restarted.dependencies.Organizer.ReadBatch(batch.ID)
	if err != nil || batch.Active {
		t.Fatal(batch, err)
	}
	for _, run := range batch.Runs {
		if run.State != "interrupted" {
			t.Fatal(run)
		}
	}
	task, err = restarted.dependencies.Workspace.Read(task.ID)
	if err != nil || task.State != "interrupted" {
		t.Fatal(task, err)
	}
}
