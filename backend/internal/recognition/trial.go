package recognition

import (
	"context"
	"encoding/json"
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
func Examples(ctx context.Context, rules []Rule) ([]map[string]any, error) {
	engine, err := Compile(rules)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, rule := range engine.Rules() {
		for _, example := range rule.Examples {
			result, err := engine.Extract(ctx, Input{Path: example.Path, Root: example.Root}, nil)
			if err != nil {
				return nil, err
			}
			var actual any
			for _, step := range result.Steps {
				if step.RuleID == rule.ID {
					actual = step.ExtractedValue
					break
				}
			}
			raw, _ := json.Marshal(actual)
			var av, expected any
			_ = json.Unmarshal(raw, &av)
			_ = json.Unmarshal(example.Expected, &expected)
			items = append(items, map[string]any{"ruleId": rule.ID, "path": example.Path, "expected": expected, "actual": av, "passed": Equal(av, expected)})
		}
	}
	return items, nil
}
