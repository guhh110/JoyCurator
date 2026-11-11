package matching

import (
	"joycurator/internal/model"
	"testing"
)

func TestLayoutInferenceUsesActualOutputStructure(t *testing.T) {
	item := func(path, kind string, id int) Item {
		return Item{File: model.File{Path: path, Season: 1}, Entry: &model.Entry{Type: kind, TMDBID: id}}
	}
	for _, tc := range []struct {
		name         string
		items        []Item
		work, season string
		prefix       bool
	}{
		{"split", []Item{item("/target/Show/Season 01/a.mkv", "tv", 2)}, "/target/Show", "/target/Show/Season 01", false},
		{"flat", []Item{item("/target/Show/a.mkv", "tv", 2)}, "/target/Show", "", false},
		{"nested needs confirmation", []Item{item("/target/Show/Season 01/Release/a.mkv", "tv", 2)}, "", "", false},
		{"multiple movies", []Item{item("/target/Films/a.mkv", "movie", 1), item("/target/Films/b.mkv", "movie", 2)}, "/target/Films", "", true},
		{"root movies", []Item{item("/target/a.mkv", "movie", 1)}, "/target", "", true},
		{"movie versions", []Item{item("/target/Film/a.mkv", "movie", 1), item("/target/Film/b.mkv", "movie", 1)}, "/target/Film", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			InferLayouts(tc.items, "/target")
			v := tc.items[0].Layout
			if v.WorkDir != tc.work || v.SeasonDir != tc.season || v.MoviePrefix != tc.prefix {
				t.Fatal(v)
			}
		})
	}
}
