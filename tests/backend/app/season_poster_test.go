package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
)

type pngSeasonProvider struct{ directoryProvider }

func (*pngSeasonProvider) Seasons(context.Context, int) ([]model.Season, error) {
	return []model.Season{{Number: 0}, {Number: 1}}, nil
}

func (*pngSeasonProvider) SeasonDetail(_ context.Context, _ int, season int) (model.Season, []model.Episode, error) {
	return model.Season{Number: season, PosterPath: fmt.Sprintf("/season%d.png", season)}, []model.Episode{{Number: 1}, {Number: 2}}, nil
}
func (*pngSeasonProvider) Image(context.Context, string) ([]byte, error) {
	var out bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.RGBA{R: 255, A: 255})
	err := png.Encode(&out, im)
	return out.Bytes(), err
}

func TestSeasonPosterPathsConversionOwnershipAndUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	a, root := fixture(t)
	c := organizer.NewCoordinator()
	t.Cleanup(c.Close)
	provider := &pngSeasonProvider{}
	factory := func(model.Settings, *trace.Recorder) (matching.Provider, error) { return provider, nil }
	a.dependencies.Workspace.coordinator, a.dependencies.Workspace.factory = c, factory
	access := testAccess(t, a)
	service := organizer.New(organizer.Dependencies{Coordinator: c, Store: a.store, Settings: a.dependencies.Config, Previews: organizer.NewPreviewCache(), Access: access, Workspaces: a.dependencies.Workspace, Mappings: a.store, Sources: a.dependencies.Files, Factory: factory, Revision: a.dependencies.Workspace.Revision, Runner: func(image func(context.Context, string) ([]byte, error)) organizer.Runner {
		return &organizer.Engine{FS: access, Image: image}
	}})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	a.dependencies.Organizer = service
	videos := []string{
		targetVideo(t, root, "Show/Season 00/Show.S00E01.mkv"),
		targetVideo(t, root, "Show/Season 01/Show.S01E01.mkv"),
		targetVideo(t, root, "Show/Season 01/Show.S01E02.mkv"),
	}
	show := filepath.Join(root, "target", "Show")
	w := confirmedTarget(t, a, show)
	plan := targetPreview(t, a, w, "images")
	if len(plan.Issues) > 0 {
		t.Fatal(plan)
	}
	for _, path := range []string{filepath.Join(show, "poster.jpg"), filepath.Join(show, "Season 00", "poster.jpg"), filepath.Join(show, "Season 01", "poster.jpg")} {
		if !slices.Contains(plan.Artifacts, path) {
			t.Fatal("missing exact artwork path", path, plan)
		}
	}
	// Selecting only a season still includes the confirmed work directory's poster.
	w = confirmedTarget(t, a, filepath.Join(show, "Season 01"))
	legacy := filepath.Join(show, "Season 01", "season01-poster.jpg")
	if err := os.WriteFile(legacy, []byte("existing legacy artwork"), 0600); err != nil {
		t.Fatal(err)
	}
	plan = targetPreview(t, a, w, "images")
	if len(plan.Issues) > 0 || len(plan.Artifacts) != 2 || !slices.Contains(plan.Artifacts, filepath.Join(show, "poster.jpg")) {
		t.Fatal(plan)
	}
	batch, err := service.ScrapeExecuteBatch(plan.ID)
	if err != nil || len(batch.Runs) != 1 {
		t.Fatal(batch, err)
	}
	run := waitOptionsRun(t, service, c, batch.Runs[0].ID)
	if run.State != "completed" {
		t.Fatal(run)
	}
	for _, op := range run.Ops {
		if len(op.Owners) != 2 || !slices.Contains(op.Owners, videos[1]) || !slices.Contains(op.Owners, videos[2]) {
			t.Fatal("wrong shared ownership", op)
		}
		if op.Target == filepath.Join(show, "Season 01", "poster.jpg") && !op.ConvertJPEG {
			t.Fatal("missing PNG conversion", op)
		}
	}
	poster := filepath.Join(show, "Season 01", "poster.jpg")
	data, err := os.ReadFile(poster)
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err := image.Decode(bytes.NewReader(data)); err != nil || format != "jpeg" {
		t.Fatal(format, err)
	}
	registered, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: videos[1:], Content: "images", OnlyMissing: true})
	if err != nil || len(registered.Artifacts) != 0 || !slices.Contains(registered.Skipped, poster) {
		t.Fatal(registered, err)
	}
	overwrite, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: videos[1:], Content: "images", OnlyMissing: false})
	if err != nil || len(overwrite.Artifacts) != 2 || !slices.Contains(overwrite.Artifacts, poster) {
		t.Fatal(overwrite, err)
	}
	undo, err := service.ScrapeResume(run.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	undo = waitOptionsRun(t, service, c, undo.ID)
	if undo.State != "undone" {
		t.Fatal(undo)
	}
	for _, path := range plan.Artifacts {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("generated artwork survived undo", path, err)
		}
	}
	if data, _ := os.ReadFile(legacy); string(data) != "existing legacy artwork" {
		t.Fatal("old artwork was changed")
	}
	for _, path := range videos {
		if data, err := os.ReadFile(path); err != nil || string(data) != "unchanged video "+filepath.ToSlash(path[len(filepath.Join(root, "target"))+1:]) {
			t.Fatal("video changed", path, err)
		}
	}
}
