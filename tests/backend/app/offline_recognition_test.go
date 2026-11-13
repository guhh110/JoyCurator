package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/trace"
)

func rejectRecognitionNetwork(t *testing.T, a *App) *int {
	t.Helper()
	calls := new(int)
	a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) {
		*calls++
		return nil, errors.New("recognition must stay offline")
	}
	// The factory must not even be consulted with an empty token.
	settings, err := a.store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Token = ""
	if err = a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	return calls
}
func TestImportReidentifyAndRetryStayOffline(t *testing.T) {
	a, root := fixture(t)
	calls := rejectRecognitionNetwork(t, a)
	file := mediaFile(t, a, root, "Show.tmdb-2.S01E01.mkv")
	task, err := a.dependencies.Workspace.Start([]string{file.Path}, "", "organize")
	if err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.State != "completed" || task.Items[0].State != "review" || task.Items[0].Entry != nil || task.Items[0].Confirmed() || task.Items[0].Selected {
		t.Fatal(task)
	}
	if _, err = a.dependencies.Workspace.Reidentify(task.ID, task.Revision, []string{file.Path}); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.State != "completed" {
		t.Fatal(task)
	}
	if _, err = a.dependencies.Workspace.Start(nil, task.ID, "organize"); err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	if task.State != "completed" || *calls != 0 {
		t.Fatal(task, *calls)
	}
}
func TestRegisteredVideoRestoresConfirmedWorkOffline(t *testing.T) {
	a, root := fixture(t)
	calls := rejectRecognitionNetwork(t, a)
	path := filepath.Join(root, "target", "video.mkv")
	if err := os.WriteFile(path, []byte("registered"), 0644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	entry := &model.Entry{Type: "tv", TMDBID: 2, Title: "confirmed title", OriginalTitle: "Original", Genres: []string{"genre"}, Countries: []string{"CN"}, Cast: []model.Actor{{Name: "actor", Role: "role"}}}
	mapping := model.MediaMapping{Entry: entry, Path: path, Type: "tv", TMDBID: 2, Season: 0, Episodes: []int{0, 1}, Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano(), RunID: "registered", State: "completed"}
	if err := a.store.SaveMapping(mapping); err != nil {
		t.Fatal(err)
	}
	task, err := a.dependencies.Workspace.Start([]string{filepath.Dir(path)}, "", "scrape")
	if err != nil {
		t.Fatal(err)
	}
	task = waitRecognition(t, a, task.ID)
	item := task.Items[0]
	if task.State != "completed" || !item.Confirmed() || !reflect.DeepEqual(item.Entry, entry) || item.TraceID != "" || *calls != 0 {
		t.Fatal(task, *calls)
	}
}
func TestReidentifyChangedFileRevokesConfirmationAndKeepsDraft(t *testing.T) {
	a, w := reidentifyFixture(t)
	calls := rejectRecognitionNetwork(t, a)
	w.Items[0].Entry = &model.Entry{Type: "tv", TMDBID: 2, Title: "manual"}
	w.Items[0].Season = 0
	w.Items[0].Episodes = []int{0, 1}
	w.Items[0].Group = "custom"
	w.Items[0].Selected = true
	w.Items[0].Edited = true
	seedConfirmedFiles(t, a, &w)
	if err := a.store.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.Items[0].Path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{w.Items[0].Path}); err != nil {
		t.Fatal(err)
	}
	final := waitRecognition(t, a, w.ID)
	item := final.Items[0]
	if final.State != "completed" || item.Confirmation != nil || item.Selected || item.Entry.Title != "manual" || item.Group != "custom" || item.Season != 0 || !reflect.DeepEqual(item.Episodes, []int{0, 1}) || *calls != 0 {
		t.Fatal(final, *calls)
	}
	if _, err := a.dependencies.Workspace.Confirm(context.Background(), w.ID, matching.ConfirmRequest{}); err == nil {
		t.Fatal("empty confirmation accepted")
	}
}

func TestWorkspaceFinalSaveFailureRemainsRetryable(t *testing.T) {
	a, root := fixture(t)
	calls := rejectRecognitionNetwork(t, a)
	file := mediaFile(t, a, root, "Show.S01E01.mkv")
	if _, err := testDB(t, a).Exec(`CREATE TRIGGER reject_terminal BEFORE UPDATE ON recognitions WHEN NEW.f_state IN ('completed','failed') BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	task, err := a.dependencies.Workspace.Start([]string{file.Path}, "", "organize")
	if err != nil {
		t.Fatal(err)
	}
	failed := waitRecognition(t, a, task.ID)
	if failed.State != "failed" {
		t.Fatal("uncommitted completion reported", failed)
	}
	if _, err = testDB(t, a).Exec("DROP TRIGGER reject_terminal"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Workspace.Start(nil, task.ID, "organize"); err != nil {
		t.Fatal(err)
	}
	final := waitRecognition(t, a, task.ID)
	if final.State != "completed" || len(final.Items) != 1 || final.Items[0].TraceID == "" || *calls != 0 {
		t.Fatal(final, *calls)
	}
}
