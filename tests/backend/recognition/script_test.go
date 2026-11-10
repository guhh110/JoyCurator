package recognition_test

import (
	"context"
	r "joycurator/internal/recognition"
	"strings"
	"testing"
)

func TestTokenizerLosslessUnicodeGroupsAndLimits(t *testing.T) {
	got := batch(t, r.DefaultPipeline(), []string{"/source/标题/𠮷[任意(01)] 1080p.mkv"}, nil)
	doc := got.Documents[0]
	if len(doc.Tokens) == 0 || doc.Tokens[0].Text != "𠮷" {
		t.Fatal(doc.Tokens)
	}
	for _, code := range []string{`function extract(ctx){return []}`, `function extract(ctx){return [{id:"x",sourceId:"name",parentId:null,kind:"text",text:"x",start:0,end:1}]}`, `function extract(ctx){ctx.sources[0].text="x";return []}`, `function extract(ctx){while(true){}}`} {
		p := r.DefaultPipeline()
		p.Tokenizer.Config.Code = code
		e, err := r.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Extract(context.Background(), r.Input{Path: "/𠮷Film.mkv"}, nil); err == nil {
			t.Fatal("invalid tokenizer accepted", code)
		}
	}
	e := r.DefaultEngine()
	if _, err := e.Extract(context.Background(), r.Input{Path: "/" + strings.Repeat("[", 18) + "x" + strings.Repeat("]", 18) + ".mkv"}, nil); err == nil {
		t.Fatal("deep groups accepted")
	}
}
func TestScriptsRejectHostAccessAndInvalidJSON(t *testing.T) {
	for _, code := range []string{`function extract(ctx){return require("fs")}`, `function extract(ctx){return fetch("https://example.com")}`, `function extract(ctx){return [()=>1]}`, `function extract(ctx){const a=[];a.push(a);return a}`, `async function extract(ctx){return []}`} {
		p := r.DefaultPipeline()
		p.Tokenizer.Config.Code = code
		e, err := r.Compile(p)
		if err == nil {
			_, err = e.Extract(context.Background(), r.Input{Path: "/Film.mkv"}, nil)
		}
		if err == nil {
			t.Fatal(code)
		}
	}
}
func TestResolverTimeoutAndCancellation(t *testing.T) {
	p := r.DefaultPipeline()
	p.Resolver.Config.Code = `function extract(ctx){while(true){}}`
	e, _ := r.Compile(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Extract(ctx, r.Input{Path: "/Film.mkv"}, nil); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestTokenizerAndCandidateCountLimits(t *testing.T) {
	for _, stage := range []string{"tokenizer", "annotator"} {
		p := r.DefaultPipeline()
		code := `function extract(ctx){return Array.from({length:1025},(_,i)=>({id:"t"+i,sourceId:"name",parentId:null,kind:"text",text:"F",start:0,end:1}))}`
		if stage == "tokenizer" {
			p.Tokenizer.Config.Code = code
		} else {
			p.Annotators[0].Config.Code = `function extract(ctx){return Array.from({length:513},()=>({value:1,spans:[{sourceId:"name",start:0,end:1}],strength:"explicit",status:"valid",reason:"x"}))}`
		}
		e, err := r.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Extract(context.Background(), r.Input{Path: "/Film.mkv"}, nil); err == nil {
			t.Fatal(stage, "limit ignored")
		}
	}
}
func TestRuntimeErrorsKeepLocationsAndGetterDeadline(t *testing.T) {
	for _, code := range []string{`function extract(ctx){trace("before");throw Error("boom")}`, `function extract(ctx){return [{get value(){while(true){}}}]}`} {
		p := r.DefaultPipeline()
		p.Tokenizer.Config.Code = code
		e, _ := r.Compile(p)
		got, err := e.Extract(context.Background(), r.Input{Path: "/Film.mkv"}, nil)
		if err == nil || len(got.Steps) != 1 || len(got.Steps[0].Attempts[0].Trace) == 0 {
			t.Fatal(got, err)
		}
	}
}
