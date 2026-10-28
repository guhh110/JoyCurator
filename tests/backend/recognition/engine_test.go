package recognition_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	r "joycurator/internal/recognition"
)

func regexRule(id string, order int, target r.Target, pattern string) r.Rule {
	return r.Rule{ID: id, Name: id, Order: order, Enabled: true, Kind: "regex", Target: target, Source: r.Filename, Config: r.Config{Pattern: pattern, Capture: "value"}, Examples: []r.Example{}}
}
func extract(t *testing.T, rules []r.Rule, path, root string) r.Result {
	t.Helper()
	engine, err := r.Compile(rules)
	if err != nil {
		t.Fatal(err)
	}
	got, err := engine.Extract(context.Background(), r.Input{Path: path, Root: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func TestOrderSingleTargetAndConflict(t *testing.T) {
	first := regexRule("first", 10, r.Season, `S(?P<value>\d+)`)
	later := regexRule("later", 20, r.Season, `E(?P<value>\d+)`)
	got := extract(t, []r.Rule{later, first}, "/Show.S02E03.mkv", "/")
	if got.Metadata.Season == nil || *got.Metadata.Season != 2 || got.Metadata.MediaType != nil || len(got.Metadata.Episodes) != 0 {
		t.Fatal(got.Metadata)
	}
	if got.Steps[0].RuleID != "first" || got.Steps[1].Status != "matched" || got.Steps[1].Applied || got.Steps[1].ExtractedValue != float64(3) {
		t.Fatal(got.Steps)
	}
	later.Overwrite = true
	got = extract(t, []r.Rule{later, first}, "/Show.S02E03.mkv", "/")
	if *got.Metadata.Season != 3 || !got.Steps[1].Applied || got.Steps[1].Before != float64(2) {
		t.Fatal(got)
	}
	first.Order = 30
	later.Overwrite = false
	got = extract(t, []r.Rule{later, first}, "/Show.S02E03.mkv", "/")
	if *got.Metadata.Season != 3 {
		t.Fatal("order ignored")
	}
}
func TestErrorsDoNotEraseMetadataAndAllRulesAreObservable(t *testing.T) {
	rules := []r.Rule{regexRule("good", 10, r.Season, `S(?P<value>\d+)`), regexRule("bad", 20, r.Season, `(?P<value>Show)`), regexRule("miss", 30, r.Year, `Year(?P<value>\d+)`), regexRule("disabled", 40, r.Title, `(?P<value>.+)`), regexRule("episodes", 50, r.Episodes, `E(?P<value>\d+)`)}
	rules[3].Enabled = false
	got := extract(t, rules, "/Show.S00E03.mkv", "/")
	if *got.Metadata.Season != 0 || !reflect.DeepEqual(got.Metadata.Episodes, []int{3}) {
		t.Fatal(got.Metadata)
	}
	for i, want := range []string{"matched", "error", "not_matched", "disabled", "matched"} {
		if got.Steps[i].Status != want {
			t.Fatalf("step %d: %s", i, got.Steps[i].Status)
		}
	}
}
func TestProtectedIDCannotBecomeEpisodeEvenWithOverwrite(t *testing.T) {
	rule := regexRule("bad-id", 0, r.Episodes, `tmdb-(?P<value>\d+)`)
	rule.Overwrite = true
	rules := append([]r.Rule{rule}, r.DefaultRules()...)
	got := extract(t, rules, "/source/The.Matrix.1999.tmdb-603.mkv", "/source")
	if *got.Metadata.TMDBID != 603 || *got.Metadata.Title != "The Matrix" || *got.Metadata.Year != 1999 || got.Metadata.MediaType != nil || len(got.Metadata.Episodes) != 0 || got.Metadata.Season != nil {
		t.Fatal(got.Metadata)
	}
	if got.Steps[0].Status != "error" || got.Steps[0].MatchedText != "tmdb-603" || got.Steps[0].Applied {
		t.Fatal(got.Steps[0])
	}
}
func TestDirectoryBoundaryAndAttempts(t *testing.T) {
	got := extract(t, r.DefaultRules(), "/outside [tmdb-999]/source/Show [tmdb-7]/Season 2/03.mkv", "/outside [tmdb-999]/source")
	if *got.Metadata.TMDBID != 7 || *got.Metadata.Season != 2 || *got.Metadata.Title != "Show" {
		t.Fatal(got.Metadata)
	}
	for _, step := range got.Steps {
		for _, input := range step.Inputs {
			if input == "outside [tmdb-999]" || input == "source" {
				t.Fatal("boundary ignored")
			}
		}
		if step.RuleID == "id-directory" && len(step.Attempts) != 2 {
			t.Fatal(step)
		}
	}
	for _, in := range []r.Input{{Path: "relative.mkv"}, {Path: "/a/file.mkv", Root: "/b"}, {Path: "/a/file.mkv", Root: "/a/file.mkv"}, {Path: "/a/"}, {Path: "/a\x00.mkv"}} {
		if _, err := r.DefaultEngine().Extract(context.Background(), in, nil); err == nil {
			t.Fatal("accepted invalid input", in)
		}
	}
}
func TestConversionsAndMergedEpisodes(t *testing.T) {
	for _, tc := range []struct {
		target r.Target
		value  string
		want   string
	}{{r.Season, "零", `0`}, {r.Season, "二十三", `23`}, {r.Episodes, "1-3,5", `[1,2,3,5]`}, {r.MediaType, "movie", `"movie"`}} {
		rule := regexRule("convert", 1, tc.target, `(?P<value>.+)`)
		got := extract(t, []r.Rule{rule}, "/"+tc.value+".mkv", "/")
		raw, _ := json.Marshal(got.Steps[0].After)
		if string(raw) != tc.want {
			t.Fatalf("%s: %s", tc.value, raw)
		}
	}
	for _, value := range []string{"0", "1,1", "4-2", "1-1001", "10000"} {
		got := extract(t, []r.Rule{regexRule("episodes", 1, r.Episodes, `(?P<value>.+)`)}, "/"+value+".mkv", "/")
		if got.Steps[0].Status != "error" || len(got.Metadata.Episodes) != 0 {
			t.Fatal(value, got)
		}
	}
}
func TestValidationAndImmutableConfiguration(t *testing.T) {
	base := regexRule("one", 10, r.Title, `(?P<value>.+)`)
	for _, mutate := range []func(*r.Rule){func(v *r.Rule) { v.Config.Pattern = "(" }, func(v *r.Rule) { v.Config.Capture = "missing" }, func(v *r.Rule) { v.Target = "titles" }, func(v *r.Rule) { v.Config.Value = json.RawMessage(`"fixed"`) }, func(v *r.Rule) { v.Kind = "builtin"; v.Config = r.Config{Handler: "season-sxe"} }, func(v *r.Rule) { v.Source = "filesystem" }} {
		candidate := base
		mutate(&candidate)
		if _, err := r.Compile([]r.Rule{candidate}); err == nil {
			t.Fatal(candidate)
		}
	}
	other := base
	other.ID = "two"
	if _, err := r.Compile([]r.Rule{base, other}); err == nil {
		t.Fatal("duplicate order")
	}
	other.Order = 20
	other.ID = base.ID
	if _, err := r.Compile([]r.Rule{base, other}); err == nil {
		t.Fatal("duplicate id")
	}
	base.Config.Capture = ""
	base.Config.Value = json.RawMessage(`"fixed"`)
	engine, err := r.Compile([]r.Rule{base})
	if err != nil {
		t.Fatal(err)
	}
	base.Config.Value[1] = 'X'
	copyRules := engine.Rules()
	copyRules[0].Config.Value[1] = 'Y'
	got, err := engine.Extract(context.Background(), r.Input{Path: "/file.mkv"}, nil)
	if err != nil || *got.Metadata.Title != "fixed" || string(got.Steps[0].RuleConfig.Config.Value) != `"fixed"` {
		t.Fatal(got, err)
	}
}
func TestCancellationPreservesCompletedStepsAndSnapshot(t *testing.T) {
	engine, _ := r.Compile(r.DefaultRules())
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	got, err := engine.Extract(ctx, r.Input{Path: "/Show.tmdb-603.S01E02.mkv"}, func(_ context.Context, step r.Step) error {
		count++
		if count == 1 {
			cancel()
			step.RuleConfig.Config.Code = "changed"
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || count != len(r.DefaultRules()) || *got.Metadata.TMDBID != 603 || got.Steps[0].RuleConfig.Config.Code != r.DefaultRules()[0].Config.Code {
		t.Fatal(count, err)
	}
	for _, step := range got.Steps[1:] {
		if step.Status != "skipped" {
			t.Fatal(step)
		}
	}
}
func TestEmptyChainUnknownValues(t *testing.T) {
	got := extract(t, []r.Rule{}, "/Film.mkv", "/")
	raw, _ := json.Marshal(got.Metadata)
	if string(raw) != `{"tmdbId":null,"title":null,"mediaType":null,"year":null,"season":null,"episodes":[],"releaseTag":null}` {
		t.Fatal(string(raw))
	}
}

func TestEpisodeMarkersDoNotAbsorbQualityOrTruncateLargeNumbers(t *testing.T) {
	for _, path := range []string{"/Show.S01E03 1080p.mkv", "/Show.S01E03 2024.mkv"} {
		got := extract(t, r.DefaultRules(), path, "/")
		if !reflect.DeepEqual(got.Metadata.Episodes, []int{3}) {
			t.Fatalf("%s: %+v", path, got.Metadata)
		}
	}
	for _, path := range []string{"/Show.S01E10001.mkv", "/Show.1x10001.mkv"} {
		got := extract(t, r.DefaultRules(), path, "/")
		if len(got.Metadata.Episodes) != 0 {
			t.Fatalf("truncated episode %s: %+v", path, got.Metadata)
		}
	}
}
