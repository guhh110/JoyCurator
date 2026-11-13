package recognition_test

import (
	"context"
	"encoding/json"
	"fmt"
	r "joycurator/internal/recognition"
	"runtime"
	"testing"
)

func TestExecutionPreservesFrozenCandidatesAndSuggestions(t *testing.T) {
	var pipeline r.Pipeline
	if err := json.Unmarshal(readCorpusFile(t, "execution-baseline-pipeline.json"), &pipeline); err != nil {
		t.Fatal(err)
	}
	oldRules, err := r.Compile(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	var samples []struct {
		Input     r.BatchInput   `json:"input"`
		Documents []r.Document   `json:"documents"`
		Files     []r.FileResult `json:"files"`
		Tree      []r.TreeNode   `json:"tree"`
	}
	if err = json.Unmarshal(readCorpusFile(t, "execution-baseline-results.json"), &samples); err != nil {
		t.Fatal(err)
	}
	same := func(a, b any) bool {
		left, _ := json.Marshal(a)
		right, _ := json.Marshal(b)
		return string(left) == string(right)
	}
	previous := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(previous)
	for _, workers := range []int{1, 4} {
		runtime.GOMAXPROCS(workers)
		t.Run(fmt.Sprintf("workers-%d", workers), func(t *testing.T) {
			for i, sample := range samples {
				for name, engine := range map[string]*r.Engine{"saved-rules": oldRules, "new-defaults": r.DefaultEngine()} {
					light, err := engine.Snapshot(context.Background(), sample.Input)
					if err != nil || !same(light.Documents, sample.Documents) || !same(light.Files, sample.Files) || !same(light.Tree, sample.Tree) {
						t.Fatalf("sample %d %s changed evidence: %v", i, name, err)
					}
					full, err := engine.ExtractBatch(context.Background(), sample.Input, nil)
					if err != nil || !same(full.Documents, sample.Documents) || !same(full.Files, sample.Files) || !same(full.Tree, sample.Tree) {
						t.Fatalf("sample %d %s debug changed evidence: %v", i, name, err)
					}
				}
			}

		})
	}
}
