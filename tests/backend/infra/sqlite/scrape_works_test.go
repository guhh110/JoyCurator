package sqlite

import (
	"fmt"
	"path/filepath"
	"testing"

	"joycurator/internal/model"
)

func TestScrapeWorksSearchAndCursor(t *testing.T) {
	root := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot, settings.TargetRoot = filepath.Join(root, "source"), filepath.Join(root, "target")
	store, err := Open(filepath.Join(root, "data"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for id := 1; id <= 105; id++ {
		for version, state := range []string{"pending", "failed", "completed"} {
			path := filepath.Join(settings.TargetRoot, fmt.Sprintf("Film %03d", id), fmt.Sprintf("version-%d.mkv", version))
			if id == 90 && version == 1 {
				path = filepath.Join(settings.TargetRoot, "Film 090", "needle.mkv")
			}
			if err := store.SaveMapping(model.MediaMapping{Path: path, Type: "movie", TMDBID: id, State: state, RunID: "seed"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	page, err := store.ScrapeWorks("", "")
	if err != nil || len(page.Items) != 50 || page.NextCursor != "movie:50" {
		t.Fatal(page, err)
	}
	seen := map[int]bool{}
	for page.NextCursor != "" {
		for _, item := range page.Items {
			if seen[item.TMDBID] || item.Pending != 1 || item.Failed != 1 || item.Completed != 1 || item.Total != 3 {
				t.Fatal("duplicate or wrong counts", item)
			}
			seen[item.TMDBID] = true
		}
		page, err = store.ScrapeWorks("", page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range page.Items {
		if seen[item.TMDBID] {
			t.Fatal("duplicate", item)
		}
		seen[item.TMDBID] = true
	}
	if len(seen) != 105 {
		t.Fatal("missing works", len(seen))
	}
	for _, search := range []string{"needle.mkv", "090"} {
		result, err := store.ScrapeWorks(search, "")
		if err != nil || len(result.Items) != 1 || result.Items[0].TMDBID != 90 {
			t.Fatal(search, result, err)
		}
	}
	if err := store.SaveMapping(model.MediaMapping{Path: filepath.Join(settings.TargetRoot, "Series", "Season 01", "episode.mkv"), Type: "tv", TMDBID: 2, State: "pending", RunID: "seed"}); err != nil {
		t.Fatal(err)
	}
	last, err := store.ScrapeWorks("", "movie:105")
	if err != nil || len(last.Items) != 1 || last.Items[0].Type != "tv" || last.NextCursor != "" {
		t.Fatal(last, err)
	}
	series, err := store.ScrapeWorks("episode.mkv", "")
	if err != nil || len(series.Items) != 1 || series.Items[0].Type != "tv" {
		t.Fatal(series, err)
	}
	items, err := store.ScrapeWorkMappings("movie", 90)
	if err != nil || len(items) != 3 {
		t.Fatal(items, err)
	}
	for _, cursor := range []string{"tv:0", "movie:01", "bad", "movie:1 trailing"} {
		if _, err := store.ScrapeWorks("", cursor); err == nil {
			t.Fatal("accepted cursor", cursor)
		}
	}
}
