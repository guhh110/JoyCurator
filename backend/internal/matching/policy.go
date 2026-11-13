package matching

import (
	"joycurator/internal/model"
)

func RegularSeasons(seasons []model.Season) []model.Season {
	result := []model.Season{}
	for _, season := range seasons {
		if season.Number > 0 {
			result = append(result, season)
		}
	}
	return result
}
func SeasonInputValid(season int, episodes []int, seasons []model.Season) bool {
	return SeasonExists(season, seasons) && len(episodes) > 0 && len(episodes) <= 1000
}
func SeasonExists(number int, seasons []model.Season) bool {
	for _, season := range seasons {
		if season.Number == number {
			return true
		}
	}
	return false
}

type EpisodeCheck struct {
	Number                         int
	Exists, Duplicate, NonNegative bool
	Still                          string
}

func CheckEpisodes(numbers []int, episodes []model.Episode) []EpisodeCheck {
	seen := map[int]bool{}
	checks := make([]EpisodeCheck, 0, len(numbers))
	for _, number := range numbers {
		exists, still := false, ""
		for _, episode := range episodes {
			if episode.Number == number {
				exists, still = true, episode.Still
				break
			}
		}
		checks = append(checks, EpisodeCheck{Number: number, Exists: exists, Duplicate: seen[number], NonNegative: number >= 0, Still: still})
		seen[number] = true
	}
	return checks
}
