package organizer

import (
	"testing"

	"joycurator/internal/model"
)

func TestSelectArtworkLanguageBeforeRating(t *testing.T) {
	candidates := []model.Artwork{
		{Kind: "poster", Path: "/english.jpg", Language: "en", Score: 10, Votes: 1000},
		{Kind: "poster", Path: "/neutral.jpg", Score: 9, Votes: 100},
		{Kind: "poster", Path: "/primary.png", Language: "zh", Score: 7, Votes: 5},
		{Kind: "poster", Path: "/localized.jpg", Language: "zh-CN", Score: 8, Votes: 1},
		{Kind: "fanart", Path: "/english-fanart.jpg", Language: "en", Score: 10},
		{Kind: "fanart", Path: "/neutral-fanart.png", Score: 1},
		{Kind: "clearlogo", Path: "/english-logo.jpg", Language: "en", Score: 4},
		{Kind: "clearlogo", Path: "/french.png", Language: "fr", Score: 10},
	}
	selected := selectArtwork(candidates, "zh-CN")
	for kind, path := range map[string]string{
		"poster": "/localized.jpg", "fanart": "/neutral-fanart.png", "clearlogo": "/english-logo.jpg",
	} {
		if selected[kind].Path != path {
			t.Errorf("%s: got %q, want %q", kind, selected[kind].Path, path)
		}
	}
}

func TestSelectArtworkStableWithinLanguage(t *testing.T) {
	candidates := []model.Artwork{
		{Kind: "poster", Path: "/z.jpg", Language: "zh", Score: 8, Votes: 10},
		{Kind: "poster", Path: "/a.jpg", Language: "zh", Score: 8, Votes: 10},
		{Kind: "poster", Path: "/votes.png", Language: "zh", Score: 8, Votes: 11},
		{Kind: "poster", Path: "/score.png", Language: "zh", Score: 9, Votes: 0},
		{Kind: "fanart", Path: "/z.png", Language: "zh", Score: 8, Votes: 10},
		{Kind: "fanart", Path: "/a.jpg", Language: "zh", Score: 8, Votes: 10},
		{Kind: "clearlogo", Path: "/bad.svg", Language: "zh", Score: 10},
		{Kind: "clearlogo", Path: "/unrelated.jpg", Language: "fr", Score: 9},
	}
	for _, order := range [][]model.Artwork{candidates, reverseArtwork(candidates)} {
		selected := selectArtwork(order, "zh-CN")
		if selected["poster"].Path != "/score.png" || selected["fanart"].Path != "/a.jpg" {
			t.Fatal("rating or path tie-break is unstable", selected)
		}
		if _, exists := selected["clearlogo"]; exists {
			t.Fatal("unsupported artwork selected", selected)
		}
	}
	candidates = candidates[:3]
	if selected := selectArtwork(candidates, "zh-CN"); selected["poster"].Path != "/votes.png" {
		t.Fatal("vote count did not break score tie", selected)
	}
}

func reverseArtwork(input []model.Artwork) []model.Artwork {
	output := append([]model.Artwork(nil), input...)
	for i, j := 0, len(output)-1; i < j; i, j = i+1, j-1 {
		output[i], output[j] = output[j], output[i]
	}
	return output
}
