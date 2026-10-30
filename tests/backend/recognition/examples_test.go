package recognition_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	r "joycurator/internal/recognition"
)

func TestExamplesRejectExecutionErrorsEvenWithEqualValues(t *testing.T) {
	for _, tc := range []struct {
		name, code, sourceCode, expected, message string
	}{
		{"throw", `function extract(ctx) { throw new Error("broken script"); }`, "", `null`, "broken script"},
		{"timeout", `function extract(ctx) { while (true) {} }`, "", `null`, "250 ms"},
		{"invalid result", `function extract(ctx) { return { title: "Film" }; }`, "", `null`, "不支持结果字段"},
		{"source error", `function extract(ctx) { return null; }`, `function extract(ctx) { throw new Error("broken cleaner"); }`, `null`, "broken cleaner"},
		{"source type", `function extract(ctx) { return null; }`, `function extract(ctx) { return null; }`, `null`, "来源清理脚本必须返回字符串"},
		{"conversion", `function extract(ctx) { return "invalid"; }`, "", `null`, "year 提取值类型或范围无效"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := scriptRule("bad", r.Year, tc.code)
			if tc.sourceCode != "" {
				rule.Source, rule.Config.SourceCode = r.CleanFilename, tc.sourceCode
			}
			rule.Examples = []r.Example{{Path: "/Film.mkv", Expected: json.RawMessage(tc.expected)}}
			got, err := r.Examples(context.Background(), []r.Rule{rule})
			if err != nil || len(got) != 1 {
				t.Fatalf("results %v, error %v", got, err)
			}
			if got[0].Passed || !strings.Contains(got[0].Error, tc.message) || !strings.Contains(got[0].Error, "bad") {
				t.Fatalf("execution error passed or lost diagnostic: %+v", got[0])
			}
		})
	}
}

func TestExamplesRejectErrorsElsewhereInChainAndKeepResults(t *testing.T) {
	for _, failingOrder := range []int{0, 2} {
		bad := scriptRule("other", r.Season, `function extract(ctx) { throw new Error("bad season"); }`)
		bad.Order = failingOrder
		checked := scriptRule("checked", r.Title, `function extract(ctx) { return "Film"; }`)
		checked.Examples = []r.Example{{Path: "/Film.mkv", Expected: json.RawMessage(`"Film"`)}, {Path: "/Other.mkv", Expected: json.RawMessage(`null`)}}
		got, err := r.Examples(context.Background(), []r.Rule{bad, checked})
		if err != nil || len(got) != 2 {
			t.Fatalf("results %v, error %v", got, err)
		}
		for _, result := range got {
			if result.Passed || result.Actual != "Film" || !strings.Contains(result.Error, "other") {
				t.Fatalf("full-chain error was ignored: %+v", result)
			}
		}
	}
}

func TestExamplesKeepNormalMissMismatchAndDisabledSemantics(t *testing.T) {
	rule := scriptRule("year", r.Year, `function extract(ctx) { return ctx.text === "Hit" ? 2024 : null; }`)
	rule.Examples = []r.Example{
		{Path: "/Hit.mkv", Expected: json.RawMessage(`2024`)},
		{Path: "/Miss.mkv", Expected: json.RawMessage(`null`)},
		{Path: "/Hit.mkv", Expected: json.RawMessage(`2023`)},
	}
	got, err := r.Examples(context.Background(), []r.Rule{rule})
	if err != nil || len(got) != 3 {
		t.Fatalf("results %v, error %v", got, err)
	}
	for i, passed := range []bool{true, true, false} {
		if got[i].Passed != passed || got[i].Error != "" {
			t.Fatal(got[i])
		}
	}
	rule.Enabled = false
	rule.Examples = rule.Examples[1:2]
	got, err = r.Examples(context.Background(), []r.Rule{rule})
	if err != nil || !got[0].Passed || got[0].Actual != nil || got[0].Error != "" {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Examples(ctx, []r.Rule{rule}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

func TestExamplesHTTPReportsErrorWithoutSaving(t *testing.T) {
	s := newService(t)
	before := decodeResponse[struct{ Rules []r.Rule }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	count := s.history(t).Total
	rule := scriptRule("broken", r.Title, `function extract(ctx) { throw new Error("sample failure"); }`)
	rule.Examples = []r.Example{{Path: "/Film.mkv", Expected: json.RawMessage(`null`)}}
	got := decodeResponse[[]r.ExampleResult](t, s.call(t, "POST", "/api/extraction/examples", map[string]any{"rules": []r.Rule{rule}}, 200))
	if len(got) != 1 || got[0].Passed || got[0].Actual != nil || got[0].Expected != nil || !strings.Contains(got[0].Error, "sample failure") {
		t.Fatal(got)
	}
	after := decodeResponse[struct{ Rules []r.Rule }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	if !reflect.DeepEqual(before, after) || s.history(t).Total != count {
		t.Fatal("sample validation changed saved rules or history")
	}
}
