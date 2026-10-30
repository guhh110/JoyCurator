package recognition_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"joycurator/internal/recognition"
)

func TestRecognitionPath(t *testing.T) {
	for _, tc := range []struct {
		path, title, kind string
		year, season, id  int
		episodes          []int
	}{
		{"流浪地球.2019.2160p.mkv", "流浪地球", "", 2019, -1, 0, []int{}},
		{"[Group] Show.Name.S02E03-E05.1080p.mkv", "Show Name", "tv", 0, 2, 0, []int{3, 4, 5}},
		{"Show.S01E01E03.mkv", "Show", "tv", 0, 1, 0, []int{1, 3}},
		{"Show.S00E01.mkv", "Show", "tv", 0, 0, 0, []int{1}},
		{"Show.1x03.mkv", "Show", "tv", 0, 1, 0, []int{3}},
		{"某剧/第二季/07.mkv", "某剧", "tv", 0, 2, 0, []int{7}},
		{"Show [tmdb-2]/Season 2/03.mkv", "Show", "tv", 0, 2, 2, []int{3}},
		{"Show/Specials/01.mkv", "Show", "tv", 0, 0, 0, []int{1}},
		{"某剧/第03集.mp4", "某剧", "tv", 0, -1, 0, []int{3}},
		{"fan人修仙/第一季/第一集.mp4", "fan人修仙", "tv", 0, 1, 0, []int{1}},
		{"某剧/第一季/第十集.mp4", "某剧", "tv", 0, 1, 0, []int{10}},
		{"某剧/第一季/第十二集.mp4", "某剧", "tv", 0, 1, 0, []int{12}},
		{"某剧/第一季/第二十三集.mp4", "某剧", "tv", 0, 1, 0, []int{23}},
		{"某剧/第一季/第两百零三集.mp4", "某剧", "tv", 0, 1, 0, []int{203}},
		{"某剧/第一季/第一千零一集.mp4", "某剧", "tv", 0, 1, 0, []int{1001}},
		{"某剧/第一季/第九千九百九十九集.mp4", "某剧", "tv", 0, 1, 0, []int{9999}},
		{"某剧/第一季/第 二十 话.mp4", "某剧", "tv", 0, 1, 0, []int{20}},
		{"某剧/第一季/第〇一話.mp4", "某剧", "tv", 0, 1, 0, []int{1}},
		{"第一集.mp4", "第一集", "", 0, -1, 0, []int{}},
		{"[Group] Show - 01 [1080p][CHS].mkv", "Show", "tv", 0, -1, 0, []int{1}},
		{"1917.2019.1080p.mkv", "1917", "", 2019, -1, 0, []int{}},
		{"[Group] 1917.2019.1080p.mkv", "1917", "", 2019, -1, 0, []int{}},
		{"2001.A.Space.Odyssey.1968.mkv", "2001 A Space Odyssey", "", 1968, -1, 0, []int{}},
		{"1917.mkv", "1917", "", 0, -1, 0, []int{}},
		{"inbox/01-movies/The.Matrix.1999.tmdb-603.mkv", "The Matrix", "", 1999, -1, 603, []int{}},
		{"The.Matrix.1999.tmdb=603.mkv", "The Matrix", "", 1999, -1, 603, []int{}},
		{"Show - 03.tmdb-603.mkv", "Show", "tv", 0, -1, 603, []int{3}},
		{"Show.S01E02.tmdb-603.mkv", "Show", "tv", 0, 1, 603, []int{2}},
		{"电影/1917.mkv", "1917", "", 0, -1, 0, []int{}},
		{"4k/01.mkv", "", "tv", 0, -1, 0, []int{1}},
		{"01.mkv", "01", "", 0, -1, 0, []int{}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			p := extractMetadata(t, filepath.Join("/source", tc.path), "/source")
			if value(p.Title, "") != tc.title || value(p.MediaType, "") != tc.kind || value(p.Year, 0) != tc.year || value(p.Season, -1) != tc.season || value(p.TMDBID, 0) != tc.id || !reflect.DeepEqual(p.Episodes, tc.episodes) {
				t.Fatalf("got %+v", p)
			}
		})
	}
}
func TestChineseEpisodeExample(t *testing.T) {
	p := extractMetadata(t, "/fan人修仙/第一季/第一集.mp4", "/")
	if value(p.Title, "") != "fan人修仙" || value(p.Season, -1) != 1 || !reflect.DeepEqual(p.Episodes, []int{1}) || value(p.MediaType, "") != "tv" {
		t.Fatalf("got %+v", p)
	}
}
func TestRecognitionNeverUsesSourceName(t *testing.T) {
	p := extractMetadata(t, "/library/Show [tmdb-999]/source/Season 2/01.mkv", "/library/Show [tmdb-999]/source")
	if p.Title != nil || p.TMDBID != nil || value(p.Season, -1) != 2 {
		t.Fatal(p)
	}
	if recognition.NormalizeTitle("SHOW.Name：星空") != recognition.NormalizeTitle("show name 星空") {
		t.Fatal("punctuation normalization")
	}
}

func value[T any](v *T, zero T) T {
	if v == nil {
		return zero
	}
	return *v
}
func extractMetadata(t *testing.T, path, root string) recognition.Metadata {
	t.Helper()
	result, err := recognition.DefaultEngine().Extract(context.Background(), recognition.Input{Path: path, Root: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Metadata
}
