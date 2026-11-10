package recognition

import (
	"context"
	"encoding/json"
)

func (a *Service) Test(ctx context.Context, in TestRequest, source string) (any, error) {
	engine := a.Current()
	if in.Pipeline != nil {
		var err error
		engine, err = Compile(*in.Pipeline)
		if err != nil {
			return nil, err
		}
	}
	if in.SourceBoundary == "" && len(in.Paths) > 0 {
		in.SourceBoundary = Root(in.Paths[0], "", source)
	}
	if in.ContextRoot == "" {
		in.ContextRoot = in.SourceBoundary
	}
	result, err := engine.ExtractBatch(ctx, in.BatchInput, nil)
	if err != nil && len(result.Steps) > 0 {
		result.Error = err.Error()
		return result, nil
	}
	return result, err
}
func Examples(ctx context.Context, p Pipeline) ([]ExampleResult, error) {
	engine, err := Compile(p)
	if err != nil {
		return nil, err
	}
	out := []ExampleResult{}
	for _, r := range engine.Rules().Rules() {
		for _, ex := range r.Examples {
			root := ex.Root
			if root == "" {
				root = "/"
			}
			targets := ex.Targets
			if len(targets) == 0 {
				targets = ex.Paths
			}
			result, err := engine.ExtractBatch(ctx, BatchInput{ContextRoot: root, SourceBoundary: root, Paths: ex.Paths, Targets: targets}, nil)
			var actual any
			if r.Stage == "resolve" {
				actual = result.Files
			} else {
				values := []any{}
				for _, s := range result.Steps {
					if s.RuleID == r.ID {
						values = append(values, s.ExtractedValue)
					}
				}
				actual = values
			}
			var expected any
			_ = json.Unmarshal(ex.Expected, &expected)
			raw, _ := json.Marshal(actual)
			_ = json.Unmarshal(raw, &actual)
			message := ""
			if err != nil {
				message = err.Error()
			}
			out = append(out, ExampleResult{RuleID: r.ID, Paths: ex.Paths, Actual: actual, Expected: expected, Passed: err == nil && Equal(actual, expected), Error: message})
		}
	}
	return out, nil
}
