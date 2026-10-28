package recognition_test

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, title, kind string
		year, season      int
		episodes          []int
	}{{"流浪地球.2019.2160p.mkv", "流浪地球", "movie", 2019, 0, []int{}}, {"Show.Name.S01E01E02.1080p.mkv", "Show Name", "tv", 0, 1, []int{1, 2}}, {"Show.S00E02.mkv", "Show", "tv", 0, 0, []int{2}}, {"Show.1x03.mkv", "Show", "tv", 0, 1, []int{3}}, {"Show.S02E03-E05.mkv", "Show", "tv", 0, 2, []int{3, 4, 5}}} {
		t.Run(tc.name, func(t *testing.T) {
			p := Parse(tc.name)
			if p.Title != tc.title || p.Type != tc.kind || p.Year != tc.year || p.Season != tc.season || !reflect.DeepEqual(p.Episodes, tc.episodes) {
				t.Fatalf("parsed %#v", p)
			}
		})
	}
}
func TestParsePathUsesFolderAsTitle(t *testing.T) {
	for _, tc := range []struct {
		path     string
		title    string
		kind     string
		season   int
		episodes []int
	}{
		{"凡人修仙传/1.mp4", "凡人修仙传", "tv", 0, []int{1}},
		{"凡人修仙传/12.mp4", "凡人修仙传", "tv", 0, []int{12}},
		{"凡人修仙传/第03集.mp4", "凡人修仙传", "tv", 0, []int{3}},
		{"凡人修仙传/EP04.mkv", "凡人修仙传", "tv", 0, []int{4}},
		{"Show/Season 2/03.mkv", "Show", "tv", 2, []int{3}},
		{"Show/S02/03.mkv", "Show", "tv", 2, []int{3}},
		{"某剧/第二季/07.mkv", "某剧", "tv", 2, []int{7}},
		{"[Group] Show [1080p]/01.mkv", "Show", "tv", 0, []int{1}},
		{"【喵萌】某番【1080P】/05.mp4", "某番", "tv", 0, []int{5}},
		{"剧名 2020/01.mkv", "剧名", "tv", 0, []int{1}},
	} {
		parsed := ParsePath(tc.path)
		if parsed.Title != tc.title || parsed.Type != tc.kind || parsed.Season != tc.season || !reflect.DeepEqual(parsed.Episodes, tc.episodes) {
			t.Fatalf("%s → %#v", tc.path, parsed)
		}
	}
	if parsed := ParsePath("剧名 2020/01.mkv"); parsed.Year != 2020 {
		t.Fatalf("year %d", parsed.Year)
	}
	// A normal file name keeps its own title.
	parsed := ParsePath("Show.S01E02.1080p.mkv")
	if parsed.Title != "Show" || parsed.Season != 1 || !reflect.DeepEqual(parsed.Episodes, []int{2}) {
		t.Fatalf("regular name %#v", parsed)
	}
}

func TestParsePathTrailingEpisode(t *testing.T) {
	for _, tc := range []struct {
		path     string
		title    string
		episodes []int
	}{
		{"Show - 01.mkv", "Show", []int{1}},
		{"[Group] Show - 01 [1080p].mkv", "Show", []int{1}},
		{"[Group] 某番 - 12 [1080P][CHS].mkv", "某番", []int{12}},
	} {
		parsed := ParsePath(tc.path)
		if parsed.Title != tc.title || parsed.Type != "tv" || !reflect.DeepEqual(parsed.Episodes, tc.episodes) {
			t.Fatalf("%s → %#v", tc.path, parsed)
		}
	}
	// A four-digit year must not be mistaken for an episode.
	if parsed := ParsePath("Some.Movie.2020.mkv"); parsed.Type != "movie" || parsed.Year != 2020 {
		t.Fatalf("year handled as episode: %#v", parsed)
	}
	// Season/episode patterns keep priority.
	if parsed := ParsePath("Show.S02E05.mkv"); parsed.Season != 2 || !reflect.DeepEqual(parsed.Episodes, []int{5}) {
		t.Fatalf("season pattern %#v", parsed)
	}
}

// A folder that only carries a quality tag must not become the title, and the
// season stays unknown until the user picks one.
func TestParsePathSkipsQualityFolders(t *testing.T) {
	root := "/media/inbox"
	parsed := ParsePathIn("/media/inbox/4k/1.mp4", []string{root})
	if parsed.Title != "" || parsed.Season != 0 || parsed.Type != "tv" || !reflect.DeepEqual(parsed.Episodes, []int{1}) {
		t.Fatalf("%#v", parsed)
	}
	// Without roots the walk still skips the quality folder.
	if parsed = ParsePath("/media/inbox/4k/1.mp4"); parsed.Season != 0 {
		t.Fatalf("season guessed: %#v", parsed)
	}
	// A folder with both a name and a tag keeps the name.
	parsed = ParsePathIn("/media/inbox/Show 4K/2.mp4", []string{root})
	if parsed.Title != "Show" || parsed.Season != 0 {
		t.Fatalf("%#v", parsed)
	}
	// A season folder still wins over the title folder.
	parsed = ParsePathIn("/media/inbox/Show/Season 2/3.mp4", []string{root})
	if parsed.Title != "Show" || parsed.Season != 2 {
		t.Fatalf("%#v", parsed)
	}
}

func TestParsePathDoesNotUseTMDBIDAsEpisode(t *testing.T) {
	parsed := ParsePath("/source/inbox/01-movies/The.Matrix.1999.tmdb-603.mkv")
	if parsed.Type != "movie" || parsed.TMDBID != 603 || parsed.Title != "The Matrix" || parsed.Year != 1999 || len(parsed.Episodes) != 0 {
		t.Fatal(parsed)
	}
	parsed = ParsePath("/source/Show - 03.tmdb-603.mkv")
	if parsed.Type != "tv" || !reflect.DeepEqual(parsed.Episodes, []int{3}) {
		t.Fatal(parsed)
	}
}
