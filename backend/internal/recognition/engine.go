package recognition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dop251/goja"
)

type compiledRule struct {
	rule    Rule
	program *goja.Program
	regex   *regexp.Regexp
	capture int
}
type Engine struct {
	pipeline   Pipeline
	tokenizer  compiledRule
	annotators []compiledRule
	resolver   compiledRule
}
type matchValue struct {
	value any
	trace []ScriptEvent
}

func clone[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func strict(value any, out any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err = d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("额外 JSON 内容")
	}
	return nil
}
func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
func Compile(p Pipeline) (*Engine, error) {
	p = clone(p)
	if p.Annotators == nil {
		return nil, errors.New("请提供完整 pipeline：tokenizer、annotators、resolver")
	}
	e := &Engine{pipeline: p}
	ids := map[string]bool{}
	orders := map[int]bool{}
	for i, r := range p.Rules() {
		for _, ex := range r.Examples {
			if len(ex.Paths) == 0 || len(ex.Paths) > 2000 || !json.Valid(ex.Expected) {
				return nil, errors.New("规则示例必须提供路径数组与合法预期 JSON")
			}
			for _, path := range ex.Paths {
				if _, err := prepare(Input{Path: path, Root: ex.Root}); err != nil {
					return nil, err
				}
			}
		}
		if r.ID == "" || len(r.ID) > 128 || ids[r.ID] || strings.TrimSpace(r.Name) == "" {
			return nil, errors.New("规则 ID 或名称无效、重复")
		}
		ids[r.ID] = true
		stage := "annotate"
		if i == 0 {
			stage = "tokenize"
		}
		if i == len(p.Annotators)+1 {
			stage = "resolve"
		}
		if r.Stage != stage {
			return nil, fmt.Errorf("规则 %s 阶段应为 %s", r.ID, stage)
		}
		if stage != "annotate" && (!r.Enabled || r.Kind != "script" || r.Target != "" || r.Source != "") {
			return nil, errors.New("分词和裁决必须为启用的脚本，不指定身份和文本来源")
		}
		if stage == "annotate" {
			if !contains(Roles(), r.Target) || !contains(Sources(), r.Source) || r.Order < 0 || orders[r.Order] {
				return nil, errors.New("标注身份、来源或顺序无效")
			}
			orders[r.Order] = true
		}
		if len(r.Config.Params) > 16384 {
			return nil, errors.New("脚本参数过长")
		}
		if len(r.Config.Params) > 0 {
			var params map[string]any
			if err := strict(r.Config.Params, &params); err != nil || params == nil {
				return nil, errors.New("参数必须为 JSON 对象")
			}
		}
		c := compiledRule{rule: r}
		var err error
		switch r.Kind {
		case "script":
			if r.Config.Pattern != "" || r.Config.Capture != "" || r.Config.Value != nil {
				return nil, errors.New("脚本不能包含正则配置")
			}
			c.program, err = compileScript(r.ID+".js", r.Config.Code)
		case "regex":
			if stage != "annotate" || r.Config.Code != "" || len(r.Config.Params) > 0 || len(r.Config.Pattern) == 0 || len(r.Config.Pattern) > 16384 {
				return nil, errors.New("正则标注配置无效")
			}
			c.regex, err = regexp.Compile(r.Config.Pattern)
			if err == nil {
				if (r.Config.Capture != "") == (r.Config.Value != nil) {
					return nil, errors.New("正则标注必须选择捕获组或固定值")
				}
				if r.Config.Capture == "" {
					var value any
					if err := strict(r.Config.Value, &value); err != nil {
						return nil, err
					}
					if contains(Targets(), r.Target) {
						if _, err := convert(r.Target, value); err != nil {
							return nil, err
						}
					} else if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
						return nil, errors.New("辅助身份固定值必须为非空文本")
					}
				}
				if r.Config.Capture != "" {
					c.capture = c.regex.SubexpIndex(r.Config.Capture)
					if c.capture < 1 {
						return nil, errors.New("命名捕获组不存在")
					}
				}
			}
		default:
			return nil, errors.New("规则类型必须为 script 或 regex")
		}
		if err != nil {
			return nil, err
		}
		if stage == "tokenize" {
			e.tokenizer = c
		} else if stage == "resolve" {
			e.resolver = c
		} else {
			e.annotators = append(e.annotators, c)
		}
	}
	sort.Slice(e.annotators, func(i, j int) bool {
		a, b := e.annotators[i].rule, e.annotators[j].rule
		if (a.Target == Title) != (b.Target == Title) {
			return a.Target != Title
		}
		return a.Order < b.Order
	})
	e.pipeline.Annotators = []Rule{}
	for _, c := range e.annotators {
		e.pipeline.Annotators = append(e.pipeline.Annotators, c.rule)
	}
	return e, nil
}
func (e *Engine) Rules() Pipeline { return clone(e.pipeline) }
func fileIdentity(d Document) map[string]any {
	return map[string]any{"fileId": d.FileID, "path": d.Path, "directory": d.Directory}
}
func makeDocument(p pathInput, id string) Document {
	d := Document{FileID: id, Path: p.path, Directory: pathDir(p.path), Sources: []TextSource{{ID: "name", Kind: Filename, Text: p.filename, Depth: 0}}, Tokens: []Token{}, Candidates: []Candidate{}}
	for i, t := range p.directories {
		d.Sources = append(d.Sources, TextSource{ID: fmt.Sprintf("dir-%d", i), Kind: Directories, Text: t, Depth: i + 1})
	}
	return d
}
func utf16len(s string) int { return len(utf16.Encode([]rune(s))) }
func slice16(s string, a, b int) (string, error) {
	r := utf16.Encode([]rune(s))
	if a < 0 || b <= a || b > len(r) {
		return "", errors.New("原文范围越界")
	}
	if a > 0 && r[a] >= 0xdc00 && r[a] <= 0xdfff || b < len(r) && r[b] >= 0xdc00 && r[b] <= 0xdfff {
		return "", errors.New("范围不能切开 Unicode 字符")
	}
	return string(utf16.Decode(r[a:b])), nil
}
func source(d Document, id string) (TextSource, bool) {
	for _, s := range d.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return TextSource{}, false
}
func validateTokens(d Document, tokens []Token) error {
	if tokens == nil || len(tokens) > 1024 {
		return errors.New("分词结果必须为数组且不超过 1024 项")
	}
	byID := map[string]Token{}
	children := map[string][]Token{}
	for _, t := range tokens {
		if t.ID == "" || len(t.ID) > 128 || byID[t.ID].ID != "" || !contains([]string{"text", "number", "separator", "group"}, t.Kind) {
			return errors.New("片段 ID、类型无效或重复")
		}
		s, ok := source(d, t.SourceID)
		text, err := slice16(s.Text, t.Start, t.End)
		if !ok || err != nil || text != t.Text {
			return errors.New("片段原文或范围无效")
		}
		byID[t.ID] = t
		key := t.SourceID + "/"
		if t.ParentID != nil {
			key += *t.ParentID
		}
		children[key] = append(children[key], t)
	}
	for _, t := range tokens {
		p := t
		depth := 0
		for p.ParentID != nil {
			parent, ok := byID[*p.ParentID]
			depth++
			if !ok || parent.Kind != "group" || parent.SourceID != t.SourceID || parent.Start > p.Start || parent.End < p.End || depth > 16 {
				return errors.New("分组层级无效或超过 16 层")
			}
			p = parent
		}
	}
	for _, list := range children {
		sort.Slice(list, func(i, j int) bool { return list[i].Start < list[j].Start })
		for i := 1; i < len(list); i++ {
			if list[i].Start < list[i-1].End {
				return errors.New("同层片段不得重叠")
			}
		}
	}
	for _, s := range d.Sources {
		pos := 0
		list := children[s.ID+"/"]
		sort.Slice(list, func(i, j int) bool { return list[i].Start < list[j].Start })
		for _, t := range list {
			if t.Start != pos {
				return errors.New("分词不能丢弃原文")
			}
			pos = t.End
		}
		if pos != utf16len(s.Text) {
			return errors.New("分词未覆盖完整来源")
		}
	}
	return nil
}

type proposal struct {
	Value    any    `json:"value"`
	Spans    []Span `json:"spans"`
	Strength string `json:"strength"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

func candidates(d Document, r Rule, value any) ([]Candidate, error) {
	var items []proposal
	if err := strict(value, &items); err != nil {
		return nil, err
	}
	if items == nil || len(items) > 512 || len(items)+len(d.Candidates) > 1024 {
		return nil, errors.New("候选数量无效或超限")
	}
	out := []Candidate{}
	for i, p := range items {
		if len(p.Spans) == 0 || !contains([]string{"explicit", "contextual", "heuristic"}, p.Strength) || !contains([]string{"valid", "invalid"}, p.Status) || strings.TrimSpace(p.Reason) == "" {
			return nil, errors.New("候选缺少范围、依据或状态")
		}
		for _, span := range p.Spans {
			if r.Source != AllSources {
				src, ok := source(d, span.SourceID)
				if !ok || src.Kind != r.Source {
					return nil, errors.New("候选超出规则来源")
				}
			}
			s, ok := source(d, span.SourceID)
			_, err := slice16(s.Text, span.Start, span.End)
			if !ok || err != nil {
				return nil, errors.New("候选范围越界")
			}
		}
		v := p.Value
		if p.Status == "valid" {
			var err error
			v, err = convert(r.Target, v)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, Candidate{ID: fmt.Sprintf("%s:%d", r.ID, i), RuleID: r.ID, Role: r.Target, Value: v, Spans: p.Spans, Strength: p.Strength, Status: p.Status, Reason: p.Reason})
	}
	return out, nil
}
func (c compiledRule) run(ctx context.Context, values map[string]any, d *Document) (matchValue, error) {
	if d != nil && c.rule.Stage == "annotate" && c.rule.Source != AllSources {
		filtered := *d
		filtered.Sources = []TextSource{}
		filtered.Tokens = []Token{}
		filtered.Candidates = []Candidate{}
		allowed := map[string]bool{}
		for _, s := range d.Sources {
			if s.Kind == c.rule.Source {
				allowed[s.ID] = true
				filtered.Sources = append(filtered.Sources, s)
			}
		}
		for _, t := range d.Tokens {
			if allowed[t.SourceID] {
				filtered.Tokens = append(filtered.Tokens, t)
			}
		}
		for _, candidate := range d.Candidates {
			ok := true
			for _, span := range candidate.Spans {
				if !allowed[span.SourceID] {
					ok = false
				}
			}
			if ok {
				filtered.Candidates = append(filtered.Candidates, candidate)
			}
		}
		values["file"] = fileIdentity(filtered)
		values["sources"] = filtered.Sources
		values["tokens"] = filtered.Tokens
		values["candidates"] = filtered.Candidates
	}
	values["params"] = map[string]any{}
	if len(c.rule.Config.Params) > 0 {
		var p any
		_ = json.Unmarshal(c.rule.Config.Params, &p)
		values["params"] = p
	}
	if c.program != nil {
		return executeScript(ctx, c.program, values, c.rule.Stage)
	}
	items := []proposal{}
	for _, s := range d.Sources {
		if c.rule.Source != AllSources && s.Kind != c.rule.Source {
			continue
		}
		for _, token := range d.Tokens {
			if token.SourceID != s.ID || (token.Kind != "text" && token.Kind != "number") {
				continue
			}
			matches := c.regex.FindAllStringSubmatchIndex(token.Text, 513)
			if len(matches)+len(items) > 512 {
				return matchValue{}, errors.New("正则命中超过 512 次")
			}
			for _, m := range matches {
				a, b := m[0], m[1]
				v := any(c.rule.Config.Value)
				if c.capture > 0 {
					a, b = m[c.capture*2], m[c.capture*2+1]
					if a < 0 || b <= a {
						continue
					}
					v = token.Text[a:b]
				}
				if b <= a {
					continue
				}
				items = append(items, proposal{Value: v, Spans: []Span{{s.ID, token.Start + utf16len(token.Text[:a]), token.Start + utf16len(token.Text[:b])}}, Strength: "explicit", Status: "valid", Reason: "正则命中"})
			}
		}
	}
	return matchValue{value: items, trace: []ScriptEvent{}}, nil
}
func (e *Engine) ExtractBatch(ctx context.Context, in BatchInput, observer Observer) (BatchResult, error) {
	result := BatchResult{Context: in, Tree: []TreeNode{}, Documents: []Document{}, Files: []FileResult{}, Steps: []Step{}}
	if len(in.Paths) == 0 || len(in.Paths) > 2000 || len(in.Targets) == 0 {
		return result, errors.New("目录上下文需要 1–2000 个视频与非空更新目标")
	}
	in.Paths = append([]string{}, in.Paths...)
	sort.Strings(in.Paths)
	seen := map[string]bool{}
	target := map[string]bool{}
	for _, p := range in.Targets {
		prepared, err := prepare(Input{Path: p, Root: in.SourceBoundary})
		if err != nil {
			return result, err
		}
		if target[prepared.path] {
			return result, errors.New("目标文件重复")
		}
		target[prepared.path] = true
	}
	first, err := prepare(Input{Path: in.Paths[0], Root: in.SourceBoundary})
	if err != nil {
		return result, err
	}
	in.SourceBoundary = first.root
	if in.ContextRoot == "" {
		in.ContextRoot = first.root
	}
	in.ContextRoot = pathClean(in.ContextRoot)
	if _, err := prepare(Input{Path: pathJoin(in.ContextRoot, "snapshot"), Root: in.SourceBoundary}); err != nil {
		return result, errors.New("上下文目录必须在解析边界内")
	}
	in.Paths = []string{}
	in.Targets = []string{}
	for p := range target {
		in.Targets = append(in.Targets, p)
	}
	sort.Strings(in.Targets)
	paths := []string{}
	for _, rawPath := range result.Context.Paths {
		p, err := prepare(Input{Path: rawPath, Root: in.SourceBoundary})
		if err != nil {
			return result, err
		}
		paths = append(paths, p.path)
	}
	sort.Strings(paths)
	tree := map[string]*TreeNode{}
	for i, path := range paths {
		p, err := prepare(Input{Path: path, Root: in.SourceBoundary})
		if err != nil {
			return result, err
		}
		if seen[p.path] {
			return result, errors.New("上下文文件重复")
		}
		seen[p.path] = true
		in.Paths = append(in.Paths, p.path)
		if !pathWithin(in.ContextRoot, p.path) {
			return result, errors.New("文件不在上下文目录内")
		}
		d := makeDocument(p, fmt.Sprintf("file-%d", i))
		result.Documents = append(result.Documents, d)
		for dir := d.Directory; ; dir = pathDir(dir) {
			if tree[dir] == nil {
				tree[dir] = &TreeNode{Path: dir, Parent: pathDir(dir), FileIDs: []string{}}
			}
			if dir == d.Directory {
				tree[dir].FileIDs = append(tree[dir].FileIDs, d.FileID)
			}
			if dir == in.ContextRoot {
				tree[dir].Parent = ""
				break
			}
			if dir == pathDir(dir) {
				return result, errors.New("目录边界无效")
			}
		}
	}
	for p := range target {
		if !seen[p] {
			return result, errors.New("目标文件不在上下文")
		}
	}
	for _, v := range tree {
		result.Tree = append(result.Tree, *v)
	}
	sort.Slice(result.Tree, func(i, j int) bool { return result.Tree[i].Path < result.Tree[j].Path })
	result.Context = in
	snapshots := map[string]bool{}
	var observerFailure error
	emit := func(c compiledRule, d *Document, values map[string]any, apply func(any) (any, error)) error {
		r := c.rule
		step := Step{RuleID: r.ID, RuleName: r.Name, Order: r.Order, Stage: r.Stage, RuleConfig: r, Target: r.Target, Inputs: []string{}, Attempts: []Attempt{}, Captures: map[string]string{}, StartedAt: time.Now().UTC(), Status: "matched", Before: map[string]any{"stage": r.Stage}}
		if r.Stage == "resolve" {
			step.Before = map[string]any{"fileCount": len(result.Documents), "targetCount": len(in.Targets)}
		}
		if r.Stage == "tokenize" {
			step.Before = map[string]any{"document": *d}
			if len(result.Steps) == 0 {
				step.Before = map[string]any{"document": *d, "context": in, "tree": result.Tree}
			}
		}
		if d != nil {
			step.FileID = d.FileID
			step.Inputs = []string{d.Path}
		}
		if snapshots[r.ID] {
			step.RuleConfig = Rule{}
		} else {
			snapshots[r.ID] = true
		}
		var err error
		if ctx.Err() != nil {
			step.Status = "skipped"
			err = ctx.Err()
		} else if !r.Enabled {
			step.Status = "disabled"
		} else {
			m, runErr := c.run(ctx, values, d)
			err = runErr
			attempt := Attempt{Matched: err == nil, Trace: m.trace}
			if err == nil {
				step.ExtractedValue, err = apply(m.value)
				if err == nil {
					step.Applied = true
					step.After = nil
				}
			}
			if err != nil {
				attempt.Error = err.Error()
			}
			step.Attempts = append(step.Attempts, attempt)
		}
		if err != nil && step.Status != "skipped" {
			step.Status = "error"
			step.Reason = err.Error()
		}
		step.DurationMS = float64(time.Since(step.StartedAt).Microseconds()) / 1000
		step = clone(step)
		result.Steps = append(result.Steps, step)
		if observer != nil {
			if observed := observer(context.WithoutCancel(ctx), clone(step)); observed != nil {
				observerFailure = observed
				return observed
			}
		}
		return err
	}
	var failures []error
	for i := range result.Documents {
		d := &result.Documents[i]
		err := emit(e.tokenizer, d, map[string]any{"file": fileIdentity(*d), "sources": d.Sources}, func(v any) (any, error) {
			var tokens []Token
			if err := strict(v, &tokens); err != nil {
				return nil, err
			}
			if err := validateTokens(*d, tokens); err != nil {
				return nil, err
			}
			d.Tokens = tokens
			return tokens, nil
		})
		if err != nil {
			return result, err
		}
		for _, c := range e.annotators {
			err = emit(c, d, map[string]any{"file": fileIdentity(*d), "sources": d.Sources, "tokens": d.Tokens, "candidates": d.Candidates}, func(v any) (any, error) {
				items, err := candidates(*d, c.rule, v)
				if err == nil {
					d.Candidates = append(d.Candidates, items...)
				}
				return items, err
			})
			if observerFailure != nil {
				return result, observerFailure
			}
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", c.rule.ID, err))
			}
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
		}
	}
	if len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	ids := []string{}
	for _, d := range result.Documents {
		if target[d.Path] {
			ids = append(ids, d.FileID)
		}
	}
	// The shared context snapshot is recorded once, not on every file step.
	values := map[string]any{"contextRoot": in.ContextRoot, "sourceBoundary": in.SourceBoundary, "tree": result.Tree, "files": result.Documents, "targetFileIds": ids}
	err = emit(e.resolver, nil, values, func(v any) (any, error) {
		var files []FileResult
		var rawFiles []map[string]json.RawMessage
		if err := strict(v, &rawFiles); err != nil {
			return nil, err
		}
		for _, f := range rawFiles {
			var fields map[string]json.RawMessage
			if err := strict(f["metadata"], &fields); err != nil {
				return nil, err
			}
			if len(fields) != len(Targets()) {
				return nil, errors.New("裁决元数据必须提供完整六个字段")
			}
		}
		if err := strict(v, &files); err != nil {
			return nil, err
		}
		if err := validateResults(result.Documents, ids, files); err != nil {
			return nil, err
		}
		result.Files = files
		return files, nil
	})
	return result, err
}
func validateResults(docs []Document, targets []string, files []FileResult) error {
	if len(files) != len(targets) {
		return errors.New("裁决必须覆盖全部目标文件")
	}
	byID := map[string]Document{}
	refs := map[string]Candidate{}
	for _, d := range docs {
		byID[d.FileID] = d
		for _, c := range d.Candidates {
			refs[d.FileID+"/"+c.ID] = c
		}
	}
	seen := map[string]bool{}
	for i := range files {
		f := &files[i]
		if !contains(targets, f.FileID) || seen[f.FileID] {
			return errors.New("裁决包含额外或重复文件")
		}
		seen[f.FileID] = true
		f.Path = byID[f.FileID].Path
		if f.Metadata.Episodes == nil {
			return errors.New("集数未知必须为 []")
		}
		for key, d := range f.Decisions {
			if !contains(Targets(), key) || d.Reason == "" || len(d.Candidates) == 0 {
				return errors.New("裁决依据无效")
			}
			for _, ref := range d.Candidates {
				c, ok := refs[ref.FileID+"/"+ref.CandidateID]
				if !ok || c.Status != "valid" {
					return errors.New("裁决引用不存在或非法候选")
				}
			}
		}
		for _, t := range Targets() {
			v := f.Metadata.value(t)
			if v == nil {
				continue
			}
			if _, err := convert(t, v); err != nil {
				return err
			}
			if _, ok := f.Decisions[t]; !ok {
				return fmt.Errorf("%s 缺少裁决依据", t)
			}
		}
	}
	return nil
}
func (e *Engine) Extract(ctx context.Context, in Input, observer Observer) (Result, error) {
	prepared, prepErr := prepare(in)
	if prepErr != nil {
		return Result{}, prepErr
	}
	batch, err := e.ExtractBatch(ctx, BatchInput{ContextRoot: pathDir(prepared.path), SourceBoundary: in.Root, Paths: []string{in.Path}, Targets: []string{in.Path}}, observer)
	out := Result{Metadata: Metadata{Episodes: []int{}}, Steps: batch.Steps}
	if len(batch.Files) > 0 {
		out.Metadata = batch.Files[0].Metadata
	}
	return out, err
}
