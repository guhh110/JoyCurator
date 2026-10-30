package recognition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	r "joycurator/internal/recognition"
)

type pathSample struct {
	ID       string                     `json:"id"`
	Category string                     `json:"category"`
	Path     string                     `json:"path"`
	Root     string                     `json:"root"`
	Reason   string                     `json:"reason"`
	Expected map[string]json.RawMessage `json:"expected"`
	Errors   map[string]string          `json:"errors"`
}

func readPathSamples(data []byte) ([]pathSample, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var samples []pathSample
	if err := decoder.Decode(&samples); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected one JSON array, trailing data: %v", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("empty corpus")
	}
	ids, inputs := map[string]bool{}, map[string]bool{}
	ruleIDs := map[string]bool{}
	for _, rule := range r.DefaultRules() {
		ruleIDs[rule.ID] = true
	}
	for _, sample := range samples {
		if sample.ID == "" || sample.Category == "" || strings.TrimSpace(sample.Reason) == "" {
			return nil, fmt.Errorf("sample needs id, category and reason: %q", sample.ID)
		}
		input := sample.Root + "\x00" + sample.Path
		if ids[sample.ID] || inputs[input] {
			return nil, fmt.Errorf("duplicate sample: %s", sample.ID)
		}
		ids[sample.ID], inputs[input] = true, true
		if !filepath.IsAbs(sample.Path) || !filepath.IsAbs(sample.Root) {
			return nil, fmt.Errorf("sample %s requires absolute path and explicit root", sample.ID)
		}
		if len(sample.Expected) != len(r.Targets()) || sample.Errors == nil {
			return nil, fmt.Errorf("sample %s requires all metadata fields and an explicit errors object", sample.ID)
		}
		for _, target := range r.Targets() {
			raw, ok := sample.Expected[string(target)]
			if !ok || (target == r.Episodes && string(raw) == "null") {
				return nil, fmt.Errorf("sample %s has missing/invalid %s", sample.ID, target)
			}
		}
		raw, _ := json.Marshal(sample.Expected)
		var metadata r.Metadata
		if err := json.Unmarshal(raw, &metadata); err != nil {
			return nil, fmt.Errorf("sample %s metadata: %w", sample.ID, err)
		}
		for id, message := range sample.Errors {
			if !ruleIDs[id] || strings.TrimSpace(message) == "" {
				return nil, fmt.Errorf("sample %s has invalid expected error %q", sample.ID, id)
			}
		}
	}
	return samples, nil
}

func loadPathSamples(t *testing.T) []pathSample {
	t.Helper()
	samples, err := readPathSamples(readCorpusFile(t, "paths.json"))
	if err != nil {
		t.Fatal(err)
	}
	return samples
}

func readCorpusFile(t *testing.T, name string) []byte {
	t.Helper()
	// Go's overlay maps tests into backend/internal/recognition, but fixtures
	// remain under tests/. Resolve from the package working directory.
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "backend", "recognition", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func sampleDifferences(sample pathSample, got r.Result) map[string]string {
	differences := map[string]string{}
	raw, _ := json.Marshal(got.Metadata)
	var actual map[string]json.RawMessage
	_ = json.Unmarshal(raw, &actual)
	for _, target := range r.Targets() {
		key := string(target)
		var before, after any
		_ = json.Unmarshal(sample.Expected[key], &before)
		_ = json.Unmarshal(actual[key], &after)
		if !reflect.DeepEqual(before, after) {
			differences[key] = fmt.Sprintf("expected %s, actual %s", sample.Expected[key], actual[key])
		}
	}
	errors := map[string]string{}
	for _, step := range got.Steps {
		if step.Status == "error" {
			errors[step.RuleID] = step.Reason
		}
	}
	if len(errors) != len(sample.Errors) {
		differences["errors"] = fmt.Sprintf("expected %v, actual %v", sample.Errors, errors)
	}
	for id, message := range sample.Errors {
		if !strings.Contains(errors[id], message) {
			differences["errors"] = fmt.Sprintf("expected %v, actual %v", sample.Errors, errors)
		}
	}
	return differences
}

func TestRecognitionPathCorpus(t *testing.T) {
	for _, sample := range loadPathSamples(t) {
		t.Run(sample.Category+"/"+sample.ID, func(t *testing.T) {
			got, err := r.DefaultEngine().Extract(context.Background(), r.Input{Path: sample.Path, Root: sample.Root}, nil)
			if err != nil {
				t.Fatalf("%s (root %s): %v", sample.Path, sample.Root, err)
			}
			for key, difference := range sampleDifferences(sample, got) {
				t.Errorf("%s: %s\npath: %s\nroot: %s\nreason: %s", key, difference, sample.Path, sample.Root, sample.Reason)
			}
		})
	}
}

func TestRecognitionCorpusFormat(t *testing.T) {
	samples := loadPathSamples(t)
	if len(samples) < 97 {
		t.Fatal("path corpus lost passing baseline samples", len(samples))
	}
	for name, mutate := range map[string]func(*pathSample){
		"missing field":      func(s *pathSample) { delete(s.Expected, "title") },
		"unknown field":      func(s *pathSample) { delete(s.Expected, "title"); s.Expected["titles"] = json.RawMessage(`null`) },
		"null episodes":      func(s *pathSample) { s.Expected["episodes"] = json.RawMessage(`null`) },
		"invalid type":       func(s *pathSample) { s.Expected["year"] = json.RawMessage(`"2024"`) },
		"missing reason":     func(s *pathSample) { s.Reason = "" },
		"missing root":       func(s *pathSample) { s.Root = "" },
		"missing errors":     func(s *pathSample) { s.Errors = nil },
		"unknown error rule": func(s *pathSample) { s.Errors["unknown"] = "failed" },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(samples[0])
			var sample pathSample
			_ = json.Unmarshal(raw, &sample)
			mutate(&sample)
			raw, _ = json.Marshal([]pathSample{sample})
			if _, err := readPathSamples(raw); err == nil {
				t.Fatal("accepted malformed corpus")
			}
		})
	}
	for _, raw := range [][]byte{[]byte(`[]`), []byte(`[{"legacy":true}]`), []byte(`[] []`)} {
		if _, err := readPathSamples(raw); err == nil {
			t.Fatal("accepted malformed corpus", string(raw))
		}
	}
	raw, _ := json.Marshal([]pathSample{samples[0], samples[0]})
	if _, err := readPathSamples(raw); err == nil {
		t.Fatal("accepted duplicate samples")
	}
}

// Known gaps keep the desired result, not a snapshot of incorrect output.
// Only the listed differences are tolerated; a fix must promote the sample.
func TestRecognitionKnownGaps(t *testing.T) {
	var gaps []struct {
		Issue       string     `json:"issue"`
		Sample      pathSample `json:"sample"`
		Differences []string   `json:"differences"`
	}
	decoder := json.NewDecoder(bytes.NewReader(readCorpusFile(t, "known-gaps.json")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&gaps); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("trailing known-gap data", err)
	}
	all := loadPathSamples(t)
	for _, gap := range gaps {
		all = append(all, gap.Sample)
	}
	raw, _ := json.Marshal(all)
	if _, err := readPathSamples(raw); err != nil {
		t.Fatal(err)
	}
	if len(all) < 100 {
		t.Fatal("corpus lost samples", len(all))
	}
	for _, gap := range gaps {
		t.Run(gap.Sample.ID, func(t *testing.T) {
			if strings.TrimSpace(gap.Issue) == "" || len(gap.Differences) == 0 {
				t.Fatal("known gap requires an issue and explicit differing fields")
			}
			got, err := r.DefaultEngine().Extract(context.Background(), r.Input{Path: gap.Sample.Path, Root: gap.Sample.Root}, nil)
			if err != nil {
				t.Fatal(err)
			}
			differences := sampleDifferences(gap.Sample, got)
			if len(differences) == 0 {
				t.Fatal("gap fixed: move this sample from known-gaps.json to paths.json")
			}
			if len(differences) != len(gap.Differences) {
				t.Fatalf("known gap changed: expected fields %v, actual %v", gap.Differences, differences)
			}
			for _, field := range gap.Differences {
				if _, ok := differences[field]; !ok {
					t.Errorf("known gap changed: expected difference in %s, actual %v", field, differences)
				}
			}
			t.Logf("KNOWN GAP (not a passing recognition): %s; %v", gap.Issue, differences)
		})
	}
}
