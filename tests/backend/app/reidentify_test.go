package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

type reidentifyProvider struct {
	matching.Provider
	search func(context.Context, string, string, int) ([]model.Entry, error)
}

func (p reidentifyProvider) Search(ctx context.Context, kind, title string, year int) ([]model.Entry, error) {
	if p.search != nil {
		return p.search(ctx, kind, title, year)
	}
	if kind == "tv" {
		return []model.Entry{}, nil
	}
	return []model.Entry{{Type: "movie", TMDBID: 99, Title: title}}, nil
}
func (reidentifyProvider) Seasons(context.Context, int) ([]model.Season, error) {
	return []model.Season{{Number: 0}, {Number: 1}}, nil
}
func (reidentifyProvider) SeasonEpisodes(context.Context, int, int) ([]model.Episode, error) {
	return []model.Episode{{Number: 0}, {Number: 1}}, nil
}

func reidentifyFixture(t *testing.T) (*App, matching.Workspace) {
	t.Helper()
	a, root := fixture(t)
	w := matching.Workspace{ID: "rerun", State: "completed", Paths: []string{root + "/source"}, Completed: 3, Total: 3}
	for i := 0; i < 3; i++ {
		file := mediaFile(t, a, root, fmt.Sprintf("Old%d.mkv", i))
		result, id, err := a.dependencies.Recognition.ExtractFile(context.Background(), file.Path, root+"/source", w.ID)
		if err != nil {
			t.Fatal(err)
		}
		w.Items = append(w.Items, matching.Item{File: file, Metadata: result.Metadata, TraceID: id, Group: "old", State: "unmatched", Candidates: []model.Entry{}})
	}
	if err := a.store.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	rules := recognition.DefaultPipeline()
	rules.Annotators = []recognition.Rule{{Stage: "annotate", ID: "new-title", Name: "new title", Target: recognition.Title, Source: recognition.Filename, Enabled: true, Order: 10, Kind: "regex", Config: recognition.Config{Pattern: ".+", Value: []byte(`"Fresh"`)}}}
	if err := a.dependencies.Recognition.Save(rules); err != nil {
		t.Fatal(err)
	}
	a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) { return reidentifyProvider{}, nil }
	// Compare reconstructed snapshots rather than nil/empty collection representation.
	w, err := a.store.Workspace(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a, w
}

func TestReidentifyOnlySelectedRowsAndPreserveManualSnapshots(t *testing.T) {
	a, w := reidentifyFixture(t)
	w.Items[1].Edited = true
	w.Items[1].Entry = &model.Entry{Type: "tv", TMDBID: 42, Title: "manual snapshot", Genres: []string{"old genre"}}
	w.Items[1].Group = "manual-group"
	w.Items[1].Season = 0
	w.Items[1].Episodes = []int{0, 1}
	w.Items[1].Selected = false
	if err := a.store.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	w, _ = a.store.Workspace(w.ID)
	untouched := w.Items[2]
	if _, err := testDB(t, a).Exec(`CREATE TRIGGER protect_unselected BEFORE INSERT ON workspace_items WHEN NEW.root='rerun' AND NEW.position=2 BEGIN SELECT RAISE(ABORT,'unselected row rewritten'); END`); err != nil {
		t.Fatal(err)
	}
	reply, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{w.Items[0].Path, w.Items[1].Path})
	if err != nil || reply.State != "running" || reply.Items[0].State != "reidentifying" {
		t.Fatal(reply, err)
	}
	final := waitRecognition(t, a, w.ID)
	if final.State != "completed" || final.Completed != 3 {
		t.Fatal(final)
	}
	if final.Items[0].Entry == nil || final.Items[0].Entry.TMDBID != 99 || !final.Items[0].Selected || *final.Items[0].Metadata.Title != "Fresh" {
		t.Fatal(final.Items[0])
	}
	manual := final.Items[1]
	if manual.Entry.Title != "manual snapshot" || manual.Group != "manual-group" || manual.Season != 0 || !reflect.DeepEqual(manual.Episodes, []int{0, 1}) || manual.Selected || !manual.Edited || *manual.Metadata.Title != "Fresh" {
		t.Fatal(manual)
	}
	stored, _ := a.store.Workspace(w.ID)
	if !reflect.DeepEqual(stored.Items[2], untouched) {
		t.Fatal("unselected file changed")
	}
	for i := 0; i < 2; i++ {
		if final.Items[i].TraceID == w.Items[i].TraceID {
			t.Fatal("old trace reused")
		}
		_, oldSteps, err := a.dependencies.Recognition.Detail(w.Items[i].TraceID)
		if err != nil || len(oldSteps) != len(recognition.DefaultPipeline().Rules()) {
			t.Fatal("old trace overwritten", err)
		}
		_, steps, err := a.dependencies.Recognition.Detail(final.Items[i].TraceID)
		if err != nil || len(steps) != 7 || steps[1].RuleID != "new-title" {
			t.Fatal(steps, err)
		}
	}
}

func TestReidentifyRejectsStaleMissingDuplicateAndLockedFiles(t *testing.T) {
	a, w := reidentifyFixture(t)
	r := organizer.Run{ID: "reserved", Purpose: "organize", State: "queued", RecognitionID: w.ID, Mappings: []model.MediaMapping{{Path: "/target/file", Source: w.Items[1].Path}}}
	if err := a.store.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {"/unknown"}, {w.Items[0].Path, w.Items[0].Path}, {w.Items[1].Path}} {
		if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, paths); err == nil {
			t.Fatal("invalid request accepted", paths)
		}
	}
	if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision-1, []string{w.Items[0].Path}); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := testDB(t, a).Exec(`CREATE TRIGGER reject_rerun BEFORE UPDATE ON recognitions BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{w.Items[0].Path}); err == nil {
		t.Fatal("failed checkpoint accepted")
	}
	saved, _ := a.store.Workspace(w.ID)
	if saved.Revision != w.Revision || saved.State != "completed" || a.coordinator.RecognitionActive(w.ID) {
		t.Fatal(saved)
	}
}

func TestReidentifyCancellationAndFailureRetainRetryScope(t *testing.T) {
	for _, mode := range []string{"cancel", "failed"} {
		t.Run(mode, func(t *testing.T) {
			a, w := reidentifyFixture(t)
			entered := make(chan struct{}, 1)
			a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) {
				return reidentifyProvider{search: func(ctx context.Context, _ string, _ string, _ int) ([]model.Entry, error) {
					select {
					case entered <- struct{}{}:
					default:
					}
					if mode == "cancel" {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return nil, errors.New("upstream unavailable")
				}}, nil
			}
			if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{w.Items[0].Path}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("matching did not start")
			}
			if mode == "cancel" {
				if err := a.dependencies.Workspace.Cancel(w.ID); err != nil {
					t.Fatal(err)
				}
			}
			stopped := waitRecognition(t, a, w.ID)
			if stopped.Items[0].State != "reidentifying" || stopped.State == "completed" {
				t.Fatal(stopped)
			}
			a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) { return reidentifyProvider{}, nil }
			if _, err := a.dependencies.Workspace.Start(nil, w.ID); err != nil {
				t.Fatal(err)
			}
			final := waitRecognition(t, a, w.ID)
			if final.State != "completed" || final.Items[0].State != "ready" || final.Items[1].TraceID != w.Items[1].TraceID || final.Items[2].TraceID != w.Items[2].TraceID {
				t.Fatal(final)
			}
		})
	}
}

func TestReidentifyRestartRetriesMarkedRowsWithoutRescanningOtherSources(t *testing.T) {
	a, w := reidentifyFixture(t)
	s, _ := a.dependencies.Config.Load()
	w.Paths = []string{w.Items[0].Path, w.Items[1].Path, w.Items[2].Path}
	if err := a.store.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) {
		return reidentifyProvider{search: func(ctx context.Context, _ string, _ string, _ int) ([]model.Entry, error) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}}, nil
	}
	if _, err := a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{w.Items[0].Path}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("matching did not start")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(w.Items[1].Path); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(filepath.Dir(filepath.Dir(s.SourceRoot)), "data")
	restarted, err := openTest(t, data, s.SourceRoot, s.TargetRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) { return reidentifyProvider{}, nil }
	current, err := restarted.dependencies.Workspace.Read(w.ID)
	if err != nil || current.State != "interrupted" || current.Items[0].State != "reidentifying" {
		t.Fatal(current, err)
	}
	if _, err := restarted.dependencies.Workspace.Start(nil, w.ID); err != nil {
		t.Fatal(err)
	}
	final := waitRecognition(t, restarted, w.ID)
	if final.State != "completed" || final.Items[0].State != "ready" || final.Items[1].TraceID != w.Items[1].TraceID {
		t.Fatal(final)
	}
}
