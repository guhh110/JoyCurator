package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"testing"
)

func sampleSnapshot(t testing.TB) recognition.Snapshot {
	t.Helper()
	paths := []string{"/source/Show.S01E01.mkv", "/source/Show.S01E02.mkv"}
	s, err := recognition.DefaultEngine().Snapshot(context.Background(), recognition.BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: paths, Targets: paths})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sameSnapshot(t testing.TB, a, b recognition.Snapshot) bool {
	t.Helper()
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	if !bytes.Equal(left, right) {
		for i := 0; i < min(len(left), len(right)); i++ {
			if left[i] != right[i] {
				t.Logf("first difference at %d: %s / %s", i, left[max(0, i-80):min(len(left), i+100)], right[max(0, i-80):min(len(right), i+100)])
				break
			}
		}
		return false
	}
	return true
}

func TestWorkspaceReferencesOnlyCommittedSnapshotsAndProtectsConfirmationEvidence(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"old", "new", "pending"} {
		trace := recognition.Trace{ID: id, Status: "running"}
		if err := s.StartTrace(trace); err != nil {
			t.Fatal(err)
		}
		if id != "pending" {
			trace.Status = "completed"
			if err := s.FinishTrace(trace, sampleSnapshot(t)); err != nil {
				t.Fatal(err)
			}
		}
	}
	w := matching.Workspace{Purpose: "organize", ID: "references", State: "running", Items: []matching.Item{{TraceID: "pending"}}}
	if err := s.SaveWorkspace(&w); err == nil {
		t.Fatal("workspace referenced an uncommitted snapshot")
	}
	w.Items[0].TraceID = "new"
	w.Items[0].Confirmation = &matching.Confirmation{Basis: matching.EpisodeBasis{Kind: "suggestion", TraceID: "old"}}
	if err := s.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"old", "new"} {
		if err := s.DeleteTrace(id); err == nil {
			t.Fatal("referenced evidence deleted", id)
		}
	}
	revision := w.Revision
	if err := s.RecognitionProgress(w.ID, 1, 10); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Workspace(w.ID)
	if err != nil || stored.Revision != revision || stored.Completed != 1 || stored.Total != 10 {
		t.Fatal("progress advanced edit revision", stored, err)
	}
	w.Items = nil
	if err = s.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteTrace("old"); err != nil {
		t.Fatal(err)
	}
}
func TestVideoCheckpointRollsBackConfirmedWorkSnapshotWithMapping(t *testing.T) {
	s := openStore(t)
	entry := &model.Entry{Type: "movie", TMDBID: 7, Title: "confirmed", Genres: []string{"g"}, Countries: []string{}, Cast: []model.Actor{}}
	m := model.MediaMapping{Entry: entry, Path: "/target/video.mkv", Type: "movie", TMDBID: 7, Source: "/source/video.mkv", State: "pending"}
	r := organizer.Run{ID: "video", Purpose: "organize", State: "running", Mappings: []model.MediaMapping{m}, Ops: []organizer.Work{{Operation: organizer.Operation{Kind: "video", Target: m.Path, Status: "published"}}}}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	// Reject only the final work link: the operation, mapping and work content have already been written in this transaction.
	if _, err := s.db.Exec(`CREATE TRIGGER reject_work BEFORE INSERT ON mapping_works BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	r.Ops[0].Status = "completed"
	r.Completed = 1
	r.Mappings[0].RunID = r.ID
	changed := *entry
	changed.Title = "new confirmed snapshot"
	r.Mappings[0].Entry = &changed
	var before int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM confirmed_work_sets").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(r, []int{0}, []int{0}, r.Mappings); err == nil {
		t.Fatal("partial registration accepted")
	}
	current, err := s.Run(r.ID)
	if err != nil || current.Ops[0].Status != "published" || current.Mappings[0].Entry.Title != entry.Title {
		t.Fatal(current, err)
	}
	var after int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM confirmed_work_sets").Scan(&after); err != nil || after != before {
		t.Fatal("uncommitted work snapshot survived", before, after, err)
	}
	if m, err := s.Mapping(m.Path); err != nil || m != nil {
		t.Fatal(m, err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_work"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveCheckpoint(r, []int{0}, []int{0}, r.Mappings); err != nil {
		t.Fatal(err)
	}
	registered, err := s.Mapping(m.Path)
	if err != nil || registered.Entry.Title != "new confirmed snapshot" || registered.RunID != r.ID {
		t.Fatal(registered, err)
	}
}
