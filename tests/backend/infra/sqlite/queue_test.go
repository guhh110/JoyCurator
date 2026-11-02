package sqlite

import (
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func TestEnqueueRollbackAndRetryOrder(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "first", Purpose: "organize", State: "queued"}
	if err := s.Enqueue([]organizer.Run{r}); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue([]organizer.Run{{ID: "second", Purpose: "organize", State: "queued"}, r}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := s.Run("second"); err == nil {
		t.Fatal("partial enqueue survived")
	}
	r.State = "failed"
	if err := s.FinishQueued(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Enqueue([]organizer.Run{{ID: "second", Purpose: "organize", State: "queued"}}); err != nil {
		t.Fatal(err)
	}
	r.State = "queued"
	if err := s.Enqueue([]organizer.Run{r}); err != nil {
		t.Fatal(err)
	}
	queued, err := s.QueuedRuns()
	if err != nil || len(queued) != 2 || queued[0].ID != "second" || queued[1].ID != "first" {
		t.Fatal(queued, err)
	}
}
func TestFinishQueueRollbackKeepsTicket(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "one", Purpose: "organize", State: "queued"}
	if err := s.Enqueue([]organizer.Run{r}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_run BEFORE UPDATE ON runs BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	r.State = "completed"
	if err := s.FinishQueued(r); err == nil {
		t.Fatal("failed transaction accepted")
	}
	queued, err := s.QueuedRuns()
	if err != nil || len(queued) != 1 || queued[0].State != "queued" {
		t.Fatal(queued, err)
	}
}
func TestVideoCheckpointAndMappingAreAtomic(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "one", Purpose: "organize", State: "running", Ops: []organizer.Work{{Operation: organizer.Operation{Kind: "video", Status: "published"}}}}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_mapping BEFORE INSERT ON media_mappings BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	r.Ops[0].Status = "completed"
	m := model.MediaMapping{Path: "/target/video.mkv", RunID: r.ID, Identity: "file", State: "pending"}
	if err := s.SaveCheckpoint(r, []int{0}, nil, []model.MediaMapping{m}); err == nil {
		t.Fatal("partial commit accepted")
	}
	stored, _ := s.Run(r.ID)
	value, _ := s.Mapping(m.Path)
	if stored.Ops[0].Status != "published" || value != nil {
		t.Fatal(stored, value)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_mapping"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(r, []int{0}, nil, []model.MediaMapping{m}); err != nil {
		t.Fatal(err)
	}
	stored, _ = s.Run(r.ID)
	value, _ = s.Mapping(m.Path)
	if stored.Ops[0].Status != "completed" || value == nil {
		t.Fatal(stored, value)
	}
}
func TestRestartPreservesQueuedOrderAndInterruptsOnlyActiveRuns(t *testing.T) {
	dir := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot = filepath.Join(dir, "source")
	settings.TargetRoot = filepath.Join(dir, "target")
	s, err := Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	runs := []organizer.Run{{ID: "active", Purpose: "organize", State: "queued"}, {ID: "waiting", Purpose: "organize", State: "queued"}, {ID: "scrape", Purpose: "scrape", State: "queued"}}
	if err = s.Enqueue(runs); err != nil {
		t.Fatal(err)
	}
	runs[0].State = "running"
	if err = s.SaveRun(runs[0]); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	active, _ := s.Run("active")
	queued, err := s.QueuedRuns()
	if err != nil || active.State != "interrupted" || len(queued) != 2 || queued[0].ID != "waiting" || queued[1].ID != "scrape" {
		t.Fatal(active, queued, err)
	}
}
func TestOrphanQueueIsReportedWithoutRebuildingRecords(t *testing.T) {
	dir := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot = filepath.Join(dir, "source")
	settings.TargetRoot = filepath.Join(dir, "target")
	s, err := Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRun(organizer.Run{ID: "orphan", Purpose: "organize", State: "queued"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if next, err := Open(dir, settings); err == nil {
		next.Close()
		t.Fatal("inconsistent queue accepted")
	} else if !strings.Contains(err.Error(), "不一致") {
		t.Fatal(err)
	}
}

func TestRecoveryRejectsWrongRunningTicketWithoutRewritingRun(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "active", Purpose: "organize", State: "queued"}
	if err := s.Enqueue([]organizer.Run{r}); err != nil {
		t.Fatal(err)
	}
	r.State = "running"
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE execution_queue SET purpose='scrape'"); err != nil {
		t.Fatal(err)
	}
	if err := s.recoverQueue(); err == nil {
		t.Fatal("inconsistent active ticket accepted")
	}
	current, err := s.Run(r.ID)
	if err != nil || current.State != "running" {
		t.Fatal("recovery rewrote inconsistent data", current, err)
	}
}
func TestFileExecutionProjectionPreservesCompletedAndUnknownFiles(t *testing.T) {
	s := openStore(t)
	first := model.MediaMapping{Path: "/target/a.mkv", Source: "/source/a.mkv", RunID: "run", Identity: "video"}
	second := model.MediaMapping{Path: "/target/b.mkv", Source: "/source/b.mkv"}
	r := organizer.Run{ID: "run", Purpose: "organize", State: "running", RecognitionID: "workspace", Mappings: []model.MediaMapping{first, second}, Ops: []organizer.Work{
		{Operation: organizer.Operation{Kind: "video", Target: first.Path, Status: "completed"}},
		{Operation: organizer.Operation{Kind: "video", Target: second.Path, Status: "manual_review"}},
	}}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	own, err := s.FileExecutions("workspace")
	if err != nil || !own[first.Source].Completed || !own[first.Source].Locked || !own[second.Source].ManualReview {
		t.Fatal(own, err)
	}
	r.State = "cancelled"
	if err = s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	own, err = s.FileExecutions("workspace")
	if err != nil || !own[first.Source].Completed || own[first.Source].Locked || !own[second.Source].Locked {
		t.Fatal(own, err)
	}
	r.Undo = true
	if err = s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	own, err = s.FileExecutions("workspace")
	if err != nil || !own[first.Source].Completed {
		t.Fatal("cancelled undo made an untouched completed video editable", own, err)
	}
	other, err := s.FileExecutions("another")
	if err != nil || other[first.Source].Completed || !other[second.Source].Locked {
		t.Fatal(other, err)
	}
	r.ID = "skip"
	r.Mappings = []model.MediaMapping{{Path: "/target/skip.mkv", Source: "/source/skip.mkv", RunID: "skip", Identity: "existing"}}
	r.Ops = nil
	r.State = "completed"
	if err = s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	own, err = s.FileExecutions("workspace")
	if err != nil || !own["/source/skip.mkv"].Completed {
		t.Fatal("skipped video remains selectable", own, err)
	}
}
