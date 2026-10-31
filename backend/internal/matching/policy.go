package matching

import (
	"sort"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

// Candidate is a query result together with the relaxation used to obtain it.
// It carries facts, never an upstream client or storage handle.
type Candidate struct {
	Entry   model.Entry
	Relaxed bool
}
type Comparison struct{ SameTitle, SameYear, Exact bool }

func CompareCandidate(metadata recognition.Metadata, candidate Candidate) Comparison {
	p := values(metadata)
	sameTitle := recognition.NormalizeTitle(candidate.Entry.Title) == recognition.NormalizeTitle(p.Title) || recognition.NormalizeTitle(candidate.Entry.OriginalTitle) == recognition.NormalizeTitle(p.Title)
	sameYear := p.Year == 0 || p.Year == candidate.Entry.Year
	return Comparison{SameTitle: sameTitle, SameYear: sameYear, Exact: !candidate.Relaxed && sameTitle && sameYear}
}

type MatchDecision struct {
	Entry                      *model.Entry
	Candidates                 []model.Entry
	State, Reason              string
	ExactCount                 int
	IDChecked, IDDisambiguated bool
	Qualified                  []model.Entry
}

// DecideMatch is a pure policy. Querying, retries, memoization and logging belong
// to Resolver, so identical supplied facts always produce the same decision.
func DecideMatch(metadata recognition.Metadata, facts []Candidate) MatchDecision {
	p := values(metadata)
	result := MatchDecision{Candidates: []model.Entry{}}
	exact := []model.Entry{}
	relaxed := false
	for _, fact := range facts {
		result.Candidates = append(result.Candidates, fact.Entry)
		relaxed = relaxed || fact.Relaxed
		if p.TMDBID > 0 || CompareCandidate(metadata, fact).Exact {
			exact = append(exact, fact.Entry)
		}
	}
	if p.TMDBID > 0 && len(exact) > 1 && recognition.NormalizeTitle(p.Title) != "" {
		result.IDChecked = true
		result.Qualified = []model.Entry{}
		for _, entry := range exact {
			if CompareCandidate(metadata, Candidate{Entry: entry}).Exact {
				result.Qualified = append(result.Qualified, entry)
			}
		}
		if len(result.Qualified) == 1 {
			exact = result.Qualified
			result.IDDisambiguated = true
		}
	}
	result.ExactCount = len(exact)
	isExact := func(e model.Entry) bool {
		for _, v := range exact {
			if v.Type == e.Type && v.TMDBID == e.TMDBID {
				return true
			}
		}
		return false
	}
	sort.SliceStable(result.Candidates, func(i, j int) bool { return isExact(result.Candidates[i]) && !isExact(result.Candidates[j]) })
	if len(result.Candidates) > 8 {
		result.Candidates = result.Candidates[:8]
	}
	switch {
	case len(exact) == 1:
		entry := exact[0]
		result.Entry = &entry
		result.State = "ready"
		result.Reason = "片名精确匹配，且没有同等候选"
		if p.Year > 0 {
			result.Reason = "片名与年份精确匹配"
		}
		if p.TMDBID > 0 {
			result.Reason = "TMDB ID 唯一匹配"
			if result.IDDisambiguated {
				result.Reason = "TMDB ID 跨类型重复，已通过片名与已知年份唯一匹配"
			}
		}
	case len(result.Candidates) > 0:
		result.State, result.Reason = "review", "存在同名或模糊候选，请选择作品"
		if relaxed {
			result.Reason = "按年份未找到结果，已放宽年份搜索，请确认"
		}
	default:
		result.State, result.Reason = "unmatched", "未找到匹配作品，可手动搜索"
		if p.Title == "" && p.TMDBID == 0 {
			result.Reason = "无法提取片名或 TMDB ID，请手动选择作品"
		}
	}
	return result
}

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

type metadataValues struct {
	Title, Type  string
	Year, TMDBID int
}

func values(m recognition.Metadata) metadataValues {
	v := metadataValues{}
	if m.Title != nil {
		v.Title = *m.Title
	}
	if m.MediaType != nil {
		v.Type = *m.MediaType
	}
	if m.Year != nil {
		v.Year = *m.Year
	}
	if m.TMDBID != nil {
		v.TMDBID = *m.TMDBID
	}
	return v
}
