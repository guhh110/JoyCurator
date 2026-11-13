package matching

import (
	"testing"

	"joycurator/internal/model"
)

func TestSeasonEpisodePolicyFromSuppliedFacts(t *testing.T) {
	seasons := []model.Season{{Number: 0}, {Number: 2}}
	regular := RegularSeasons(seasons)
	if len(regular) != 1 || regular[0].Number != 2 {
		t.Fatal(regular)
	}
	if !SeasonInputValid(0, []int{1, 2}, seasons) || SeasonInputValid(-1, []int{1}, seasons) || SeasonInputValid(1, []int{1}, seasons) || SeasonInputValid(2, nil, seasons) {
		t.Fatal("invalid season policy")
	}
	checks := CheckEpisodes([]int{1, 2, 2, 3, 0, 0, -1}, []model.Episode{{Number: 0}, {Number: 1}, {Number: 2}, {Number: -1}})
	if !checks[0].Exists || checks[0].Duplicate || !checks[2].Duplicate || checks[3].Exists || !checks[4].NonNegative || !checks[4].Exists || !checks[5].Duplicate || checks[6].NonNegative {
		t.Fatal(checks)
	}
	if CheckEpisodes([]int{0}, []model.Episode{{Number: 1}})[0].Exists {
		t.Fatal("E0 must still exist in TMDB")
	}
}
