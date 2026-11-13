package recognition_test

import (
	"context"
	"encoding/json"
	"errors"
	r "joycurator/internal/recognition"
	"reflect"
	"strings"
	"testing"
)

func regexRule(id string, order int, target r.Target, pattern string) r.Rule {
	return r.Rule{ID: id, Name: id, Stage: "annotate", Order: order, Enabled: true, Kind: "regex", Target: target, Source: r.Filename, Config: r.Config{Pattern: pattern, Capture: "value"}, Examples: []r.Example{}}
}
func extract(t *testing.T, p r.Pipeline, path, root string) r.Result {
	t.Helper()
	e, err := r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Extract(context.Background(), r.Input{Path: path, Root: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func batch(t *testing.T, p r.Pipeline, paths, targets []string) r.BatchResult {
	t.Helper()
	e, err := r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if targets == nil {
		targets = paths
	}
	got, err := e.ExtractBatch(context.Background(), r.BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: paths, Targets: targets}, nil)
	if err != nil {
		t.Fatal(err)
	}
	light, err := e.Snapshot(context.Background(), got.Context)
	if err != nil || !r.Equal(light.Documents, got.Documents) || !r.Equal(light.Files, got.Files) || !reflect.DeepEqual(light.Tree, got.Tree) {
		t.Fatal("light/debug parity", err)
	}
	return got
}
func TestCandidatesRetainConflictsAndOrder(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators = []r.Rule{regexRule("first", 10, r.Season, `S(?P<value>\d+)`), regexRule("later", 20, r.Season, `E(?P<value>\d+)`)}
	got := batch(t, p, []string{"/source/Show.S02E03.mkv"}, nil)
	if len(got.Documents[0].Candidates) != 2 || value(got.Files[0].Metadata.Season, -1) != 2 {
		t.Fatal(got)
	}
	p.Resolver.Config.Code = `function extract(ctx){const f=ctx.files[0],c=f.candidates[1];return [{fileId:f.fileId,metadata:{tmdbId:null,title:null,mediaType:null,year:null,season:c.value,episodes:[]},decisions:{season:{candidates:[{fileId:f.fileId,candidateId:c.id}],reason:"选择后者"}}}];}`
	got = batch(t, p, []string{"/source/Show.S02E03.mkv"}, nil)
	if value(got.Files[0].Metadata.Season, -1) != 3 {
		t.Fatal(got)
	}
}
func TestBatchScreenshotAndVaryingNumbers(t *testing.T) {
	paths := []string{"/source/斗破苍穹 第3季/[斗破苍穹 第3季][Fights Break Sphere III][01][HEVC][GB][1080P].mp4"}
	got := batch(t, r.DefaultPipeline(), paths, nil)
	m := got.Files[0].Metadata
	if value(m.Title, "") != "斗破苍穹" || value(m.Season, -1) != 3 || len(m.Episodes) != 0 {
		t.Fatal(m)
	}
	paths = append(paths, strings.Replace(paths[0], "[01]", "[03]", 1))
	corroborated := batch(t, r.DefaultPipeline(), paths, paths[:1])
	if !reflect.DeepEqual(corroborated.Files[0].Metadata.Episodes, []int{1}) {
		t.Fatal(corroborated.Files)
	}
	paths = []string{"/source/任意作品/[任意作品][202][01][1080p].mkv", "/source/任意作品/[任意作品][202][03][1080p].mkv", "/source/任意作品/[任意作品][202][03][720p].mkv"}
	withoutAnchor := batch(t, r.DefaultPipeline(), paths, paths[:1])
	if len(withoutAnchor.Files[0].Metadata.Episodes) != 0 {
		t.Fatal(withoutAnchor.Files)
	}
	for i := range paths {
		paths[i] = strings.Replace(paths[i], "[202]", "[第一季][202]", 1)
	}
	got = batch(t, r.DefaultPipeline(), paths, paths[:1])
	if len(got.Files) != 1 || !reflect.DeepEqual(got.Files[0].Metadata.Episodes, []int{1}) || len(got.Files[0].Decisions[r.Episodes].Candidates) < 2 {
		t.Fatal(got.Files)
	}
	paths[0], paths[2] = paths[2], paths[0]
	other := batch(t, r.DefaultPipeline(), paths, []string{got.Files[0].Path})
	if !reflect.DeepEqual(got.Files, other.Files) {
		t.Fatal("input order changed result")
	}
}
func TestSeparateWorksAndSeasons(t *testing.T) {
	paths := []string{"/source/Alpha/Season 1/01.mkv", "/source/Alpha/Season 2/02.mkv", "/source/Beta/Season 3/01.mkv"}
	got := batch(t, r.DefaultPipeline(), paths, nil)
	for i, want := range []int{1, 2, 3} {
		if value(got.Files[i].Metadata.Season, -1) != want {
			t.Fatal(got.Files)
		}
	}
}
func TestValidationAndImmutableConfiguration(t *testing.T) {
	p := r.DefaultPipeline()
	e, _ := r.Compile(p)
	p.Tokenizer.Config.Code = "changed"
	copy := e.Rules()
	copy.Resolver.Config.Code = "changed"
	if e.Rules().Tokenizer.Config.Code == "changed" || e.Rules().Resolver.Config.Code == "changed" {
		t.Fatal("mutable config")
	}
	for _, mutate := range []func(*r.Pipeline){func(p *r.Pipeline) { p.Tokenizer.Enabled = false }, func(p *r.Pipeline) { p.Annotators = nil }, func(p *r.Pipeline) { p.Resolver.Stage = "annotate" }, func(p *r.Pipeline) { p.Annotators[0].ID = p.Tokenizer.ID }, func(p *r.Pipeline) { p.Annotators[0].Config.Code = "invalid" }, func(p *r.Pipeline) { p.Annotators[0].Order = p.Annotators[1].Order }} {
		p := r.DefaultPipeline()
		mutate(&p)
		if _, err := r.Compile(p); err == nil {
			t.Fatal("invalid pipeline accepted")
		}
	}
}
func TestCancellationAndAnnotationFailure(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators[0].Config.Code = `function extract(ctx){throw Error("broken")}`
	e, _ := r.Compile(p)
	got, err := e.Extract(context.Background(), r.Input{Path: "/source/Show.S01E02.mkv", Root: "/source"}, nil)
	if err == nil || len(got.Steps) != len(p.Annotators)+1 || got.Metadata.Title != nil {
		t.Fatal(got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = r.DefaultEngine()
	got, err = e.Extract(ctx, r.Input{Path: "/Show.mkv"}, func(_ context.Context, s r.Step) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) || len(got.Steps) == 0 {
		t.Fatal(err)
	}
}
func TestResolverReferencesAndCoverage(t *testing.T) {
	for _, code := range []string{`return []`, `return [{fileId:"extra",metadata:{episodes:[]},decisions:{}}]`, `return [{fileId:ctx.targetFileIds[0],metadata:{title:"x",episodes:[]},decisions:{title:{reason:"fake",candidates:[{fileId:"file-0",candidateId:"missing"}]}}}]`, `return [{fileId:ctx.targetFileIds[0],metadata:{title:"x",episodes:[]},decisions:{}}]`} {
		p := r.DefaultPipeline()
		p.Resolver.Config.Code = "function extract(ctx){" + code + "}"
		e, err := r.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Extract(context.Background(), r.Input{Path: "/Show.mkv"}, nil); err == nil {
			t.Fatal(code)
		}
	}
}
func TestEpisodeIDProtectionAndInvalidExplicitSyntax(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators = append([]r.Rule{regexRule("bad-id", 0, r.Episodes, `^(?P<value>603)$`)}, p.Annotators...)
	e, _ := r.Compile(p)
	got, err := e.Extract(context.Background(), r.Input{Path: "/Film.tmdb-603.mkv"}, nil)
	if err != nil || len(got.Metadata.Episodes) != 0 {
		t.Fatal(got.Metadata, err)
	}
	found := false
	for _, step := range got.Steps {
		if step.RuleID == "bad-id" && step.Status == "matched" {
			found = true
		}
	}
	if !found {
		t.Fatal("conflicting explanation was not retained")
	}
	for _, path := range []string{"/Show.S01E05-E03.mkv", "/Show.S01E01-E1001.mkv", "/Film.2024.1080p-Group_1.mkv"} {
		got := extract(t, r.DefaultPipeline(), path, "/")
		if len(got.Metadata.Episodes) != 0 {
			t.Fatal(path, got.Metadata)
		}
	}
}
func TestJSONUnknownContract(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators = []r.Rule{}
	got := extract(t, p, "/Film.mkv", "/")
	raw, _ := json.Marshal(got.Metadata)
	if string(raw) != `{"tmdbId":null,"title":null,"mediaType":null,"year":null,"season":null,"episodes":[]}` {
		t.Fatal(string(raw))
	}
}

func TestObserverFailureStopsImmediately(t *testing.T) {
	stop := errors.New("observer failed")
	count := 0
	result, err := r.DefaultEngine().Extract(context.Background(), r.Input{Path: "/Show.mkv"}, func(context.Context, r.Step) error {
		count++
		if count == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || count != 2 || len(result.Steps) != 2 {
		t.Fatal(count, len(result.Steps), err)
	}
}
func TestBatchValidationRejectsOverflowAndCanonicalDuplicates(t *testing.T) {
	for _, in := range []r.BatchInput{{ContextRoot: "/source", SourceBoundary: "/source", Paths: []string{"/source/Film.mkv"}, Targets: []string{"/source/Film.mkv", "/source/a/../Film.mkv"}}, {ContextRoot: "/", SourceBoundary: "/source", Paths: []string{"/source/Film.mkv"}, Targets: []string{"/source/Film.mkv"}}, {ContextRoot: "/source", Paths: make([]string, 2001), Targets: []string{"/source/Film.mkv"}}} {
		if _, err := r.DefaultEngine().ExtractBatch(context.Background(), in, nil); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
func TestSourceSelectionAndHostOwnedCandidateFields(t *testing.T) {
	for _, body := range []string{`return [{id:"fake",value:"x",spans:[{sourceId:"name",start:0,end:1}],strength:"explicit",status:"valid",reason:"x"}]`, `return [{value:"x",spans:[{sourceId:"dir-0",start:0,end:1}],strength:"explicit",status:"valid",reason:"x"}]`} {
		p := r.DefaultPipeline()
		p.Annotators = []r.Rule{{ID: "custom", Name: "custom", Stage: "annotate", Target: r.Title, Source: r.Filename, Enabled: true, Kind: "script", Config: r.Config{Code: "function extract(ctx){" + body + "}"}}}
		e, _ := r.Compile(p)
		if _, err := e.Extract(context.Background(), r.Input{Path: "/source/Show/Film.mkv", Root: "/source"}, nil); err == nil {
			t.Fatal("invalid candidate accepted")
		}
	}
}
