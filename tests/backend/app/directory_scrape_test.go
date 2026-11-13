package app

import (
	"context"
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type directoryProvider struct {
	matching.Provider
	searches atomic.Int64
}

func (p *directoryProvider) Search(_ context.Context, kind, title string, _ int) ([]model.Entry, error) {
	p.searches.Add(1)
	if strings.Contains(title, "Unknown") {
		return []model.Entry{}, nil
	}
	target := "movie"
	id := 1
	if strings.Contains(title, "Show") {
		target = "tv"
		id = 2
	}
	if strings.Contains(title, "FilmB") {
		id = 3
	}
	if kind != target {
		return []model.Entry{}, nil
	}
	return []model.Entry{{Type: kind, TMDBID: id, Title: title, Year: 2024}}, nil
}
func (*directoryProvider) Detail(_ context.Context, kind string, id int) (model.Entry, error) {
	return model.Entry{Type: kind, TMDBID: id, Title: map[int]string{1: "FilmA", 2: "Show", 3: "FilmB"}[id], Year: 2024}, nil
}
func (*directoryProvider) Seasons(context.Context, int) ([]model.Season, error) {
	return []model.Season{{Number: 1}, {Number: 2}}, nil
}
func (*directoryProvider) SeasonEpisodes(context.Context, int, int) ([]model.Episode, error) {
	return []model.Episode{{Number: 1}, {Number: 2}, {Number: 3}, {Number: 4}}, nil
}
func (*directoryProvider) SeasonDetail(_ context.Context, _ int, n int) (model.Season, []model.Episode, error) {
	return model.Season{Number: n, Name: "Season", PosterPath: "/season.jpg"}, []model.Episode{{Number: 1, Name: "One", Still: "/still.jpg"}, {Number: 2, Name: "Two", Still: "/still.jpg"}}, nil
}
func (*directoryProvider) Artwork(context.Context, string, int, string) ([]model.Artwork, error) {
	return []model.Artwork{{Kind: "poster", Path: "/poster.jpg"}}, nil
}
func (*directoryProvider) Image(context.Context, string) ([]byte, error) {
	return []byte("fixture image"), nil
}
func directoryFixture(t *testing.T) (*App, string, *directoryProvider) {
	t.Helper()
	a, root := fixture(t)
	c := organizer.NewCoordinator()
	t.Cleanup(c.Close)
	p := &directoryProvider{}
	factory := func(model.Settings, *trace.Recorder) (matching.Provider, error) { return p, nil }
	a.dependencies.Workspace.coordinator = c
	a.dependencies.Workspace.factory = factory
	access := testAccess(t, a)
	service := organizer.New(organizer.Dependencies{Coordinator: c, Store: a.store, Settings: a.dependencies.Config, Previews: organizer.NewPreviewCache(), Access: access, Workspaces: a.dependencies.Workspace, Mappings: a.store, Sources: a.dependencies.Files, Factory: factory, Revision: a.dependencies.Workspace.Revision, Runner: func(image func(context.Context, string) ([]byte, error)) organizer.Runner {
		return &organizer.Engine{FS: access, Image: image}
	}})
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	a.dependencies.Organizer = service
	return a, root, p
}
func targetVideo(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, "target", name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("unchanged video "+name), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
func confirmedTarget(t *testing.T, a *App, path string) matching.Workspace {
	t.Helper()
	w, e := a.dependencies.Workspace.Start([]string{path}, "", "scrape")
	if e != nil {
		t.Fatal(e)
	}
	w = waitRecognition(t, a, w.ID)
	if w.State != "completed" {
		t.Fatal(w)
	}
	groups := map[string][]matching.ConfirmItem{}
	for _, it := range w.Items {
		if it.Confirmed() {
			continue
		}
		e := model.Entry{Type: "movie", TMDBID: 1}
		if it.Metadata.MediaType != nil && *it.Metadata.MediaType == "tv" {
			e.Type, e.TMDBID = "tv", 2
		} else if strings.Contains(it.Path, "FilmB") {
			e.TMDBID = 3
		} else if strings.Contains(it.Path, "Unknown") {
			continue
		}
		season := it.Season
		if e.Type == "movie" {
			season = -1
		}
		eps := it.Episodes
		if e.Type == "movie" {
			eps = []int{}
		}
		key := matching.EntryGroup(e)
		groups[key] = append(groups[key], matching.ConfirmItem{Path: it.Path, Type: e.Type, TMDBID: e.TMDBID, Season: season, Episodes: eps, Basis: matching.EpisodeBasis{Kind: "manual"}})
	}
	for _, items := range groups {
		var err error
		w, err = a.dependencies.Workspace.Confirm(context.Background(), w.ID, matching.ConfirmRequest{Revision: w.Revision, Items: items})
		if err != nil {
			t.Fatal(err)
		}
	}
	return w
}
func targetPreview(t *testing.T, a *App, w matching.Workspace, content string) organizer.ScrapePlan {
	t.Helper()
	p, e := a.dependencies.Organizer.ScrapeBatchPreview(context.Background(), organizer.ScrapeBatchPreviewRequest{RecognitionID: w.ID, Revision: w.Revision, Content: content, OnlyMissing: true})
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestDirectoryScrapeEndToEndBatchAndRegistration(t *testing.T) {
	a, root, _ := directoryFixture(t)
	videos := []string{targetVideo(t, root, "FilmA/FilmA.2024.mkv"), targetVideo(t, root, "Show/Season 01/Show.S01E01.mkv"), targetVideo(t, root, "Show/Season 01/Show.S01E02.mkv")}
	before := map[string][]byte{}
	for _, p := range videos {
		before[p], _ = os.ReadFile(p)
	}
	w := confirmedTarget(t, a, filepath.Join(root, "target"))
	if len(w.Items) != 3 {
		t.Fatal(w)
	}
	for _, it := range w.Items {
		if it.State != "ready" || it.Identity == "" || it.Layout.WorkDir == "" {
			t.Fatal(it)
		}
		if m, _ := a.store.Mapping(it.Path); m != nil {
			t.Fatal("scan registered video", m)
		}
	}
	latest, e := a.dependencies.Workspace.Latest("scrape")
	if e != nil || latest.ID != w.ID {
		t.Fatal(latest, e)
	}
	if v, e := a.dependencies.Workspace.Latest("organize"); e != nil || v != nil {
		t.Fatal(v, e)
	}
	if _, e = a.dependencies.Organizer.BatchPreview(context.Background(), organizer.BatchPreviewRequest{RecognitionID: w.ID, Revision: w.Revision, Mode: "move", DuplicatePolicy: "skip"}); e == nil {
		t.Fatal("scrape accepted by organizer")
	}
	p := targetPreview(t, a, w, "nfo")
	if len(p.Groups) != 2 || len(p.Issues) != 0 {
		t.Fatal(p)
	}
	batch, e := a.dependencies.Organizer.ScrapeExecuteBatch(p.ID)
	if e != nil || len(batch.Runs) != 2 {
		t.Fatal(batch, e)
	}
	if _, e = a.dependencies.Organizer.ScrapeExecuteBatch(p.ID); e == nil {
		t.Fatal("preview replay accepted")
	}
	for _, r := range batch.Runs {
		stored := waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, r.ID)
		if stored.State != "completed" {
			t.Fatal(stored)
		}
		if len(stored.Mappings) == 0 || stored.Mappings[0].RunID != stored.ID {
			t.Fatal(stored)
		}
	}
	for _, p := range videos {
		b, _ := os.ReadFile(p)
		if !reflect.DeepEqual(b, before[p]) {
			t.Fatal("video modified", p)
		}
		m, e := a.store.Mapping(p)
		if e != nil || m == nil || m.State != "completed" || m.Layout.WorkDir == "" {
			t.Fatal(m, e)
		}
	}
	after, _ := a.store.Workspace(w.ID)
	if after.Revision != w.Revision {
		t.Fatal("progress changed revision")
	}
	// Completed outputs do not turn into empty queue tasks or register scanned files again.
	w = confirmedTarget(t, a, filepath.Join(root, "target"))
	p = targetPreview(t, a, w, "nfo")
	if len(p.Artifacts) != 0 || len(p.Skipped) == 0 {
		t.Fatal(p)
	}
	if _, e = a.dependencies.Organizer.ScrapeExecuteBatch(p.ID); e == nil {
		t.Fatal("empty task accepted")
	}
}

func TestDirectoryScrapeLayoutAndMoviePrefixes(t *testing.T) {
	a, root, _ := directoryFixture(t)
	show := targetVideo(t, root, "Show/Show.S01E01.mkv")
	targetVideo(t, root, "Show/Show.S02E01.mkv")
	w := confirmedTarget(t, a, filepath.Dir(show))
	p := targetPreview(t, a, w, "all")
	if len(p.Issues) != 0 {
		t.Fatal(p)
	}
	joined := strings.Join(p.Artifacts, "\n")
	if strings.Contains(joined, "season.nfo") || !strings.Contains(joined, "season01-poster.jpg") || !strings.Contains(joined, "season02-poster.jpg") || !strings.Contains(joined, "tvshow.nfo") {
		t.Fatal(p)
	}
	for _, path := range p.Artifacts {
		if filepath.Dir(path) != filepath.Dir(show) {
			t.Fatal("flat show escaped", path)
		}
	}
	first := targetVideo(t, root, "Flat/FilmA.2024.mkv")
	second := targetVideo(t, root, "Flat/FilmB.2024.mkv")
	w = confirmedTarget(t, a, filepath.Dir(first))
	p = targetPreview(t, a, w, "images")
	if len(p.Issues) != 0 || len(p.Artifacts) != 2 {
		t.Fatal(p)
	}
	for _, video := range []string{first, second} {
		want := strings.TrimSuffix(video, ".mkv") + "-poster.jpg"
		if !strings.Contains(strings.Join(p.Artifacts, "\n"), want) {
			t.Fatal(p, w)
		}
	}
	season := targetVideo(t, root, "Other/Season 01/Show.S01E01.mkv")
	w = confirmedTarget(t, a, filepath.Dir(season))
	p = targetPreview(t, a, w, "nfo")
	if !strings.Contains(strings.Join(p.Artifacts, "\n"), filepath.Join(root, "target", "Other", "tvshow.nfo")) {
		t.Fatal("selected season lost show root", p)
	}
}

func TestDirectoryScrapeRejectsStaleAcceptance(t *testing.T) {
	for _, change := range []string{"video", "revision", "settings", "mapping", "output", "ancestor"} {
		t.Run(change, func(t *testing.T) {
			a, root, _ := directoryFixture(t)
			video := targetVideo(t, root, "FilmA/FilmA.2024.mkv")
			w := confirmedTarget(t, a, filepath.Dir(video))
			p := targetPreview(t, a, w, "nfo")
			switch change {
			case "video":
				_ = os.WriteFile(video, []byte("changed"), 0644)
			case "revision":
				_ = a.store.SaveWorkspace(&w)
			case "settings":
				s, _ := a.store.LoadSettings()
				s.Version++
				_ = a.store.SaveSettings(s)
			case "mapping":
				info, _ := os.Stat(video)
				_ = a.store.SaveMapping(model.MediaMapping{Entry: &model.Entry{Type: "movie", TMDBID: 99, Title: "fixture"}, Path: video, Type: "movie", TMDBID: 99, RunID: "external", Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano()})
			case "ancestor":
				moved := filepath.Dir(video) + "-moved"
				if e := os.Rename(filepath.Dir(video), moved); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(moved, filepath.Dir(video)); e != nil {
					t.Fatal(e)
				}
			case "output":
				_ = os.WriteFile(p.Artifacts[0], []byte("external"), 0644)
			}
			if _, e := a.dependencies.Organizer.ScrapeExecuteBatch(p.ID); e == nil {
				t.Fatal("stale preview accepted", change)
			}
			queue, _ := a.store.QueuePage("scrape", 1)
			if queue.Total != 0 {
				t.Fatal("partial admission", queue)
			}
		})
	}
}

func TestTargetDirectoryScopeAndSymlinks(t *testing.T) {
	a, root, _ := directoryFixture(t)
	targetVideo(t, root, "FilmA/FilmA.2024.mkv")
	if _, e := a.dependencies.Workspace.Start([]string{root + "/source"}, "", "scrape"); e == nil {
		t.Fatal("source accepted")
	}
	if _, e := a.dependencies.Workspace.Start([]string{root + "/target"}, "", "organize"); e == nil {
		t.Fatal("target accepted as source")
	}
	if _, e := a.dependencies.Files.Browse(root+"/source", "target"); e == nil {
		t.Fatal("browse escaped")
	}
	if _, e := a.dependencies.Files.Browse("", "target"); e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(root, "target", ".hidden.mkv"), []byte("hidden"), 0644)
	if e := os.Symlink(filepath.Join(root, "target", "FilmA"), filepath.Join(root, "target", "link")); e != nil {
		t.Fatal(e)
	}
	if _, e := a.dependencies.Workspace.Start([]string{root + "/target/link"}, "", "scrape"); e == nil {
		t.Fatal("symlink accepted")
	}
	w := confirmedTarget(t, a, root+"/target")
	if len(w.Items) != 1 {
		t.Fatal(w)
	}
	empty := filepath.Join(root, "target", "empty")
	_ = os.Mkdir(empty, 0755)
	w = confirmedTarget(t, a, empty)
	if len(w.Items) != 0 {
		t.Fatal(w)
	}
}

func TestDirectoryScrapeRegistrationFailureIsAtomicAndRetryable(t *testing.T) {
	a, root, _ := directoryFixture(t)
	video := targetVideo(t, root, "FilmA/FilmA.2024.mkv")
	w := confirmedTarget(t, a, filepath.Dir(video))
	p := targetPreview(t, a, w, "nfo")
	if _, e := testDB(t, a).Exec(`CREATE TRIGGER reject_directory_mapping BEFORE INSERT ON media_mappings BEGIN SELECT RAISE(ABORT,'fixture mapping failure'); END`); e != nil {
		t.Fatal(e)
	}
	batch, e := a.dependencies.Organizer.ScrapeExecuteBatch(p.ID)
	if e != nil {
		t.Fatal(e)
	}
	r := waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, batch.Runs[0].ID)
	if r.State != "failed" {
		t.Fatal(r)
	}
	if m, _ := a.store.Mapping(video); m != nil {
		t.Fatal("failed transaction registered mapping")
	}
	if r.Mappings[0].RunID != "" {
		t.Fatal("checkpoint committed before mapping", r)
	}
	if _, e = os.Stat(p.Artifacts[0]); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("artifact written before registration", e)
	}
	_, _ = testDB(t, a).Exec("DROP TRIGGER reject_directory_mapping")
	if _, e = a.dependencies.Organizer.ScrapeResume(r.ID, false); e != nil {
		t.Fatal(e)
	}
	r = waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, r.ID)
	if r.State != "completed" {
		t.Fatal(r)
	}
	if _, e = a.dependencies.Organizer.ScrapeResume(r.ID, true); e != nil {
		t.Fatal(e)
	}
	r = waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, r.ID)
	if r.State != "undone" {
		t.Fatal(r)
	}
	if b, e := os.ReadFile(video); e != nil || !strings.HasPrefix(string(b), "unchanged video") {
		t.Fatal("undo touched video", e)
	}
}

func TestDirectoryScrapeReusesRegistrationAndPersistsCorrections(t *testing.T) {
	a, root, provider := directoryFixture(t)
	video := targetVideo(t, root, "FilmA/FilmA.2024.mkv")
	w := confirmedTarget(t, a, filepath.Dir(video))
	p := targetPreview(t, a, w, "nfo")
	batch, e := a.dependencies.Organizer.ScrapeExecuteBatch(p.ID)
	if e != nil {
		t.Fatal(e)
	}
	first := waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, batch.Runs[0].ID)
	if first.State != "completed" {
		t.Fatal(first)
	}
	searches := provider.searches.Load()
	w = confirmedTarget(t, a, filepath.Dir(video))
	if provider.searches.Load() != searches || w.Items[0].Original == nil {
		t.Fatal("registration was rematched", w)
	}
	it := w.Items[0]
	changed, e := a.dependencies.Workspace.Update(context.Background(), w.ID, matching.Update{Revision: w.Revision, Items: []matching.Edit{{Path: video, Type: "movie", TMDBID: 3, Selected: true, Season: it.Season, Episodes: it.Episodes, Layout: model.MediaLayout{WorkDir: filepath.Dir(video), MoviePrefix: true, Manual: true}}}})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := a.store.Workspace(w.ID)
	if e != nil || raw.Items[0].Original.TMDBID != 1 || raw.Items[0].Entry.TMDBID != 3 || !raw.Items[0].Layout.Manual {
		t.Fatal(raw, e)
	}
	mapping, _ := a.store.Mapping(video)
	if mapping.TMDBID != 1 {
		t.Fatal("edit changed catalog", mapping)
	}
	changed, e = a.dependencies.Workspace.Confirm(context.Background(), changed.ID, matching.ConfirmRequest{Revision: changed.Revision, Items: []matching.ConfirmItem{{Path: video, Type: "movie", TMDBID: 3, Season: -1, Episodes: []int{}, Basis: matching.EpisodeBasis{Kind: "manual"}}}})
	if e != nil {
		t.Fatal(e)
	}
	p, e = a.dependencies.Organizer.ScrapeBatchPreview(context.Background(), organizer.ScrapeBatchPreviewRequest{RecognitionID: changed.ID, Revision: changed.Revision, Content: "nfo", OnlyMissing: false})
	if e != nil {
		t.Fatal(e)
	}
	batch, e = a.dependencies.Organizer.ScrapeExecuteBatch(p.ID)
	if e != nil {
		t.Fatal(e)
	}
	run := waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, batch.Runs[0].ID)
	if run.State != "completed" || run.Mappings[0].Original == nil || run.Mappings[0].Original.TMDBID != 1 || run.Mappings[0].TMDBID != 3 {
		t.Fatal(run)
	}
	mapping, _ = a.store.Mapping(video)
	if mapping.TMDBID != 3 || mapping.Original != nil || !mapping.Layout.MoviePrefix {
		t.Fatal(mapping)
	}
	// A later registration owns the path; undoing the earlier scrape must not erase it.
	if _, e = a.dependencies.Organizer.ScrapeResume(first.ID, true); e != nil {
		t.Fatal(e)
	}
	_ = waitOptionsRun(t, a.dependencies.Organizer, a.dependencies.Workspace.coordinator, first.ID)
	mapping, _ = a.store.Mapping(video)
	if mapping.TMDBID != 3 || mapping.RunID != run.ID {
		t.Fatal("undo erased newer mapping", mapping)
	}
}

func TestDirectoryScrapeAmbiguousLayoutAndCrossWorkCollision(t *testing.T) {
	a, root, _ := directoryFixture(t)
	video := targetVideo(t, root, "Show/Season 01/Release/Show.S01E01.mkv")
	w := confirmedTarget(t, a, filepath.Join(root, "target", "Show"))
	if w.Items[0].Layout.WorkDir != "" {
		t.Fatal("nested layout auto-confirmed", w)
	}
	if _, e := a.dependencies.Organizer.ScrapeBatchPreview(context.Background(), organizer.ScrapeBatchPreviewRequest{RecognitionID: w.ID, Revision: w.Revision, Content: "all"}); e == nil {
		t.Fatal("ambiguous layout submitted")
	}
	it := w.Items[0]
	layout := model.MediaLayout{WorkDir: filepath.Join(root, "target", "Show"), SeasonDir: filepath.Join(root, "target", "Show", "Season 01"), Manual: true}
	w, e := a.dependencies.Workspace.Update(context.Background(), w.ID, matching.Update{Revision: w.Revision, Items: []matching.Edit{{Path: video, Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{1}, Selected: true, Layout: layout, Group: it.Group}}})
	if e != nil {
		t.Fatal(e)
	}
	p := targetPreview(t, a, w, "all")
	if len(p.Issues) != 0 {
		t.Fatal(p)
	}
	first := targetVideo(t, root, "Mixed/FilmA.2024.mkv")
	targetVideo(t, root, "Mixed/FilmB.2024.mkv")
	w = confirmedTarget(t, a, filepath.Dir(first))
	edits := []matching.Edit{}
	for _, it := range w.Items {
		edits = append(edits, matching.Edit{Path: it.Path, Type: it.Entry.Type, TMDBID: it.Entry.TMDBID, Selected: true, Layout: model.MediaLayout{WorkDir: filepath.Dir(first), Manual: true}, Season: it.Season, Episodes: it.Episodes})
	}
	w, e = a.dependencies.Workspace.Update(context.Background(), w.ID, matching.Update{Revision: w.Revision, Items: edits})
	if e != nil {
		t.Fatal(e)
	}
	p = targetPreview(t, a, w, "images")
	if len(p.Issues) == 0 {
		t.Fatal("cross-work poster overwrite accepted", p)
	}
	if _, e = a.dependencies.Organizer.ScrapeExecuteBatch(p.ID); e == nil {
		t.Fatal("conflicted batch queued")
	}
}

func TestDirectoryScrapeSkippedVideoStillInvalidatesPreview(t *testing.T) {
	a, root, _ := directoryFixture(t)
	first := targetVideo(t, root, "FilmA/FilmA.2024.mkv")
	targetVideo(t, root, "FilmA/FilmA.2024.[1080p].mkv")
	_ = os.WriteFile(strings.TrimSuffix(first, ".mkv")+".nfo", []byte("existing"), 0644)
	w := confirmedTarget(t, a, filepath.Dir(first))
	p := targetPreview(t, a, w, "nfo")
	if len(p.Artifacts) != 1 || len(p.Skipped) != 1 {
		t.Fatal(p)
	}
	_ = os.WriteFile(first, []byte("changed skipped video"), 0644)
	if _, e := a.dependencies.Organizer.ScrapeExecuteBatch(p.ID); e == nil {
		t.Fatal("changed skipped input accepted")
	}
}

func TestDirectoryScanCancellationRetriesWithinTarget(t *testing.T) {
	a, root, _ := directoryFixture(t)
	video := targetVideo(t, root, "FilmA/FilmA.2024.mkv")
	gate := gateRecognition(t, a, nil)
	w, e := a.dependencies.Workspace.Start([]string{filepath.Dir(video)}, "", "scrape")
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("local recognition did not start")
	}
	if e = a.dependencies.Workspace.Cancel(w.ID); e != nil {
		t.Fatal(e)
	}
	close(gate.release)
	w = waitRecognition(t, a, w.ID)
	if w.State != "cancelled" || w.Purpose != "scrape" {
		t.Fatal(w)
	}
	a.dependencies.Workspace.extract = a.dependencies.Recognition
	if _, e = a.dependencies.Workspace.Start(nil, w.ID, "scrape"); e != nil {
		t.Fatal(e)
	}
	w = waitRecognition(t, a, w.ID)
	if w.State != "completed" || w.Items[0].State != "review" || w.Items[0].Confirmation != nil {
		t.Fatal(w)
	}
}
