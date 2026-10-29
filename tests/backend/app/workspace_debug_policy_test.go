package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

func TestDebugPathInput(t *testing.T) {
	settings := model.Settings{SourceRoot: "/source", TargetRoot: "/source/organized"}
	for _, tc := range []struct {
		name, path, root, wantRoot string
		source, target, valid      bool
	}{
		{"missing source file", "/source/Show/01.mkv", "", "/source", true, false, true},
		{"outside source", "/nas/Show/01.mkv", "", "/", false, false, true},
		{"custom boundary", "/nas/Show/01.mkv", "/nas", "/nas", false, false, true},
		{"target scope", "/source/organized/Film.mkv", "", "/source", true, true, true},
		{"normalized", "/source/other/../Film.mkv", "", "/source", true, false, true},
		{"not ancestor", "/source/Film.mkv", "/different", "", false, false, false},
		{"file as boundary", "/source/Film.mkv", "/source/Film.mkv", "", false, false, false},
		{"relative", "Show/01.mkv", "", "", false, false, false},
		{"windows", `C:\Show\01.mkv`, "", "", false, false, false},
		{"directory", "/source/Show/", "", "", false, false, false},
		{"control", "/source/Show\n01.mkv", "", "", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := debugPathInput(RecognitionDebugRequest{Path: tc.path, Root: tc.root, Mode: "parse"}, settings)
			if (err == nil) != tc.valid {
				t.Fatal(input, err)
			}
			if tc.valid && (input.Root != tc.wantRoot || input.WithinSource != tc.source || input.WithinTarget != tc.target || input.FileAccess != "unverified" || input.FileExistence != "unverified") {
				t.Fatal(input)
			}
		})
	}
}
func TestDebugMatchesProductionAndExplainsDecisions(t *testing.T) {
	for _, tc := range []struct{ name, path, want, rule string }{
		{"unique", "/source/Film.2024.mkv", "ready", "candidate-check"},
		{"ambiguous", "/source/Same.mkv", "review", "match-decision"},
		{"fallback", "/source/Fallback.2020.mkv", "review", "year-fallback"},
		{"id collision", "/source/Film [tmdb-7].mkv", "review", "id-lookup"},
		{"single season", "/source/Show/01.mkv", "ready", "infer-season"},
		{"multiple seasons", "/source/Multi/01.mkv", "review", "infer-season"},
		{"specials", "/source/Show.S00E01.mkv", "ready", "explicit-season"},
		{"invalid episode", "/source/Show.S01E03.mkv", "review", "episode-check"},
		{"unmatched", "/source/Missing.mkv", "unmatched", "match-decision"},
		{"upstream error", "/source/Failure.mkv", "failed", "request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/search/"):
					title := r.URL.Query().Get("query")
					kind := "movie"
					if strings.HasSuffix(r.URL.Path, "/tv") {
						kind = "tv"
					}
					if title == "Failure" {
						w.WriteHeader(401)
						fmt.Fprint(w, `{"private":"sensitive-upstream-body"}`)
						return
					}
					results := "[]"
					if title == "Film" && kind == "movie" {
						results = `[{"id":1,"title":"Film","release_date":"2024-01-01"}]`
					}
					if title == "Same" && kind == "movie" {
						results = `[{"id":1,"title":"Same"},{"id":2,"title":"Same"}]`
					}
					if title == "Fallback" && kind == "movie" && r.URL.Query().Get("year") == "" {
						results = `[{"id":1,"title":"Fallback","release_date":"2019-01-01"}]`
					}
					if (title == "Show" || title == "Multi") && kind == "tv" {
						results = fmt.Sprintf(`[{"id":2,"name":%q}]`, title)
					}
					fmt.Fprintf(w, `{"results":%s}`, results)
				case strings.Contains(r.URL.Path, "/season/"):
					fmt.Fprint(w, `{"episodes":[{"episode_number":1},{"episode_number":2}]}`)
				case strings.HasSuffix(r.URL.Path, "/tv/2"):
					seasons := `[{"season_number":0},{"season_number":1}]`
					if tc.name == "multiple seasons" {
						seasons = `[{"season_number":1},{"season_number":2}]`
					}
					fmt.Fprintf(w, `{"seasons":%s}`, seasons)
				default:
					fmt.Fprint(w, `{"id":7,"title":"Film","name":"Film","overview":"Description"}`)
				}
			}
			factory := func(_ model.Settings, log *trace.Recorder) (matching.Provider, error) {
				c := mockRecognitionClient(t, handler)
				c.Trace = log
				return c, nil
			}
			settings := model.Settings{SourceRoot: "/source", Token: "private-config-token"}
			input, err := debugPathInput(RecognitionDebugRequest{Path: tc.path, Mode: "full"}, settings)
			if err != nil {
				t.Fatal(err)
			}
			result := recognizeDebug(context.Background(), "full", input, settings, factory)
			if result.State != tc.want || !debugHasRule(result, tc.rule) || !debugHasRule(result, "parsed-result") {
				t.Fatalf("%s: %s; %+v", tc.want, result.State, result.Steps)
			}
			client, _ := factory(settings, nil)
			extracted, _ := recognition.DefaultEngine().Extract(context.Background(), recognition.Input{Path: input.NormalizedPath, Root: input.Root}, nil)
			parsed := recognition.ParsedMetadata(extracted.Metadata)
			normal := matching.Item{Metadata: recognition.MetadataFromParsed(parsed), File: model.File{Season: parsed.Season, Episodes: parsed.Episodes}, Candidates: []model.Entry{}}
			memo := matching.NewResolver(client)
			memo.Match(context.Background(), &normal)
			if normal.Entry != nil {
				memo.Validate(context.Background(), &normal)
			}
			if !reflect.DeepEqual(result.Parsed, parsed) || result.State != normal.State || result.Reason != normal.Reason || result.Season != normal.Season || !reflect.DeepEqual(result.Episodes, normal.Episodes) || !reflect.DeepEqual(result.Entry, normal.Entry) || !reflect.DeepEqual(result.Candidates, normal.Candidates) {
				t.Fatalf("debug diverged from production: %+v vs %+v", result, normal)
			}
			raw, _ := json.Marshal(result)
			for _, secret := range []string{"private-config-token", "test-token", "sensitive-upstream-body", "Authorization"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("debug leaked", secret)
				}
			}
		})
	}
}
