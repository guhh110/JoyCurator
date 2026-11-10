package recognition_test

import (
	"context"
	r "joycurator/internal/recognition"
	"reflect"
	"testing"
)

func TestWordTokenizerBoundariesAndNestedGroups(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators = []r.Rule{}
	got := batch(t, p, []string{"/source/《第3集》：HD1080P@S01E03_H.264-tmdb=123 [𠮷祥（2024）]😀.mkv"}, nil)
	words := []string{}
	groups := map[string]r.Token{}
	for _, token := range got.Documents[0].Tokens {
		if token.Kind == "text" || token.Kind == "number" {
			words = append(words, token.Text)
			if token.Text == "264" && token.Kind != "number" || token.Text == "HD1080P" && token.Kind != "text" {
				t.Fatal(token)
			}
		}
		if token.Kind == "group" {
			groups[token.Text] = token
		}
	}
	if !reflect.DeepEqual(words, []string{"第3集", "HD1080P", "S01E03", "H", "264", "tmdb", "123", "𠮷祥", "2024"}) {
		t.Fatal(words)
	}
	inner, outer := groups["（2024）"], groups["[𠮷祥（2024）]"]
	if inner.ParentID == nil || *inner.ParentID != outer.ID {
		t.Fatal(groups)
	}
}

func TestAnnotationsRequireWordTokens(t *testing.T) {
	p := r.DefaultPipeline()
	p.Tokenizer.Config.Code = `function extract(ctx) { return ctx.sources.map((s,i)=>({id:"t"+i,sourceId:s.id,parentId:null,kind:"separator",text:s.text,start:0,end:s.text.length})) }`
	got := batch(t, p, []string{"/source/作品第3季.第01集.tmdb-123.2024.HD1080P.H.264-AAC.mkv"}, nil)
	if len(got.Documents[0].Candidates) != 0 {
		t.Fatal(got.Documents[0].Candidates)
	}
}

func TestSharedChinesePathWithWordAnnotations(t *testing.T) {
	path := "/主页/quark2/来自：分享/三年之约 去片头片尾纯享版/斗破苍穹：三年之约.第01集.2021.HD1080P .【国漫】- 无水印 去片头片尾纯享版 - AAC. H264 .AVC .酷安@此间微凉.mp4"
	got, err := r.DefaultEngine().ExtractBatch(context.Background(), r.BatchInput{SourceBoundary: "/", Paths: []string{path}, Targets: []string{path}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := got.Files[0].Metadata
	if value(m.Title, "") != "斗破苍穹 三年之约" || value(m.Year, 0) != 2021 || !reflect.DeepEqual(m.Episodes, []int{1}) || value(m.MediaType, "") != "tv" || m.Season != nil || m.TMDBID != nil {
		t.Fatal(m)
	}
	resolution := false
	titles := []string{}
	for _, c := range got.Documents[0].Candidates {
		if c.Role == "resolution" && c.Value == "HD1080P" {
			resolution = true
		}
		if c.Role == r.Title && c.Spans[0].SourceID == "name" {
			titles = append(titles, c.Value.(string))
		}
	}
	if !resolution || !reflect.DeepEqual(titles, []string{"斗破苍穹 三年之约", "无水印 去片头片尾纯享版", "酷安 此间微凉"}) {
		t.Fatal(resolution, titles)
	}
	restored, err := r.RestoreBatch(got.Steps)
	if err != nil || !reflect.DeepEqual(restored.Files, got.Files) {
		t.Fatal(restored.Files, err)
	}
}

func TestWordAnnotationsAcrossSeparatorsAndWithinWords(t *testing.T) {
	for _, tc := range []struct {
		path, title string
		id, season  int
		episodes    []int
	}{
		{"Film.tmdb-603.2024.H.264.mkv", "Film", 603, -1, []int{}},
		{"Show.S02E03-E05.DTS-HD.5.1.mkv", "Show", -1, 2, []int{3, 4, 5}},
		{"斗破苍穹第3季第02集.mkv", "斗破苍穹", -1, 3, []int{2}},
	} {
		got := batch(t, r.DefaultPipeline(), []string{"/source/" + tc.path}, nil)
		m := got.Files[0].Metadata
		if value(m.Title, "") != tc.title || value(m.TMDBID, -1) != tc.id || value(m.Season, -1) != tc.season || !reflect.DeepEqual(m.Episodes, tc.episodes) {
			t.Fatal(tc.path, m)
		}
	}
}

func TestTitleAnnotationsRunLastAndKeepSeparateRemainders(t *testing.T) {
	p := r.DefaultPipeline()
	for i := range p.Annotators {
		if p.Annotators[i].Target == r.Title {
			p.Annotators[i].Order = 0
		}
	}
	p.Annotators = append(p.Annotators, r.Rule{ID: "invalid", Name: "invalid", Stage: "annotate", Target: r.Season, Order: 1000, Enabled: true, Kind: "script", Source: r.Filename, Config: r.Config{Code: `function extract(ctx){ return ctx.tokens.filter(t=>t.text==="INVALID").map(t=>({value:null,spans:[{sourceId:t.sourceId,start:t.start,end:t.end}],strength:"explicit",status:"invalid",reason:"无效季号"})) }`}, Examples: []r.Example{}})
	got := batch(t, p, []string{"/source/前缀:INVALID:后缀.2024.mkv"}, nil)
	titles := []string{}
	for _, c := range got.Documents[0].Candidates {
		if c.Role == r.Title {
			titles = append(titles, c.Value.(string))
		}
	}
	if !reflect.DeepEqual(titles, []string{"前缀", "后缀"}) || got.Steps[len(got.Steps)-2].Target != r.Title {
		t.Fatal(titles, got.Steps)
	}
	if value(got.Files[0].Metadata.Title, "") != "前缀" || got.Files[0].Metadata.Season != nil {
		t.Fatal(got.Files)
	}
	got = batch(t, r.DefaultPipeline(), []string{"/source/前缀第3季后缀.mkv"}, nil)
	titles = []string{}
	for _, c := range got.Documents[0].Candidates {
		if c.Role == r.Title {
			titles = append(titles, c.Value.(string))
		}
	}
	if !reflect.DeepEqual(titles, []string{"前缀", "后缀"}) {
		t.Fatal(titles)
	}
}

func TestRegexMatchesWordsAndMapsUnicodeOffsets(t *testing.T) {
	p := r.DefaultPipeline()
	p.Annotators = []r.Rule{regexRule("id", 10, r.TMDBID, `^(?P<value>123)$`)}
	got := batch(t, p, []string{"/source/𠮷祥.tmdb-123.mkv"}, nil)
	c := got.Documents[0].Candidates[0]
	if value(got.Files[0].Metadata.TMDBID, 0) != 123 || c.Spans[0].Start != 9 || c.Spans[0].End != 12 {
		t.Fatal(got.Files, c)
	}
	p.Annotators[0].Config.Pattern = `tmdb-(?P<value>123)`
	got = batch(t, p, []string{"/source/𠮷祥.tmdb-123.mkv"}, nil)
	if len(got.Documents[0].Candidates) != 0 {
		t.Fatal(got.Documents[0].Candidates)
	}
}
