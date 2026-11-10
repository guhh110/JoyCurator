package recognition

import "encoding/json"

func DefaultPipeline() Pipeline {
	catalog := Catalog()
	p := Pipeline{Tokenizer: catalog[0], Annotators: []Rule{}, Resolver: catalog[len(catalog)-1]}
	for i, r := range catalog[1 : len(catalog)-1] {
		r.Order = (i + 1) * 10
		p.Annotators = append(p.Annotators, r)
	}
	p.Tokenizer.Examples = []Example{{Paths: []string{"/source/Film.mkv"}, Root: "/source", Targets: []string{"/source/Film.mkv"}, Expected: json.RawMessage(`[[{"id":"t0","sourceId":"name","parentId":null,"kind":"text","text":"Film","start":0,"end":4}]]`)}}
	p.Annotators[0].Examples = []Example{{Paths: []string{"/source/Film.tmdb-7.mkv"}, Root: "/source", Targets: []string{"/source/Film.tmdb-7.mkv"}, Expected: json.RawMessage(`[[{"id":"tmdb-id:0","ruleId":"tmdb-id","role":"tmdbId","value":7,"spans":[{"sourceId":"name","start":5,"end":11}],"strength":"explicit","status":"valid","reason":"TMDB 标记"}]]`)}}
	p.Tokenizer.Examples = append(p.Tokenizer.Examples, Example{Paths: []string{"/source/第3集.HD1080P.mkv"}, Root: "/source", Targets: []string{"/source/第3集.HD1080P.mkv"}, Expected: json.RawMessage(`[[{"id":"t0","sourceId":"name","parentId":null,"kind":"text","text":"第3集","start":0,"end":3},{"id":"t1","sourceId":"name","parentId":null,"kind":"separator","text":".","start":3,"end":4},{"id":"t2","sourceId":"name","parentId":null,"kind":"text","text":"HD1080P","start":4,"end":11}]]`)})
	p.Resolver.Examples = []Example{{Paths: []string{"/source/Show.S00E01.mkv"}, Root: "/source", Targets: []string{"/source/Show.S00E01.mkv"}, Expected: json.RawMessage(`[{"fileId":"file-0","path":"/source/Show.S00E01.mkv","metadata":{"tmdbId":null,"title":"Show","mediaType":"tv","year":null,"season":0,"episodes":[1],"releaseTag":null},"decisions":{"title":{"candidates":[{"fileId":"file-0","candidateId":"title:0"}],"reason":"来源片名"},"season":{"candidates":[{"fileId":"file-0","candidateId":"season:0"}],"reason":"季集标记"},"episodes":{"candidates":[{"fileId":"file-0","candidateId":"episodes:0"}],"reason":"季集标记"},"mediaType":{"candidates":[{"fileId":"file-0","candidateId":"season:0"},{"fileId":"file-0","candidateId":"episodes:0"}],"reason":"季号或集数提供剧集依据"}}}]`)}}
	return p
}

var defaultEngine = func() *Engine {
	e, err := Compile(DefaultPipeline())
	if err != nil {
		panic(err)
	}
	return e
}()

func DefaultEngine() *Engine { return defaultEngine }
