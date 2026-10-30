package recognition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (a *Service) Test(ctx context.Context, in TestRequest, source string) (any, error) {
	engine := a.Current()
	if in.Rules != nil {
		var err error
		engine, err = Compile(*in.Rules)
		if err != nil {
			return nil, err
		}
	}
	root := Root(in.Path, in.Root, source)
	result, err := engine.Extract(ctx, Input{Path: in.Path, Root: root}, nil)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": in.Path, "root": root, "metadata": result.Metadata, "steps": result.Steps}, nil
}
func Examples(ctx context.Context, rules []Rule) ([]ExampleResult, error) {
	engine, err := Compile(rules)
	if err != nil {
		return nil, err
	}
	items := []ExampleResult{}
	for _, rule := range engine.Rules() {
		for _, example := range rule.Examples {
			result, err := engine.Extract(ctx, Input{Path: example.Path, Root: example.Root}, nil)
			if err != nil {
				return nil, err
			}
			var actual any
			failures := []string{}
			for _, step := range result.Steps {
				if step.RuleID == rule.ID {
					actual = step.ExtractedValue
				}
				// Every example runs the full chain; an error in another rule can
				// also invalidate the metadata this rule relies on.
				if step.Status == "error" {
					failures = append(failures, fmt.Sprintf("规则 %s：%s", step.RuleID, step.Reason))
				}
			}
			raw, _ := json.Marshal(actual)
			var av, expected any
			_ = json.Unmarshal(raw, &av)
			_ = json.Unmarshal(example.Expected, &expected)
			items = append(items, ExampleResult{RuleID: rule.ID, Path: example.Path, Expected: expected, Actual: av, Passed: len(failures) == 0 && Equal(av, expected), Error: strings.Join(failures, "；")})
		}
	}
	return items, nil
}
