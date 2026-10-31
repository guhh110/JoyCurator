package sqlite

import (
	"path/filepath"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func TestMappingStateFollowsNewestActiveScrape(t *testing.T) {
	root := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot, settings.TargetRoot = filepath.Join(root, "source"), filepath.Join(root, "target")
	store, err := Open(filepath.Join(root, "data"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mapping := model.MediaMapping{Path: filepath.Join(settings.TargetRoot, "film.mkv"), RunID: "organize", Identity: "1:2", State: "pending"}
	if err = store.SaveMapping(mapping); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if err = store.SaveRun(organizer.Run{ID: id, Purpose: "scrape", State: "completed", Mappings: []model.MediaMapping{mapping}}); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.UpdateMappingState(mapping, "completed", "second"); err != nil {
		t.Fatal(err)
	}
	if err = store.UpdateMappingState(mapping, "failed", "first"); err != nil {
		t.Fatal(err)
	}
	current, err := store.Mapping(mapping.Path)
	if err != nil || current.State != "completed" {
		t.Fatalf("older run replaced current state: %+v %v", current, err)
	}
	if err = store.DeleteMapping(model.MediaMapping{Path: mapping.Path, RunID: "other", Identity: mapping.Identity}); err != nil {
		t.Fatal(err)
	}
	current, err = store.Mapping(mapping.Path)
	if err != nil || current == nil {
		t.Fatal("unrelated organize run deleted mapping", err)
	}
}

func TestSharedScrapeRecognizesOverwrittenPredecessor(t *testing.T) {
	root := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot, settings.TargetRoot = filepath.Join(root, "source"), filepath.Join(root, "target")
	store, err := Open(filepath.Join(root, "data"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := filepath.Join(settings.TargetRoot, "poster.jpg")
	first := model.Work{Operation: model.Operation{Kind: "image", Target: target, Status: "completed", Identity: "1:2"}}
	second := model.Work{Operation: model.Operation{Kind: "image", Target: target, Status: "completed", Identity: "1:3", ReplaceIdentity: "1:2"}}
	for _, run := range []organizer.Run{{ID: "first", Purpose: "scrape", State: "completed", Ops: []model.Work{first}}, {ID: "second", Purpose: "scrape", State: "completed", Ops: []model.Work{second}}} {
		if err = store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
	}
	shared, err := store.Shared("first", target, "1:2")
	if err != nil || !shared {
		t.Fatalf("overwritten predecessor should be retained: %v %v", shared, err)
	}
}

func TestOlderOrganizeRetryCannotReplaceNewerMapping(t *testing.T) {
	root := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot, settings.TargetRoot = filepath.Join(root, "source"), filepath.Join(root, "target")
	store, err := Open(filepath.Join(root, "data"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"old", "new"} {
		if err = store.SaveRun(organizer.Run{ID: id, Purpose: "organize", State: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(settings.TargetRoot, "film.mkv")
	if err = store.SaveMapping(model.MediaMapping{Path: path, RunID: "new", Identity: "new"}); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveMapping(model.MediaMapping{Path: path, RunID: "old", Identity: "old"}); err == nil {
		t.Fatal("old retry replaced newer mapping")
	}
	current, err := store.Mapping(path)
	if err != nil || current.RunID != "new" {
		t.Fatalf("new mapping lost: %+v %v", current, err)
	}
}
