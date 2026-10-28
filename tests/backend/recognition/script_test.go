package recognition_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	r "joycurator/internal/recognition"
)

func scriptRule(id string, target r.Target, code string) r.Rule {
	return r.Rule{ID: id, Name: id, Order: 1, Enabled: true, Kind: "script", Target: target, Source: r.Filename, Config: r.Config{Code: code}, Examples: []r.Example{}}
}
func TestDefaultRulesAreEditableScriptsWithAutomaticRegexTrace(t *testing.T) {
	for _, rule := range r.DefaultRules() {
		if rule.Kind != "script" || rule.Config.Code == "" || rule.Config.Handler != "" {
			t.Fatal("opaque default", rule.ID)
		}
	}
	for _, template := range r.Catalog() {
		if template.Config.Code == "" {
			t.Fatal("template lacks code", template.ID)
		}
	}
	got := extract(t, r.DefaultRules(), "/Show.S02E03-E05.mkv", "/")
	if !reflect.DeepEqual(got.Metadata.Episodes, []int{3, 4, 5}) {
		t.Fatal(got.Metadata)
	}
	found := false
	for _, step := range got.Steps {
		if step.RuleID == "episodes-sxe" {
			for _, event := range step.Attempts[0].Trace {
				if event.Operation == "re_search" && event.Line > 0 && event.Function == "extract" {
					found = true
				}
				if event.Line < 0 {
					t.Fatal(event)
				}
			}
		}
	}
	if !found {
		t.Fatal("missing automatic regex call and source position")
	}
}
func TestScriptEditingChangesOutputAndSnapshot(t *testing.T) {
	rule := scriptRule("editable", r.Title, "def extract(ctx):\n    trace(\"original\", ctx.text)\n    return \"first\"\n")
	original := extract(t, []r.Rule{rule}, "/Any.mkv", "/")
	rule.Config.Code = strings.ReplaceAll(rule.Config.Code, "first", "second")
	next := extract(t, []r.Rule{rule}, "/Any.mkv", "/")
	if *original.Metadata.Title != "first" || *next.Metadata.Title != "second" || !strings.Contains(original.Steps[0].RuleConfig.Config.Code, "first") {
		t.Fatal(original.Metadata, next.Metadata)
	}
	if trace := original.Steps[0].Attempts[0].Trace; len(trace) != 3 || trace[1].Line != 2 || trace[1].Message != "original" {
		t.Fatal(trace)
	}
}
func TestScriptErrorsKeepTraceAndContinue(t *testing.T) {
	first := scriptRule("bad", r.Title, "def extract(ctx):\n    trace(\"before failure\", ctx.text)\n    fail(\"explanation\")\n")
	second := regexRule("good-year", 2, r.Year, `(?P<value>2024)`)
	got := extract(t, []r.Rule{first, second}, "/Film.2024.mkv", "/")
	if got.Metadata.Title != nil || *got.Metadata.Year != 2024 || got.Steps[0].Status != "error" || !strings.Contains(got.Steps[0].Reason, "bad.star:3") || len(got.Steps[0].Attempts[0].Trace) != 3 {
		t.Fatal(got)
	}
}
func TestScriptCompileValidationAndSingleTargetContract(t *testing.T) {
	for _, code := range []string{"", "def extract(:", "def other(ctx):\n    return None", "def extract():\n    return None", "load(\"file\", \"secret\")\ndef extract(ctx):\n    return secret", "def extract(ctx):\n    return open(\"/private\")", "def extract(ctx):\n    return getenv(\"TOKEN\")"} {
		if _, err := r.Compile([]r.Rule{scriptRule("invalid", r.Title, code)}); err == nil {
			t.Fatal("invalid script accepted", code)
		}
	}
	for _, code := range []string{"def extract(ctx):\n    return {\"title\": \"other field\"}", "def extract(ctx):\n    return {\"value\": \"Title\", \"season\": 2}", "def extract(ctx):\n    return 123", "def extract(ctx):\n    ctx.metadata[\"season\"] = 3\n    return \"Title\""} {
		got := extract(t, []r.Rule{scriptRule("invalid-result", r.Title, code)}, "/Film.mkv", "/")
		if got.Metadata.Title != nil || got.Metadata.Season != nil || got.Steps[0].Status != "error" {
			t.Fatal(got)
		}
	}
	rule := scriptRule("mixed", r.Title, "def extract(ctx):\n    return None")
	rule.Config.Pattern = "."
	if _, err := r.Compile([]r.Rule{rule}); err == nil {
		t.Fatal("mixed configuration accepted")
	}
	rule.Config.Pattern = ""
	rule.Config.Params = json.RawMessage(`[]`)
	if _, err := r.Compile([]r.Rule{rule}); err == nil {
		t.Fatal("non-object parameters accepted")
	}
}
func TestScriptProtectionAndEditableSourceCleaner(t *testing.T) {
	rule := scriptRule("safe-episode", r.Episodes, "def extract(ctx):\n    trace(\"protected input\", [ctx.text, ctx.filename, ctx.directories, ctx.metadata.get(\"tmdbId\")])\n    m = re_search(r\"tmdb-(\\d+)\", ctx.text)\n    return int(m[\"groups\"][1]) if m else None\n")
	rule.Order = 0
	got := extract(t, append([]r.Rule{rule}, r.DefaultRules()...), "/source/Show.tmdb-603.mkv", "/source")
	if len(got.Metadata.Episodes) != 0 || got.Steps[0].Status != "not_matched" || got.Steps[0].Attempts[0].Trace[0].Operation != "protect" {
		t.Fatal(got.Steps[0])
	}
	clean := regexRule("source", 1, r.Title, `(?P<value>Show)`)
	clean.Source = r.CleanFilename
	got = extract(t, []r.Rule{clean}, "/[Group] Show.1080p.mkv", "/")
	if *got.Metadata.Title != "Show" || got.Steps[0].RuleConfig.Config.SourceCode == "" || len(got.Steps[0].Attempts[0].Trace) == 0 || got.Steps[0].Attempts[0].Trace[0].Phase != "source" {
		t.Fatal(got)
	}
	clean.Config.SourceCode = "def extract(ctx):\n    trace(\"custom cleaner\", ctx.text)\n    return \"Show\""
	got = extract(t, []r.Rule{clean}, "/Other.mkv", "/")
	if *got.Metadata.Title != "Show" {
		t.Fatal(got)
	}
}
func TestScriptBudgetAndBoundedLogs(t *testing.T) {
	loop := scriptRule("bounded", r.Title, "def extract(ctx):\n    count = 0\n    for i in range(10000000):\n        count += i\n    return str(count)")
	next := regexRule("next", 2, r.Year, `(?P<value>2024)`)
	got := extract(t, []r.Rule{loop, next}, "/Film.2024.mkv", "/")
	if got.Steps[0].Status != "error" || !strings.Contains(got.Steps[0].Reason, "执行") || *got.Metadata.Year != 2024 {
		t.Fatal(got)
	}
	log := scriptRule("logs", r.Title, "def extract(ctx):\n    for i in range(300):\n        trace(\"step\", i)\n    return \"Title\"")
	got = extract(t, []r.Rule{log}, "/Film.mkv", "/")
	events := got.Steps[0].Attempts[0].Trace
	if *got.Metadata.Title != "Title" || len(events) != 129 || events[128].Operation != "limit" {
		t.Fatal(len(events), got.Steps[0].Reason)
	}
}
func TestScriptInvocationsDoNotShareGlobals(t *testing.T) {
	rule := scriptRule("isolated", r.Title, "values = []\ndef extract(ctx):\n    values.append(ctx.text)\n    return str(len(values))")
	engine, err := r.Compile([]r.Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := engine.Extract(context.Background(), r.Input{Path: "/Film.mkv"}, nil)
			if err != nil || result.Metadata.Title == nil || *result.Metadata.Title != "1" {
				t.Errorf("shared script state: %+v %v", result.Metadata, err)
			}
		}()
	}
	group.Wait()
}
func TestLegacyRulesMigrateWithoutChangingUserChoices(t *testing.T) {
	legacy := r.Rule{ID: "custom-id", Name: "custom name", Description: "keep", Target: r.Year, Source: r.Directories, Kind: "builtin", Order: 71, Enabled: false, Overwrite: true, Config: r.Config{Handler: "year", Params: json.RawMessage(`{"min":1980,"max":2030}`)}, Examples: []r.Example{{Path: "/Film.2024.mkv", Expected: json.RawMessage(`2024`)}}}
	migrated, changed, err := r.MigrateLegacyRules([]r.Rule{legacy})
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	got := migrated[0]
	if got.Kind != "script" || got.ID != legacy.ID || got.Name != legacy.Name || got.Source != legacy.Source || got.Order != 71 || got.Enabled || !got.Overwrite || got.Config.Handler != "" || got.Config.Code == "" || !reflect.DeepEqual(got.Examples, legacy.Examples) {
		t.Fatal(got)
	}
	if _, err := r.Compile(migrated); err != nil {
		t.Fatal(err)
	}
	if legacy.Config.Code != "" || legacy.Kind != "builtin" {
		t.Fatal("migration mutated input")
	}
	repeated, changed, err := r.MigrateLegacyRules(migrated)
	if err != nil || changed || !reflect.DeepEqual(repeated, migrated) {
		t.Fatal(changed, err)
	}
}
