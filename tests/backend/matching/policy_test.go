package matching

import (
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

func TestCandidatePolicyFromSuppliedFacts(t *testing.T) {
	movie := model.Entry{Type: "movie", TMDBID: 603, Title: "黑客帝国", OriginalTitle: "The Matrix", Year: 1999}
	tv := model.Entry{Type: "tv", TMDBID: 603, Title: "Other", Year: 1997}
	for _, tc := range []struct {
		name   string
		parsed recognition.Metadata
		facts  []Candidate
		state  string
		wantID int
	}{
		{"unique title and year", recognition.MetadataFromParsed(model.Parsed{Title: "The.Matrix", Year: 1999}), []Candidate{{Entry: movie}}, "ready", 603},
		{"same title ambiguity", recognition.MetadataFromParsed(model.Parsed{Title: "The Matrix"}), []Candidate{{Entry: movie}, {Entry: movie}}, "review", 0},
		{"year fallback", recognition.MetadataFromParsed(model.Parsed{Title: "The Matrix", Year: 1999}), []Candidate{{Entry: movie, Relaxed: true}}, "review", 0},
		{"year conflict", recognition.MetadataFromParsed(model.Parsed{Title: "The Matrix", Year: 2000}), []Candidate{{Entry: movie}}, "review", 0},
		{"id collision unknown title", recognition.MetadataFromParsed(model.Parsed{TMDBID: 603}), []Candidate{{Entry: movie}, {Entry: tv}}, "review", 0},
		{"id title disambiguation", recognition.MetadataFromParsed(model.Parsed{TMDBID: 603, Title: "The Matrix", Year: 1999}), []Candidate{{Entry: movie}, {Entry: tv}}, "ready", 603},
		{"no results", recognition.MetadataFromParsed(model.Parsed{Title: "Missing"}), nil, "unmatched", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := DecideMatch(tc.parsed, tc.facts)
			if decision.State != tc.state {
				t.Fatal(decision)
			}
			if tc.wantID > 0 && (decision.Entry == nil || decision.Entry.TMDBID != tc.wantID) {
				t.Fatal(decision)
			}
			if tc.wantID == 0 && decision.Entry != nil {
				t.Fatal(decision)
			}
		})
	}
}

func TestSeasonEpisodePolicyFromSuppliedFacts(t *testing.T) {
	seasons := []model.Season{{Number: 0}, {Number: 2}}
	regular := RegularSeasons(seasons)
	if len(regular) != 1 || regular[0].Number != 2 {
		t.Fatal(regular)
	}
	if !SeasonInputValid(0, []int{1, 2}, seasons) || SeasonInputValid(-1, []int{1}, seasons) || SeasonInputValid(1, []int{1}, seasons) || SeasonInputValid(2, nil, seasons) {
		t.Fatal("invalid season policy")
	}
	checks := CheckEpisodes([]int{1, 2, 2, 3, 0}, []model.Episode{{Number: 1}, {Number: 2}})
	if !checks[0].Exists || checks[0].Duplicate || !checks[2].Duplicate || checks[3].Exists || checks[4].Positive {
		t.Fatal(checks)
	}
}
