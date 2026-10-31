package app

import (
	"context"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOrganizationPreviewExcludesScrapeArtifacts(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "Show.S01E01.mkv")
	plan, err := a.dependencies.Planning.Preview(context.Background(), previewRequest(t, a, "tv", 2, "copy", "error", file))
	if err != nil || len(plan.Issues) != 0 || len(plan.Ops) != 1 || plan.Ops[0].Kind != "video" {
		t.Fatalf("organization preview: %+v %v", plan, err)
	}
	if len(plan.Mappings) != 1 || plan.Mappings[0].Path != plan.Rows[0].Target {
		t.Fatal("missing scrape mapping")
	}
}

func TestScrapePreviewUsesEpisodeMetadataAndSharedArtwork(t *testing.T) {
	a, root := fixture(t)
	seed := func(key, value string) {
		t.Helper()
		if _, err := testDB(t, a).Exec("INSERT INTO tmdb_cache VALUES(?,?,9999999999) ON CONFLICT(key) DO UPDATE SET value=excluded.value,expires_at=excluded.expires_at", key, value); err != nil {
			t.Fatal(err)
		}
	}
	seed("/tv/2/images?include_image_language=zh-CN%2Cen%2Cnull&language=zh-CN", `{"posters":[{"file_path":"/p.png","iso_639_1":"zh","vote_average":8,"vote_count":10}],"backdrops":[{"file_path":"/f.jpg","iso_639_1":null,"vote_average":7}],"logos":[{"file_path":"/l.png","iso_639_1":"en","vote_average":9}]}`)
	seed("/tv/2/season/1?language=zh-CN", `{"id":321,"season_number":1,"name":"第一季","overview":"季简介","air_date":"2023-02-01","poster_path":"/s.jpg","episodes":[{"id":11,"episode_number":1,"name":"第一集","overview":"第一个故事","air_date":"2023-02-02","still_path":"/e.jpg"},{"id":12,"episode_number":2,"name":"第二集","overview":"第二个故事","air_date":"2023-02-03","still_path":"/e2.jpg"}]}`)
	f1 := mediaFile(t, a, root, "Show.S01E01.mkv")
	f2 := mediaFile(t, a, root, "Show.S01E02.mkv")
	s, _ := a.dependencies.Config.Load()
	dir := filepath.Join(s.TargetRoot, "Show", "Season 01")
	for _, name := range []string{"first.mkv", "second.mkv"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{filepath.Join(dir, "first.mkv"), filepath.Join(dir, "second.mkv")}
	for i, path := range paths {
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		mapping := model.MediaMapping{Path: path, Source: []string{f1.Path, f2.Path}[i], Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{i + 1}, State: "pending", Identity: fsops.Identity(stat), Size: stat.Size(), MTime: stat.ModTime().UnixNano(), RunID: "fixture"}
		if err := a.store.SaveMapping(mapping); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := a.dependencies.Organizer.ScrapePreview(context.Background(), paths)
	if err != nil || len(plan.Issues) > 0 {
		t.Fatalf("scrape preview: %+v %v", plan, err)
	}
	for _, name := range []string{"tvshow.nfo", "season.nfo", "season01-poster.jpg", "first.nfo", "second.nfo", "first.jpg", "second.jpg", "poster.jpg", "fanart.jpg", "clearlogo.png"} {
		found := false
		for _, target := range plan.Artifacts {
			if filepath.Base(target) == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %s", name)
		}
	}
	for _, target := range plan.Artifacts {
		switch filepath.Base(target) {
		case "p.png", "f.jpg", "l.png":
			t.Errorf("raw TMDB image still planned: %s", target)
		}
	}
	if runtime.GOOS == "windows" {
		if _, err := a.dependencies.Organizer.ScrapeExecute(plan.ID); err == nil {
			t.Fatal("Windows scrape executed")
		}
	}
}

func TestMovieScrapePreviewKeepsOnlyStandardArtwork(t *testing.T) {
	a, root := fixture(t)
	seed := func(value string) {
		t.Helper()
		_, err := testDB(t, a).Exec(`INSERT INTO tmdb_cache VALUES(?,?,9999999999)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value,expires_at=excluded.expires_at`,
			"/movie/1/images?include_image_language=zh-CN%2Cen%2Cnull&language=zh-CN", value)
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(`{"posters":[{"file_path":"/poster-raw.jpg","iso_639_1":"en","vote_average":8,"vote_count":10},{"file_path":"/ignored.svg","iso_639_1":"en"},{"file_path":"/french.jpg","iso_639_1":"fr"}],"backdrops":[{"file_path":"/backdrop-raw.png","iso_639_1":null,"vote_average":9}],"logos":[{"file_path":"/logo-raw.png","iso_639_1":"zh"}]}`)
	path := filepath.Join(root, "target", "Film", "version.mkv")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SaveMapping(model.MediaMapping{Path: path, Type: "movie", TMDBID: 1, Identity: fsops.Identity(stat), Size: stat.Size(), MTime: stat.ModTime().UnixNano(), RunID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Organizer.ScrapePreview(context.Background(), []string{path})
	if err != nil || len(plan.Issues) != 0 {
		t.Fatalf("movie scrape preview: %+v %v", plan, err)
	}
	want := map[string]bool{"version.nfo": false, "poster.jpg": false, "fanart.jpg": false, "clearlogo.png": false}
	for _, artifact := range plan.Artifacts {
		base := filepath.Base(artifact)
		if _, ok := want[base]; ok {
			want[base] = true
		}
		if base == "ignored.svg" || base == "french.jpg" || base == "poster-raw.jpg" || base == "backdrop-raw.png" || base == "logo-raw.png" {
			t.Fatal("raw or invalid image candidate included", base)
		}
	}
	for name, found := range want {
		if !found {
			t.Error("missing artwork", name)
		}
	}
	// NFO content is included in the estimate; the image portion is three files.
	if plan.Required < 3*(30<<20) || plan.Required >= 4*(30<<20) {
		t.Fatal("image space budget does not reflect three images", plan.Required)
	}
	seed(`{"posters":[{"file_path":"/same.jpg","iso_639_1":"en"}],"backdrops":[{"file_path":"/same.jpg","iso_639_1":null}],"logos":[]}`)
	plan, err = a.dependencies.Organizer.ScrapePreview(context.Background(), []string{path})
	if err != nil || len(plan.Issues) != 0 {
		t.Fatalf("raw filename collision should not matter: %+v %v", plan, err)
	}
	for _, artifact := range plan.Artifacts {
		if filepath.Base(artifact) == "same.jpg" {
			t.Fatal("raw filename collision was planned")
		}
	}
	if len(plan.Artifacts) != 3 || plan.Required < 2*(30<<20) || plan.Required >= 3*(30<<20) {
		t.Fatal("missing logo should leave just two images and one NFO", plan.Artifacts, plan.Required)
	}
	seed(`{"posters":[],"backdrops":[],"logos":[{"file_path":"/only-logo.jpg","iso_639_1":"en"}]}`)
	plan, err = a.dependencies.Organizer.ScrapePreview(context.Background(), []string{path})
	if err != nil || len(plan.Issues) != 0 || len(plan.Artifacts) != 2 {
		t.Fatal("JPEG-only logo preview failed", plan, err)
	}
	if filepath.Base(plan.Artifacts[0]) != "clearlogo.jpg" && filepath.Base(plan.Artifacts[1]) != "clearlogo.jpg" {
		t.Fatal("JPEG logo did not keep its extension", plan.Artifacts)
	}
}

func TestScrapeWorkUnavailableAndSameEpisodeVersions(t *testing.T) {
	a, root := fixture(t)
	seed := func(key, value string) {
		t.Helper()
		_, err := testDB(t, a).Exec(`INSERT INTO tmdb_cache VALUES(?,?,9999999999)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value,expires_at=excluded.expires_at`, key, value)
		if err != nil {
			t.Fatal(err)
		}
	}
	seed("/tv/2/images?include_image_language=zh-CN%2Cen%2Cnull&language=zh-CN", `{"posters":[{"file_path":"/p.jpg","iso_639_1":"en"}],"backdrops":[],"logos":[]}`)
	seed("/tv/2/season/1?language=zh-CN", `{"id":99,"season_number":1,"name":"第一季","episodes":[{"id":11,"episode_number":1,"name":"初见山海","overview":"独立简介","air_date":"2023-01-02","still_path":"/still.jpg"}]}`)
	dir := filepath.Join(root, "target", "Show", "Season 01")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(dir, "version-a.mkv"), filepath.Join(dir, "version-b.mkv"), filepath.Join(dir, "missing.mkv")}
	for i, path := range paths {
		if err := os.WriteFile(path, []byte{byte(i)}, 0644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		mapping := model.MediaMapping{Path: path, Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{1}, Identity: fsops.Identity(info), Size: info.Size(), MTime: info.ModTime().UnixNano(), RunID: "fixture", State: "pending"}
		if err := a.store.SaveMapping(mapping); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}
	queue, err := a.dependencies.Organizer.ScrapeWork("tv", 2)
	if err != nil || len(queue) != 3 || queue[0].State != "unavailable" || queue[0].Path != paths[2] {
		t.Fatalf("missing video mapping disappeared: %+v %v", queue, err)
	}
	if _, err := a.dependencies.Organizer.ScrapePreview(context.Background(), paths); err == nil {
		t.Fatal("unavailable video was accepted")
	}
	plan, err := a.dependencies.Organizer.ScrapePreview(context.Background(), paths[:2])
	if err != nil || len(plan.Issues) != 0 {
		t.Fatalf("two versions preview: %+v %v", plan, err)
	}
	counts := map[string]int{}
	for _, target := range plan.Artifacts {
		counts[filepath.Base(target)]++
	}
	for _, name := range []string{"version-a.nfo", "version-a.jpg", "version-b.nfo", "version-b.jpg", "season.nfo", "tvshow.nfo", "poster.jpg"} {
		if counts[name] != 1 {
			t.Errorf("unexpected artifact count for %s: %d", name, counts[name])
		}
	}
}
