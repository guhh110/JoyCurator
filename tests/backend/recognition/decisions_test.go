package recognition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	r "joycurator/internal/recognition"
)

func TestCompactSeasonNumbersRequireDirectoryCorroboration(t *testing.T) {
	paths := []string{"/source/斗破苍穹 第1季/斗破苍穹 第一季03.1080p无水印.mp4", "/source/斗破苍穹 第1季/斗破苍穹 第一季04.1080p无水印.mp4"}
	single := batch(t, r.DefaultPipeline(), paths[:1], nil)
	if len(single.Files[0].Metadata.Episodes) != 0 {
		t.Fatal(single.Files)
	}
	got := batch(t, r.DefaultPipeline(), paths, nil)
	for i, f := range got.Files {
		if value(f.Metadata.Title, "") != "斗破苍穹" || value(f.Metadata.Season, -1) != 1 || value(f.Metadata.MediaType, "") != "tv" || !reflect.DeepEqual(f.Metadata.Episodes, []int{i + 3}) {
			t.Fatal(got.Files)
		}
		peer := false
		for _, ref := range f.Decisions[r.Episodes].Candidates {
			if ref.FileID != f.FileID {
				peer = true
			}
		}
		if !peer {
			t.Fatal("missing directory evidence", f)
		}
	}
	found := false
	for _, c := range got.Documents[0].Candidates {
		if c.Role == r.Episodes && c.Strength == "heuristic" {
			if c.Spans[0].Start != 8 || c.Spans[0].End != 10 || !reflect.DeepEqual(c.Value, []int{3}) {
				t.Fatal(c)
			}
			found = true
		}
	}
	if !found {
		t.Fatal(got.Documents)
	}
	reversed := batch(t, r.DefaultPipeline(), []string{paths[1], paths[0]}, paths[:1])
	if !reflect.DeepEqual(got.Files[:1], reversed.Files) {
		t.Fatal("order or target subset changed decision", reversed.Files)
	}
	restored, err := r.DefaultEngine().Snapshot(context.Background(), got.Context)
	if err != nil || !reflect.DeepEqual(restored.Files, got.Files) {
		t.Fatal(restored.Files, err)
	}
}

func TestDirectoryTrimsOnlyConfirmedEpisodeRanges(t *testing.T) {
	for _, tc := range []struct {
		names []string
		title string
		want  []int
	}{
		{[]string{"三体1.mkv", "三体2.mkv"}, "三体1", nil},
		{[]string{"三体1.S01.mkv", "三体3.S01.mkv"}, "三体1", nil},
		{[]string{"前03后.S01.mkv", "前05后.S01.mkv"}, "前03后", nil},
		{[]string{"𠮷祥03.S01.mkv", "𠮷祥05.S01.mkv"}, "𠮷祥03", nil},
		{[]string{"作品03.S01.mkv", "作品03.S01.1080p.mkv"}, "作品03", nil},
		{[]string{"作品03.S01.mkv"}, "作品03", nil},
		{[]string{"作品03.mkv", "作品第04集.mkv"}, "作品03", nil},
		{[]string{"作品.03.S01.mkv", "作品.05.S01.mkv"}, "作品", []int{3}},
		{[]string{"𠮷祥.03.S01.mkv", "𠮷祥.05.S01.mkv"}, "𠮷祥", []int{3}},
	} {
		paths := make([]string, len(tc.names))
		for i, n := range tc.names {
			paths[i] = "/source/" + n
		}
		got := batch(t, r.DefaultPipeline(), paths, paths[:1])
		f := got.Files[0]
		if value(f.Metadata.Title, "") != tc.title || len(f.Metadata.Episodes) != len(tc.want) || len(tc.want) > 0 && !reflect.DeepEqual(f.Metadata.Episodes, tc.want) {
			t.Fatal(tc.names, f)
		}
		if strings.HasPrefix(tc.names[0], "三体1.mkv") && f.Metadata.MediaType != nil {
			t.Fatal(f)
		}
		if len(tc.want) > 0 {
			if len(f.Decisions[r.Title].Candidates) < 2 {
				t.Fatal("derived title needs both title and episode evidence", f)
			}
			original := false
			for _, c := range got.Documents[0].Candidates {
				if c.Role == r.Title && (strings.Contains(c.Value.(string), "03") || c.Value == "三体1") {
					original = true
				}
			}
			if !original {
				t.Fatal("original title disappeared", got.Documents[0])
			}
		}
	}
}

func TestHeuristicEpisodesKeepAllNumericTitlesWithoutEvidence(t *testing.T) {
	for _, name := range []string{"1917", "三体2", "作品03", "[作品][03]", "07v2", "作品123456"} {
		got := batch(t, r.DefaultPipeline(), []string{"/source/" + name + ".mkv"}, nil)
		if len(got.Files[0].Metadata.Episodes) != 0 || got.Files[0].Metadata.MediaType != nil {
			t.Fatal(name, got.Files)
		}
		if name == "1917" || name == "三体2" || name == "作品03" || name == "07v2" {
			if value(got.Files[0].Metadata.Title, "") != name {
				t.Fatal(name, got.Files)
			}
		}
		for _, c := range got.Documents[0].Candidates {
			if c.Role == r.Episodes && (name == "1917" || name == "作品123456") {
				t.Fatal(name, c)
			}
		}
	}
}

func TestDirectoryInferenceRejectsConflictsAndMultipleVaryingPositions(t *testing.T) {
	cases := [][]string{
		{"/source/作品第一季03.mkv", "/source/作品第二季04.mkv"},
		{"/source/作品03.S01.tmdb-7.mkv", "/source/作品04.S01.tmdb-8.mkv"},
		{"/source/一/作品03.S01.mkv", "/source/二/作品04.S01.mkv"},
		{"/source/作品1版1.S01.mkv", "/source/作品2版1.S01.mkv", "/source/作品1版2.S01.mkv"},
		{"/source/作品03.S01.mkv", "/source/别作04.S01.mkv"},
	}
	for _, paths := range cases {
		got := batch(t, r.DefaultPipeline(), paths, paths[:1])
		if len(got.Files[0].Metadata.Episodes) > 0 {
			t.Fatal(paths, got.Files)
		}
	}
	// A metadata-free file must not bridge two conflicting season/ID cohorts.
	for _, paths := range [][]string{
		{"/source/作品03.mkv", "/source/作品04.S01.mkv", "/source/作品05.S02.mkv"},
		{"/source/作品03.S01.mkv", "/source/作品04.S01.tmdb-7.mkv", "/source/作品05.S01.tmdb-8.mkv"},
	} {
		got := batch(t, r.DefaultPipeline(), paths, paths[:1])
		if len(got.Files[0].Metadata.Episodes) > 0 {
			t.Fatal(paths, got.Files)
		}
	}
}

func TestDirectoryInferenceKeepsUnrecognizedTechnicalTextInStructure(t *testing.T) {
	paths := []string{"/source/Show/[作品][第一季][202][01][1080p].mkv", "/source/Show/[作品][第一季][202][03][720p].mkv", "/source/Show/[作品][第一季][202][03][1080p].mkv"}
	got := batch(t, r.DefaultPipeline(), paths, nil)
	for _, file := range got.Files {
		ep := []int{3}
		if strings.Contains(file.Path, "[01]") {
			ep = []int{1}
		}
		if strings.Contains(file.Path, "720p") {
			ep = []int{}
		}
		m := file.Metadata
		if !reflect.DeepEqual(m.Episodes, ep) || value(m.Title, "") != "作品" {
			t.Fatal(got.Files)
		}
	}
	for _, doc := range got.Documents {
		for _, c := range doc.Candidates {
			if c.Role == r.Episodes && reflect.DeepEqual(c.Value, []int{1080}) {
				t.Fatal(c)
			}
		}
	}
}

func TestInvalidExplicitEpisodeDoesNotFallBackToDirectoryNumbers(t *testing.T) {
	paths := []string{"/source/Show.S01E05-E03.01.mkv", "/source/Show.S01E05-E03.02.mkv"}
	got := batch(t, r.DefaultPipeline(), paths, nil)
	for _, f := range got.Files {
		if len(f.Metadata.Episodes) > 0 {
			t.Fatal(got.Files)
		}
	}
}

func TestNumericEvidenceAndDerivedTitleSurviveHistory(t *testing.T) {
	paths := []string{"/source/作品.03.S01.2024.H264.tmdb-7.mkv", "/source/作品.04.S01.2024.H264.tmdb-7.mkv"}
	got := batch(t, r.DefaultPipeline(), paths, paths[:1])
	f := got.Files[0]
	if value(f.Metadata.Title, "") != "作品" || value(f.Metadata.Year, 0) != 2024 || value(f.Metadata.TMDBID, 0) != 7 || !reflect.DeepEqual(f.Metadata.Episodes, []int{3}) {
		t.Fatal(f)
	}
	for _, c := range got.Documents[0].Candidates {
		if c.Role == r.Episodes && !reflect.DeepEqual(c.Value, []int{3}) && !reflect.DeepEqual(c.Value, []int{7}) {
			t.Fatal(c)
		}
	}
	restored, err := r.DefaultEngine().Snapshot(context.Background(), got.Context)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(got.Files)
	actual, _ := json.Marshal(restored.Files)
	if string(want) != string(actual) {
		t.Fatal(string(actual))
	}
	// Disabling numeric inference preserves the original title through the same pipeline.
	p := r.DefaultPipeline()
	for i := range p.Annotators {
		if p.Annotators[i].Target == r.Episodes {
			p.Annotators[i].Enabled = false
		}
	}
	engine, err := r.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.ExtractBatch(context.Background(), r.BatchInput{SourceBoundary: "/source", Paths: paths, Targets: paths[:1]}, nil)
	if err != nil || value(result.Files[0].Metadata.Title, "") != "作品 03" {
		t.Fatal(result.Files, err)
	}
}

func BenchmarkDirectoryDecisions2000(b *testing.B) {
	paths := make([]string, 2000)
	for i := range paths {
		paths[i] = fmt.Sprintf("/source/第一季/作品.%d.1080p.mp4", i+2100)
	}
	input := r.BatchInput{ContextRoot: "/source/第一季", SourceBoundary: "/source", Paths: paths, Targets: paths}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		got, err := r.DefaultEngine().ExtractBatch(context.Background(), input, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(got.Files) != 2000 || value(got.Files[0].Metadata.Title, "") != "作品" || !reflect.DeepEqual(got.Files[0].Metadata.Episodes, []int{2100}) || !reflect.DeepEqual(got.Files[1999].Metadata.Episodes, []int{4099}) {
			b.Fatal("directory inference lost files or decisions")
		}
		for _, f := range got.Files {
			if len(f.Decisions[r.Episodes].Candidates) > 4 {
				b.Fatal("unbounded directory evidence", f.Path)
			}
		}
	}
}
