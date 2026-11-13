package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestNativeInputMatchesJSONAndFreezesPlainData(t *testing.T) {
	values := map[string]any{
		"params":  map[string]any{"__proto__": map[string]any{"polluted": true}, "constructor": "ordinary", "10": "ten", "2": "two", "nested": []any{nil, []any{}, "𠮷<&\u2028\u2029\xff", math.Copysign(0, -1)}},
		"sources": []TextSource{{ID: "name", Kind: Filename, Text: "标题", Depth: 0}},
		"tokens":  []Token{}, "candidates": []Candidate{}, "nil": []any(nil),
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	native, size, err := makeScriptInput(context.Background(), vm, values, len(raw))
	if err != nil {
		t.Fatal(err)
	}
	if size != len(raw) {
		t.Fatalf("bytes=%d want %d", size, len(raw))
	}
	if _, _, err = makeScriptInput(context.Background(), goja.New(), values, len(raw)-1); err == nil {
		t.Fatal("input byte limit ignored")
	}
	vm.Set("native", native)
	vm.Set("encoded", string(raw))
	_, err = vm.RunString(`
 const parsed=JSON.parse(encoded);
 function inspect(a,b){
  if(a===null || typeof a!=="object") {if(!Object.is(a,b)) throw Error("scalar differs");return;}
  if(!Object.isFrozen(a)) throw Error("unfrozen input");
  if(Object.getPrototypeOf(a)!==(Array.isArray(a)?Array.prototype:Object.prototype)) throw Error("host prototype");
  const keys=Object.keys(a);if(JSON.stringify(keys)!==JSON.stringify(Object.keys(b))) throw Error("key order");
  for(const k of keys) {const d=Object.getOwnPropertyDescriptor(a,k);if(!("value" in d)) throw Error("host accessor");inspect(a[k],b[k]);}
 }
 inspect(native,parsed);
 if(!Object.hasOwnProperty.call(native.params,"__proto__") || ({}).polluted) throw Error("special property");
 `)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = makeScriptInput(ctx, goja.New(), values, len(raw)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestJSONByteCountersMatchEncoder(t *testing.T) {
	for _, value := range []any{"", "\x00\x01\b\t\n\r\f\\\"<&>", "𠮷é\u2028\u2029\xff\xfe", 0.0, math.Copysign(0, -1), 1e-7, 1e-6, 1e20, 1e21, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var size int
		switch v := value.(type) {
		case string:
			size = jsonStringSize(v)
		case float64:
			size = jsonFloatSize(v)
		}
		if size != len(raw) {
			t.Fatalf("%q bytes=%d want %d", raw, size, len(raw))
		}
	}
	for _, value := range []any{nil, true, false, json.Number("1e-7"), []any{}, map[string]any{"<&>": []any{json.Number("-0"), "\xff", nil}}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		size, err := jsonDataSize(value)
		if err != nil || size != len(raw) {
			t.Fatalf("%q bytes=%d: %v", raw, size, err)
		}
	}
}

func TestPreparedRulesAreIsolatedAcrossParallelCalls(t *testing.T) {
	p := DefaultPipeline()
	p.Annotators[0].Config.Params = json.RawMessage(`{"__proto__":{"safe":true},"nested":[{"value":1}]}`)
	p.Annotators[0].Config.Code = `function extract(ctx){
 if(globalThis.used || Object.prototype.leaked || Array.prototype.leaked) throw Error("leaked runtime");
 if(!Object.prototype.hasOwnProperty.call(ctx.params,"__proto__") || !ctx.params.__proto__.safe) throw Error("special key lost");
 globalThis.used=true;Object.prototype.leaked=true;Array.prototype.leaked=true;Object.freeze=()=>{throw Error("freeze replaced")};return [];
 }`
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/source/d.mkv", "/source/c.mkv", "/source/b.mkv", "/source/a.mkv"}
	in := BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: paths, Targets: paths}
	for n := 0; n < 2; n++ {
		if _, err = e.Snapshot(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParallelExecutionKeepsOrderAndStopsWorkers(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previous)
	p := DefaultPipeline()
	// Reverse completion order while keeping the same path-ordered diagnostics.
	p.Annotators[0].Config.Code = `function extract(ctx){const until=Date.now()+(ctx.file.path.includes("a.")?25:1);while(Date.now()<until){};throw Error(ctx.file.path);}`
	p.Resolver.Config.Code = `function extract(ctx){throw Error("resolver must not run after annotation errors")}`
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/source/d.mkv", "/source/c.mkv", "/source/b.mkv", "/source/a.mkv"}
	in := BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: paths, Targets: paths}
	var progress []int
	snapshot, err := e.Snapshot(context.Background(), in, func(done, total int) error {
		if total != 4 {
			t.Fatal(total)
		}
		progress = append(progress, done)
		return nil
	})
	if err == nil || !reflect.DeepEqual(progress, []int{1, 2, 3, 4}) || len(snapshot.Files) != 0 || len(snapshot.Diagnostics) != 4 {
		t.Fatalf("%v %v %+v", err, progress, snapshot.Diagnostics)
	}
	for i, d := range snapshot.Diagnostics {
		if d.FileID != snapshot.Documents[i].FileID || !strings.Contains(d.Message, snapshot.Documents[i].Path) {
			t.Fatal(d)
		}
	}
	stop := errors.New("progress failed")
	_, err = e.Snapshot(context.Background(), in, func(int, int) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	p.Tokenizer.Config.Code = `function extract(ctx){while(true){}}`
	e, err = Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = e.Snapshot(ctx, in)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("workers did not stop: %v", err)
	}
}

func TestLightAndDebugRejectSameInvalidResults(t *testing.T) {
	for _, expression := range []string{
		`[{id:"x",unknown:1}]`, `[{id:1}]`, `[{start:1.5}]`, `[{end:Infinity}]`,
		`Promise.resolve([])`, `(()=>{const a=[];a.push(a);return a})()`,
		`[{get id(){while(true){}}}]`, `Array(20001).fill(null)`,
	} {
		t.Run(expression, func(t *testing.T) {
			p := DefaultPipeline()
			p.Tokenizer.Config.Code = `function extract(ctx){return ` + expression + `}`
			e, err := Compile(p)
			if err != nil {
				t.Fatal(err)
			}
			in := BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: []string{"/source/a.mkv"}, Targets: []string{"/source/a.mkv"}}
			light, le := e.Snapshot(context.Background(), in)
			debug, de := e.ExtractBatch(context.Background(), in, nil)
			if le == nil || de == nil || len(light.Diagnostics) != 1 || len(debug.Steps) != 1 {
				t.Fatalf("invalid result accepted: %v %v", le, de)
			}
		})
	}
}

func TestTokenizerFailureKeepsOrderedPrefixAndCancelsLaterFiles(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previous)
	p := DefaultPipeline()
	p.Tokenizer.Config.Code = strings.Replace(p.Tokenizer.Config.Code, `function extract(ctx) {`, `function extract(ctx) {
 if(ctx.file.path.endsWith("/a.mkv")){const until=Date.now()+30;while(Date.now()<until){}}
 if(ctx.file.path.endsWith("/b.mkv"))throw Error("tokenizer failed");
 if(ctx.file.path.endsWith("/c.mkv"))while(true){}
 `, 1)
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/source/d.mkv", "/source/c.mkv", "/source/b.mkv", "/source/a.mkv", "/source/e.mkv"}
	in := BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: paths, Targets: paths}
	var progress []int
	got, err := e.Snapshot(context.Background(), in, func(done, total int) error { progress = append(progress, done); return nil })
	if err == nil || !strings.Contains(err.Error(), "tokenizer failed") || !reflect.DeepEqual(progress, []int{1}) || len(got.Diagnostics) != 1 || got.Diagnostics[0].FileID != "file-1" {
		t.Fatalf("%v %v %+v", err, progress, got.Diagnostics)
	}
	if len(got.Files) != 0 || len(got.Documents[0].Tokens) == 0 {
		t.Fatal("prefix or failed tree changed")
	}
	for _, doc := range got.Documents[1:] {
		if len(doc.Tokens) != 0 || len(doc.Candidates) != 0 {
			t.Fatalf("later result escaped failed prefix: %s", doc.Path)
		}
	}
}

func TestLightExecutionStillValidatesTraceArguments(t *testing.T) {
	p := DefaultPipeline()
	p.Annotators[0].Config.Code = `function extract(ctx){trace("bad",Infinity);return []}`
	e, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	in := BatchInput{ContextRoot: "/source", SourceBoundary: "/source", Paths: []string{"/source/a.mkv"}, Targets: []string{"/source/a.mkv"}}
	got, err := e.Snapshot(context.Background(), in)
	if err == nil || len(got.Diagnostics) != 1 || !strings.Contains(got.Diagnostics[0].Message, "安全数值") {
		t.Fatalf("trace validation skipped: %v %+v", err, got.Diagnostics)
	}
	for _, value := range []any{map[string]any{"": nil}, map[string]any{"z": nil, "a": nil}} {
		if _, err := resultObject(value, "known"); err == nil || !strings.Contains(err.Error(), `"`) {
			t.Fatal("unknown field accepted", err)
		}
	}
}
