package recognition

import "encoding/json"

func DefaultRules() []Rule {
	rules := []Rule{}
	add := func(id string, target Target, source Source, templateID string) {
		d, _ := descriptor(templateID)
		rules = append(rules, Rule{ID: id, Name: d.Name, Description: d.Description, Target: target, Source: source, Order: (len(rules) + 1) * 10, Enabled: true, Kind: "script", Config: d.Config, Examples: []Example{}})
	}
	add("id-filename", TMDBID, Filename, "tmdb-id")
	add("id-directory", TMDBID, Directories, "tmdb-id")
	add("season-sxe", Season, Filename, "season-sxe")
	add("episodes-sxe", Episodes, Filename, "episodes-sxe")
	add("type-sxe", MediaType, Filename, "type-sxe")
	add("season-x", Season, Filename, "season-x")
	add("episodes-x", Episodes, Filename, "episodes-x")
	add("type-x", MediaType, Filename, "type-x")
	add("season-marker", Season, Filename, "season-marker")
	add("type-season-marker", MediaType, Filename, "type-season-marker")
	add("season-folder", Season, Directories, "season-folder")
	add("type-folder", MediaType, Directories, "type-folder")
	add("episodes-number", Episodes, Filename, "episodes-number")
	add("type-number", MediaType, Filename, "type-number")
	add("episodes-trailing", Episodes, Filename, "episodes-trailing")
	add("type-trailing", MediaType, Filename, "type-trailing")
	add("title-filename", Title, Filename, "title-filename")
	add("title-directory", Title, Directories, "title-directory")
	add("year-filename", Year, Filename, "year")
	add("year-directory", Year, Directories, "year")
	add("release-tag", ReleaseTag, Filename, "release-tag")
	for i := range rules {
		switch rules[i].ID {
		case "id-filename":
			rules[i].Examples = []Example{{Path: "/source/The.Matrix.1999.tmdb-603.mkv", Root: "/source", Expected: json.RawMessage(`603`)}}
		case "season-sxe":
			rules[i].Examples = []Example{{Path: "/Show.S00E01.mkv", Expected: json.RawMessage(`0`)}}
		case "episodes-sxe":
			rules[i].Examples = []Example{{Path: "/Show.S02E03-E05.mkv", Expected: json.RawMessage(`[3,4,5]`)}}
		case "episodes-trailing":
			rules[i].Examples = []Example{{Path: "/source/The.Matrix.1999.tmdb-603.mkv", Expected: json.RawMessage(`null`)}}
		case "title-filename":
			rules[i].Examples = []Example{{Path: "/[Group] 1917.2019.1080p.mkv", Expected: json.RawMessage(`"1917"`)}}
		case "season-folder":
			rules[i].Examples = []Example{{Path: "/source/Show/第二季/07.mkv", Root: "/source", Expected: json.RawMessage(`2`)}}
		}
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
