package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
)

type scrapeOptionsProvider struct {
	matching.Provider
	artwork atomic.Int64
}

func (*scrapeOptionsProvider) Detail(context.Context, string, int) (model.Entry, error) {
	return model.Entry{Type: "tv", TMDBID: 2, Title: "Show"}, nil
}
func (*scrapeOptionsProvider) SeasonDetail(context.Context, int, int) (model.Season, []model.Episode, error) {
	return model.Season{Number: 0, Name: "特别篇", PosterPath: "/season.jpg"}, []model.Episode{{Number: 0, Name: "特别篇零", Still: "/episode.jpg"}, {Number: 1, Name: "第一集", Still: "/episode.jpg"}}, nil
}
func (p *scrapeOptionsProvider) Artwork(context.Context, string, int, string) ([]model.Artwork, error) {
	p.artwork.Add(1)
	return []model.Artwork{{Kind: "poster", Path: "/poster.jpg"}, {Kind: "fanart", Path: "/fanart.jpg"}}, nil
}
func (*scrapeOptionsProvider) Image(context.Context, string) ([]byte, error) {
	return []byte("image fixture"), nil
}

func scrapeOptionsFixture(t *testing.T) (*App, *organizer.Service, *organizer.Coordinator, []string, *scrapeOptionsProvider) {
	t.Helper()
	a, root := fixture(t)
	paths := []string{}
	for _, name := range []string{"one", "two"} {
		path := filepath.Join(root, "target", "Show", "Season 00", name+".mkv")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(path)
		if err := a.store.SaveMapping(model.MediaMapping{Path: path, Type: "tv", TMDBID: 2, Season: 0, Episodes: []int{0}, RunID: "organized", Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano(), State: "pending"}); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	c := organizer.NewCoordinator()
	t.Cleanup(c.Close)
	p := &scrapeOptionsProvider{}
	access := testAccess(t, a)
	service := organizer.New(organizer.Dependencies{Coordinator: c, Store: a.store, Settings: a.dependencies.Config, Previews: organizer.NewPreviewCache(), Access: access, Mappings: a.store, Factory: func(model.Settings, *trace.Recorder) (matching.Provider, error) { return p, nil }, Runner: func(image func(context.Context, string) ([]byte, error)) organizer.Runner {
		return &organizer.Engine{FS: access, Image: image}
	}})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	return a, service, c, paths, p
}
func waitOptionsRun(t *testing.T, service *organizer.Service, c *organizer.Coordinator, id string) organizer.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !c.Busy() {
			r, err := service.ScrapeRecord(id)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scrape timed out")
	return organizer.Run{}
}

func TestScrapeContentOptionsFilterArtifactsAndUpstreamArtwork(t *testing.T) {
	_, service, _, paths, p := scrapeOptionsFixture(t)
	for _, content := range []string{"nfo", "images", "all"} {
		before := p.artwork.Load()
		plan, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: paths, Content: content})
		if err != nil || len(plan.Issues) != 0 || len(plan.Artifacts) == 0 || plan.Content != content {
			t.Fatal(plan, err)
		}
		nfo, images := 0, 0
		for _, path := range plan.Artifacts {
			if strings.HasSuffix(path, ".nfo") {
				nfo++
			} else {
				images++
			}
		}
		if content == "nfo" && (images != 0 || nfo != 4 || p.artwork.Load() != before) {
			t.Fatal(plan, "NFO requested artwork")
		}
		if content == "images" && (nfo != 0 || images != 5) {
			t.Fatal(plan)
		}
		if content == "all" && (nfo != 4 || images != 5) {
			t.Fatal(plan)
		}
	}
	for _, content := range []string{"", "unknown"} {
		if _, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: paths, Content: content}); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
}

func TestOnlyMissingPreservesExistingArtifactsAndUnaffectedMapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	a, service, c, paths, _ := scrapeOptionsFixture(t)
	dir := filepath.Dir(paths[0])
	show := filepath.Dir(dir)
	existing := []string{filepath.Join(show, "tvshow.nfo"), filepath.Join(dir, "season.nfo"), strings.TrimSuffix(paths[0], ".mkv") + ".nfo"}
	for _, path := range existing {
		if err := os.WriteFile(path, []byte("external nfo"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: paths, Content: "nfo", OnlyMissing: true})
	if err != nil || len(plan.Issues) != 0 || len(plan.Artifacts) != 1 || len(plan.Skipped) != 3 {
		t.Fatal(plan, err)
	}
	r, err := service.ScrapeExecute(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	r = waitOptionsRun(t, service, c, r.ID)
	if r.State != "completed" || len(r.Ops) != 1 || r.Ops[0].Replace || len(r.Mappings) != 1 || r.Mappings[0].Path != paths[1] {
		t.Fatal(r)
	}
	for _, path := range existing {
		if got, _ := os.ReadFile(path); string(got) != "external nfo" {
			t.Fatal("existing artifact changed", path)
		}
	}
	first, _ := a.store.Mapping(paths[0])
	second, _ := a.store.Mapping(paths[1])
	if first.State != "pending" || second.State != "completed" {
		t.Fatal(first, second)
	}
	allPresent, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: paths, Content: "nfo", OnlyMissing: true})
	if err != nil || len(allPresent.Artifacts) != 0 || len(allPresent.Skipped) != 4 {
		t.Fatal(allPresent, err)
	}
	if _, err := service.ScrapeExecute(allPresent.ID); err == nil {
		t.Fatal("empty task enqueued")
	}
	undo, err := service.ScrapeResume(r.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	undo = waitOptionsRun(t, service, c, undo.ID)
	if undo.State != "undone" {
		t.Fatal(undo)
	}
	for _, path := range existing {
		if got, _ := os.ReadFile(path); string(got) != "external nfo" {
			t.Fatal("undo touched skipped file")
		}
	}
	if _, err := os.Stat(plan.Artifacts[0]); !os.IsNotExist(err) {
		t.Fatal("generated nfo survived undo")
	}
}

func TestOnlyMissingNeverOverwritesArtifactCreatedAfterPreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	_, service, _, paths, _ := scrapeOptionsFixture(t)
	plan, err := service.ScrapePreview(context.Background(), organizer.ScrapePreviewRequest{Paths: paths[:1], Content: "nfo", OnlyMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.Artifacts[0], []byte("new external file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ScrapeExecute(plan.ID); err == nil {
		t.Fatal("stale missing preview accepted")
	}
	if got, _ := os.ReadFile(plan.Artifacts[0]); string(got) != "new external file" {
		t.Fatal("foreign file changed")
	}
}
