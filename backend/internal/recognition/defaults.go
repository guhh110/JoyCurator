package recognition

import "encoding/json"

// DefaultRules is one rule per field. Each script reads the filename and the
// directories itself, so the rules use the relative path as their source.
// Season and episodes run before the media type, which reuses their result.
func DefaultRules() []Rule {
	examples := map[string][]Example{
		"tmdb-id":     {{Path: "/source/Show [tmdb-7]/Season 2/03.mkv", Root: "/source", Expected: json.RawMessage(`7`)}},
		"season":      {{Path: "/Show.S00E01.mkv", Expected: json.RawMessage(`0`)}, {Path: "/source/Show/第二季/07.mkv", Root: "/source", Expected: json.RawMessage(`2`)}},
		"episodes":    {{Path: "/Show.S02E03-E05.mkv", Expected: json.RawMessage(`[3,4,5]`)}, {Path: "/source/The.Matrix.1999.tmdb-603.mkv", Root: "/source", Expected: json.RawMessage(`null`)}},
		"media-type":  {{Path: "/Show.1x03.mkv", Expected: json.RawMessage(`"tv"`)}, {Path: "/The.Matrix.1999.mkv", Expected: json.RawMessage(`null`)}},
		"title":       {{Path: "/[Group] 1917.2019.1080p.mkv", Expected: json.RawMessage(`"1917"`)}},
		"year":        {{Path: "/2001.A.Space.Odyssey.1968.mkv", Expected: json.RawMessage(`1968`)}},
		"release-tag": {{Path: "/Film.2020.1080p-GROUP.mkv", Expected: json.RawMessage(`"GROUP"`)}},
	}
	rules := []Rule{}
	for _, id := range []string{"tmdb-id", "season", "episodes", "media-type", "title", "year", "release-tag"} {
		d, _ := descriptor(id)
		rules = append(rules, Rule{ID: id, Name: d.Name, Description: d.Description, Target: d.Target, Source: RelativePath, Order: (len(rules) + 1) * 10, Enabled: true, Kind: "script", Config: d.Config, Examples: examples[id]})
	}
	return rules
}

var defaultEngine = func() *Engine {
	engine, err := Compile(DefaultRules())
	if err != nil {
		panic(err)
	}
	return engine
}()

func DefaultEngine() *Engine { return defaultEngine }
