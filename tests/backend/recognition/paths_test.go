package recognition_test

import (
	"context"
	"reflect"
	"testing"

	"joycurator/internal/recognition"
)

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
