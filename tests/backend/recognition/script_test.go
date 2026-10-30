package recognition_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	r "joycurator/internal/recognition"
)

func scriptRule(id string, target r.Target, code string) r.Rule {
	return r.Rule{ID: id, Name: id, Order: 1, Enabled: true, Kind: "script", Target: target, Source: r.Filename, Config: r.Config{Code: code}, Examples: []r.Example{}}
}
func TestDefaultRulesAreEditableScriptsWithAutomaticRegexTrace(t *testing.T) {
	for _, rule := range r.DefaultRules() {
		if rule.Kind != "script" || rule.Config.Code == "" {
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
		if step.RuleID == "episodes" {
			for _, event := range step.Attempts[0].Trace {
				if event.Operation == "reSearch" && event.Line > 0 && event.Function != "" {
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
	rule := scriptRule("editable", r.Title, "function extract(ctx) {\n  trace(\"original\", ctx.text);\n  return \"first\";\n}")
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
	first := scriptRule("bad", r.Title, "function extract(ctx) {\n  trace(\"before failure\", ctx.text);\n  throw new Error(\"explanation\");\n}")
	second := regexRule("good-year", 2, r.Year, `(?P<value>2024)`)
	got := extract(t, []r.Rule{first, second}, "/Film.2024.mkv", "/")
	if got.Metadata.Title != nil || *got.Metadata.Year != 2024 || got.Steps[0].Status != "error" || !strings.Contains(got.Steps[0].Reason, "bad.js:3") || len(got.Steps[0].Attempts[0].Trace) != 3 {
		t.Fatal(got)
	}
}
func TestScriptCompileValidationAndSingleTargetContract(t *testing.T) {
	for _, code := range []string{"", "function extract( {", "function other(ctx) { return null; }", "function extract() { return null; }", "async function extract(ctx) { return null; }", "function* extract(ctx) { yield 1; }", "function extract(ctx, extra) { return null; }"} {
		if _, err := r.Compile([]r.Rule{scriptRule("invalid", r.Title, code)}); err == nil {
			t.Fatal("invalid script accepted", code)
		}
	}
	for _, code := range []string{"function extract(ctx) { return {title: \"other field\"}; }", "function extract(ctx) { return {value: \"Title\", season: 2}; }", "function extract(ctx) { return 123; }", "function extract(ctx) { ctx.metadata.season = 3; return \"Title\"; }"} {
		got := extract(t, []r.Rule{scriptRule("invalid-result", r.Title, code)}, "/Film.mkv", "/")
		if got.Metadata.Title != nil || got.Metadata.Season != nil || got.Steps[0].Status != "error" {
			t.Fatal(got)
		}
	}
	rule := scriptRule("mixed", r.Title, "function extract(ctx) { return null; }")
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
	rule := scriptRule("safe-episode", r.Episodes, "function extract(ctx) { trace(\"protected input\", [ctx.text, ctx.filename, ctx.directories, ctx.metadata.tmdbId]); const m = reSearch(\"tmdb-(\\\\d+)\", ctx.text); return m ? Number(m.groups[1]) : null; }")
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
	clean.Config.SourceCode = "function extract(ctx) { trace(\"custom cleaner\", ctx.text); return \"Show\"; }"
	got = extract(t, []r.Rule{clean}, "/Other.mkv", "/")
	if *got.Metadata.Title != "Show" {
		t.Fatal(got)
	}
}
func TestScriptBudgetAndBoundedLogs(t *testing.T) {
	loop := scriptRule("bounded", r.Title, "function extract(ctx) { while (true) {} }")
	next := regexRule("next", 2, r.Year, `(?P<value>2024)`)
	got := extract(t, []r.Rule{loop, next}, "/Film.2024.mkv", "/")
	if got.Steps[0].Status != "error" || !strings.Contains(got.Steps[0].Reason, "执行") || *got.Metadata.Year != 2024 {
		t.Fatal(got)
	}
	log := scriptRule("logs", r.Title, "function extract(ctx) { for (let i = 0; i < 300; i++) trace(\"step\", i); return \"Title\"; }")
	got = extract(t, []r.Rule{log}, "/Film.mkv", "/")
	events := got.Steps[0].Attempts[0].Trace
	if *got.Metadata.Title != "Title" || len(events) != 129 || events[128].Operation != "limit" {
		t.Fatal(len(events), got.Steps[0].Reason)
	}
}
func TestScriptInvocationsDoNotShareGlobals(t *testing.T) {
	rule := scriptRule("isolated", r.Title, "const values = []; function extract(ctx) { values.push(ctx.text); return String(values.length); }")
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

func TestJavaScriptRuntimeBoundaries(t *testing.T) {
	for _, code := range []string{
		`function extract(ctx) { return require("fs"); }`,
		`function extract(ctx) { return fetch("https://example.com"); }`,
		`function extract(ctx) { return process.env.TOKEN; }`,
		`function extract(ctx) { return Promise.resolve("Title"); }`,
		`function extract(ctx) { ctx.params.values.push("changed"); return "Title"; }`,
		`function extract(ctx) { return NaN; }`,
		`function extract(ctx) { return Infinity; }`,
		`function extract(ctx) { return "x".repeat(32769); }`,
		`function extract(ctx) { return {get value() { throw new Error("getter failure"); }}; }`,
		`function extract(ctx) { return {get value() { while (true) {} }}; }`,
		`function extract(ctx) { const v = {}; v.value = v; return v; }`,
	} {
		t.Run(code, func(t *testing.T) {
			rule := scriptRule("boundary", r.Title, code)
			rule.Config.Params = json.RawMessage(`{"values":[]}`)
			got := extract(t, []r.Rule{rule}, "/Film.mkv", "/")
			if got.Steps[0].Status != "error" || got.Metadata.Title != nil {
				t.Fatal(got)
			}
		})
	}
}

func TestJavaScriptNativeRegexAndUnicodeOffsets(t *testing.T) {
	rule := scriptRule("native", r.Season, `function extract(ctx) {
  const match = /S(\d+)E\d+/i.exec(ctx.text);
  trace("native regex", {matched: !!match});
  return match ? Number(match[1]) : null;
 }`)
	got := extract(t, []r.Rule{rule}, "/中文.S00E02.mkv", "/")
	if got.Metadata.Season == nil || *got.Metadata.Season != 0 {
		t.Fatal(got)
	}
	for _, path := range []string{"/流浪地球.2019.mkv", "/𠮷祥.2024.mkv"} {
		got = extract(t, r.DefaultRules(), path, "/")
		title := strings.Split(strings.TrimPrefix(path, "/"), ".")[0]
		if got.Metadata.Title == nil || *got.Metadata.Title != title {
			t.Fatal(got.Metadata)
		}
	}
}

func TestJavaScriptCancellationPreservesTrace(t *testing.T) {
	rule := scriptRule("cancel", r.Title, `function extract(ctx) { trace("started"); while(true) {} }`)
	next := regexRule("next", 2, r.Year, `(?P<value>2024)`)
	engine, err := r.Compile([]r.Rule{rule, next})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(30*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()
	got, err := engine.Extract(ctx, r.Input{Path: "/Film.2024.mkv"}, nil)
	if !errors.Is(err, context.Canceled) || len(got.Steps) != 2 || got.Steps[1].Status != "skipped" {
		t.Fatal(got, err)
	}
	events := got.Steps[0].Attempts[0].Trace
	if len(events) < 3 || events[1].Message != "started" {
		t.Fatal(events)
	}
}

// The default chain is one rule per field, ordered so the media type can reuse
// the season and episodes already found, and every bundled example must pass.
func TestDefaultChainHasOneRulePerField(t *testing.T) {
	rules := r.DefaultRules()
	seen := map[r.Target]int{}
	for _, rule := range rules {
		if rule.Source != r.RelativePath {
			t.Fatalf("%s source %s", rule.ID, rule.Source)
		}
		if rule.Target == r.MediaType && (seen[r.Season] == 0 || seen[r.Episodes] == 0) {
			t.Fatal("media type runs before season or episodes")
		}
		seen[rule.Target]++
	}
	if len(rules) != len(r.Targets()) {
		t.Fatalf("%d default rules for %d fields", len(rules), len(r.Targets()))
	}
	for _, target := range r.Targets() {
		if seen[target] != 1 {
			t.Fatalf("%s has %d default rules", target, seen[target])
		}
	}
	results, err := r.Examples(context.Background(), rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < len(rules) {
		t.Fatalf("only %d examples", len(results))
	}
	for _, result := range results {
		if result["passed"] != true {
			t.Fatalf("example failed: %+v", result)
		}
	}
}

// The title script used to repeat the same cleanup; keep its trace readable.
func TestTitleRuleTraceStaysShort(t *testing.T) {
	for _, path := range []string{"/source/[Group] Show.Name.S02E03.1080p.WEB-DL.mkv", "/source/某剧/第二季/07.mkv", "/source/The.Matrix.1999.tmdb-603.mkv"} {
		got := extract(t, r.DefaultRules(), path, "/source")
		for _, step := range got.Steps {
			if step.Target != r.Title {
				continue
			}
			events := 0
			for _, attempt := range step.Attempts {
				events += len(attempt.Trace)
			}
			t.Logf("%s => %d events", path, events)
			if events > 12 {
				t.Fatalf("%s: title trace has %d events", path, events)
			}
		}
	}
}
