package recognition_test

import (
	"context"
	"encoding/json"
	r "joycurator/internal/recognition"
	"testing"
)

func TestExamplesUseFullPipelineAndFailOnExecutionErrors(t *testing.T) {
	p := r.DefaultPipeline()
	p.Tokenizer.Examples = []r.Example{}
	p.Resolver.Examples = []r.Example{}
	p.Annotators[0].Examples = []r.Example{{Paths: []string{"/Film.mkv"}, Root: "/", Targets: []string{"/Film.mkv"}, Expected: json.RawMessage(`[[]]`)}}
	got, err := r.Examples(context.Background(), p)
	if err != nil || len(got) != 1 || !got[0].Passed {
		t.Fatal(got, err)
	}
	p.Annotators[1].Config.Code = `function extract(ctx){throw Error("broken")}`
	got, err = r.Examples(context.Background(), p)
	if err != nil || got[0].Passed || got[0].Error == "" {
		t.Fatal(got, err)
	}
}
func TestHTTPPipelineSimulationAndStrictPayload(t *testing.T) {
	s := newService(t)
	library := decodeResponse[struct{ Pipeline r.Pipeline }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	if library.Pipeline.Tokenizer.ID == "" {
		t.Fatal(library)
	}
	path := s.source + "/Show.S00E01-E03.mkv"
	got := decodeResponse[r.BatchResult](t, s.call(t, "POST", "/api/extraction/test", map[string]any{"paths": []string{path}, "targets": []string{path}}, 200))
	if len(got.Files) != 1 || value(got.Files[0].Metadata.Season, -1) != 0 || s.history(t).Total != 0 {
		t.Fatal(got)
	}
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"rules": []any{}}, 400)
	s.call(t, "PUT", "/api/extraction/rules", map[string]any{"pipeline": library.Pipeline}, 200)
	s.call(t, "POST", "/api/extraction/test", map[string]any{"path": path}, 400)
	if err := s.app.Close(); err != nil {
		t.Fatal(err)
	}
	s.app = nil
	s.open(t)
	after := decodeResponse[struct{ Pipeline r.Pipeline }](t, s.call(t, "GET", "/api/extraction/rules", nil, 200))
	if after.Pipeline.Resolver.ID != library.Pipeline.Resolver.ID {
		t.Fatal(after)
	}
}

func TestDefaultExamples(t *testing.T) {
	results, err := r.Examples(context.Background(), r.DefaultPipeline())
	if err != nil || len(results) != 4 {
		t.Fatal(results, err)
	}
	for _, r := range results {
		if !r.Passed {
			t.Fatal(r)
		}
	}
}
