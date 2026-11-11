package recognition_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	r "joycurator/internal/recognition"
)

func TestCoreAnnotationsAreIndependentOfExecutionOrder(t *testing.T) {
	paths := []string{
		"/source/作品/作品第一季03.2024.HD1080P.tmdb-7.mkv",
		"/source/作品/作品第一季05.2024.HD1080P.tmdb-7.mkv",
		"/source/仙逆 (2023) [tmdb-223911]/Season 01/仙逆 - S01E01.mp4",
		"/source/冲突 - 第 2024 集.tmdb-603.mkv",
	}
	p := r.DefaultPipeline()
	baseline := batch(t, p, paths, nil)
	normalize := func(documents []r.Document) {
		for i := range documents {
			sort.Slice(documents[i].Candidates, func(a, b int) bool {
				return documents[i].Candidates[a].ID < documents[i].Candidates[b].ID
			})
		}
	}
	normalize(baseline.Documents)
	var permute func(int)
	permute = func(index int) {
		if index == len(p.Annotators)-1 {
			for i := range p.Annotators {
				p.Annotators[i].Order = (i + 1) * 10
			}
			got := batch(t, p, paths, nil)
			normalize(got.Documents)
			if !reflect.DeepEqual(got.Files, baseline.Files) || !reflect.DeepEqual(got.Documents, baseline.Documents) {
				t.Fatal("execution order changed candidates or decisions", got.Files)
			}
			return
		}
		for i := index; i < len(p.Annotators)-1; i++ {
			p.Annotators[index], p.Annotators[i] = p.Annotators[i], p.Annotators[index]
			permute(index + 1)
			p.Annotators[index], p.Annotators[i] = p.Annotators[i], p.Annotators[index]
		}
	}
	permute(0)
}

func TestCoreResolverOwnsOverlappingInterpretations(t *testing.T) {
	got := batch(t, r.DefaultPipeline(), []string{"/source/Show - 第 2024 集.tmdb-603.mkv"}, nil)
	m := got.Files[0].Metadata
	if m.Year != nil || value(m.TMDBID, 0) != 603 || !reflect.DeepEqual(m.Episodes, []int{2024}) {
		t.Fatal(m)
	}
	year, guess := false, false
	for _, c := range got.Documents[0].Candidates {
		if c.Role == r.Year && c.Value == 2024 {
			year = true
		}
	}
	p := r.DefaultPipeline()
	p.Annotators = append(p.Annotators, regexRule("conflicting-season", 100, r.Season, `^(?P<value>603)$`), regexRule("conflicting-episode", 110, r.Episodes, `^(?P<value>603)$`))
	got = batch(t, p, []string{"/source/Show.tmdb-603.mkv"}, nil)
	m = got.Files[0].Metadata
	if m.Season != nil || len(m.Episodes) != 0 || value(m.TMDBID, 0) != 603 {
		t.Fatal(m)
	}
	for _, c := range got.Documents[0].Candidates {
		if c.RuleID == "conflicting-episode" && c.Status == "valid" {
			guess = true
		}
	}
	if !year || !guess {
		t.Fatal("alternative interpretations disappeared")
	}
}

func TestHeuristicEpisodesOnlyUseClearNumericPositions(t *testing.T) {
	for _, name := range []string{"作品1", "HD1080P", "H264", "07v2", "第一季123456"} {
		got := batch(t, r.DefaultPipeline(), []string{"/source/" + name + ".mkv"}, nil)
		for _, c := range got.Documents[0].Candidates {
			if c.Role == r.Episodes {
				t.Fatal(name, c)
			}
		}
	}
	for _, name := range []string{"03", "[作品][03]", "作品第一季03"} {
		got := batch(t, r.DefaultPipeline(), []string{"/source/" + name + ".mkv"}, nil)
		found := false
		for _, c := range got.Documents[0].Candidates {
			if c.Role == r.Episodes && c.Strength == "heuristic" && reflect.DeepEqual(c.Value, []int{3}) {
				found = true
			}
		}
		if !found || len(got.Files[0].Metadata.Episodes) != 0 {
			t.Fatal(name, got.Files)
		}
	}
	got := batch(t, r.DefaultPipeline(), []string{"/source/Film.HD1080P.AAC.GROUP.mkv"}, nil)
	if value(got.Files[0].Metadata.Title, "") != "Film HD1080P AAC GROUP" {
		t.Fatal(got.Files)
	}
}

func TestRemovedIdentitiesAndSevenFieldResultsAreRejected(t *testing.T) {
	if len(r.Roles()) != 6 || len(r.Targets()) != 6 {
		t.Fatal(r.Roles())
	}
	for _, role := range []r.Target{"releaseTag", "resolution", "videoCodec", "audioCodec", "bitDepth", "dynamicRange", "language", "subtitleLanguage", "checksum"} {
		p := r.DefaultPipeline()
		p.Annotators[0].Target = role
		if _, err := r.Compile(p); err == nil {
			t.Fatal("removed identity accepted", role)
		}
	}
	p := r.DefaultPipeline()
	p.Resolver.Config.Code = `function extract(ctx){return ctx.targetFileIds.map(fileId=>({fileId,metadata:{tmdbId:null,title:null,mediaType:null,year:null,season:null,episodes:[],releaseTag:null},decisions:{}}))}`
	e, err := r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Extract(context.Background(), r.Input{Path: "/source/Film.mkv", Root: "/source"}, nil); err == nil || !strings.Contains(err.Error(), "六个字段") {
		t.Fatal(err)
	}
}
