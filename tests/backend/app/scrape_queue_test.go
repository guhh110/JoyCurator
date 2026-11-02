package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func TestQueuedScrapeReusesOnlyUnchangedRecordedSharedArtifacts(t *testing.T) {
	a, root := fixture(t)
	video := filepath.Join(root, "target", "Film", "video.mkv")
	if err := os.MkdirAll(filepath.Dir(video), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(video)
	mapping := model.MediaMapping{Path: video, Type: "movie", TMDBID: 1, Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano(), RunID: "organized", State: "pending"}
	if err := a.store.SaveMapping(mapping); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB(t, a).Exec("INSERT INTO tmdb_cache VALUES(?,?,?)", "/movie/1/images?include_image_language=zh-CN%2Cen%2Cnull&language=zh-CN", `{"posters":[],"backdrops":[],"logos":[]}`, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	// Both previews see an absent NFO. The second confirmation happens after the first publishes it.
	first, err := a.dependencies.Organizer.ScrapePreview(context.Background(), []string{video})
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.dependencies.Organizer.ScrapePreview(context.Background(), []string{video})
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.dependencies.Organizer.ScrapeExecute(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for a.coordinator.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stored, err := a.dependencies.Organizer.ScrapeRecord(run.ID)
	if err != nil || stored.State != "completed" {
		t.Fatal(stored, err)
	}
	run, err = a.dependencies.Organizer.ScrapeExecute(second.ID)
	if err != nil {
		t.Fatal("identical owned artifact rejected", err)
	}
	for a.coordinator.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	stored, err = a.dependencies.Organizer.ScrapeRecord(run.ID)
	if err != nil || stored.State != "completed" || !stored.Ops[0].Reused {
		t.Fatal(stored, err)
	}
	third, err := a.dependencies.Organizer.ScrapePreview(context.Background(), []string{video})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stored.Ops[0].Target, []byte("external change"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Organizer.ScrapeExecute(third.ID); err == nil {
		t.Fatal("external artifact change accepted")
	}
}

func TestTwoQueuedEpisodeScrapesReuseSharedNFOAtDispatch(t *testing.T) {
	a, root := fixture(t)
	paths := []string{}
	for i, name := range []string{"one.mkv", "two.mkv"} {
		path := filepath.Join(root, "target", "Show", "Season 01", name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(path)
		if err := a.store.SaveMapping(model.MediaMapping{Path: path, Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{i + 1}, Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano(), RunID: "organized", State: "pending"}); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	if _, err := testDB(t, a).Exec("INSERT INTO tmdb_cache VALUES(?,?,9999999999)", "/tv/2/images?include_image_language=zh-CN%2Cen%2Cnull&language=zh-CN", `{"posters":[],"backdrops":[],"logos":[]}`); err != nil {
		t.Fatal(err)
	}
	c := organizer.NewCoordinator()
	t.Cleanup(c.Close)
	access := testAccess(t, a)
	runner := &heldRunner{Runner: &organizer.Engine{FS: access}, entered: make(chan struct{}), release: make(chan struct{})}
	service := organizer.New(organizer.Dependencies{Coordinator: c, Store: a.store, Settings: a.dependencies.Config, Previews: organizer.NewPreviewCache(), Access: access, Mappings: a.store, Factory: a.dependencies.Workspace.factory, Runner: func(func(context.Context, string) ([]byte, error)) organizer.Runner { return runner }})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	first, err := service.ScrapePreview(context.Background(), paths[:1])
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ScrapePreview(context.Background(), paths[1:])
	if err != nil {
		t.Fatal(err)
	}
	run1, err := service.ScrapeExecute(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("scrape did not start")
	}
	run2, err := service.ScrapeExecute(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if service.Queue().Scrape.Queued != 1 {
		t.Fatal(service.Queue())
	}
	close(runner.release)
	deadline := time.Now().Add(3 * time.Second)
	for c.Busy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	one, err := service.ScrapeRecord(run1.ID)
	if err != nil || one.State != "completed" {
		t.Fatal(one, err)
	}
	two, err := service.ScrapeRecord(run2.ID)
	if err != nil || two.State != "completed" {
		t.Fatal(two, err)
	}
	reused := 0
	for _, op := range two.Ops {
		if op.Reused {
			reused++
		}
	}
	if reused != 2 {
		t.Fatal("shared show and season NFO were not reused", two.Ops)
	}
	for _, path := range paths {
		m, err := a.store.Mapping(path)
		if err != nil || m.State != "completed" {
			t.Fatal(m, err)
		}
	}
}
