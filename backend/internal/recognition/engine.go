package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.starlark.net/starlark"
	"regexp"
	"sort"
	"strings"
	"time"
)

type compiledRule struct {
	rule          Rule
	regex         *regexp.Regexp
	capture       int
	program       *starlark.Program
	sourceProgram *starlark.Program
}
type Engine struct{ rules []compiledRule }
type matchValue struct {
	value    any
	text     string
	captures map[string]string
	trace    []ScriptEvent
}

func Compile(rules []Rule) (*Engine, error) {
	// Deep-copy configuration once: callers cannot mutate a compiled invocation.
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	var copyRules []Rule
	if err = json.Unmarshal(raw, &copyRules); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	orders := map[int]bool{}
	engine := &Engine{rules: []compiledRule{}}
	for _, rule := range copyRules {
		if strings.TrimSpace(rule.ID) == "" || len(rule.ID) > 128 || ids[rule.ID] {
			return nil, errors.New("规则 ID 不能为空、重复或超过 128 字符")
		}
		ids[rule.ID] = true
		if strings.TrimSpace(rule.Name) == "" || rule.Order < 0 || orders[rule.Order] {
			return nil, errors.New("规则名称不能为空，顺序必须为非负且不重复")
		}
		orders[rule.Order] = true
		validTarget, validSource := false, false
		for _, v := range Targets() {
			validTarget = validTarget || v == rule.Target
		}
		for _, v := range Sources() {
			validSource = validSource || v == rule.Source
		}
		if !validTarget || !validSource {
			return nil, fmt.Errorf("规则 %s 的目标或来源无效", rule.Name)
		}
		if rule.Source == CleanFilename && rule.Config.SourceCode == "" {
			rule.Config.SourceCode = cleanFilenameScript
		}
		c := compiledRule{rule: rule}
		if rule.Source == CleanFilename {
			c.sourceProgram, err = compileScript(rule.ID+"-source.star", rule.Config.SourceCode)
			if err != nil {
				return nil, fmt.Errorf("规则 %s 的来源脚本无效：%w", rule.Name, err)
			}
		} else if rule.Config.SourceCode != "" {
			return nil, errors.New("来源脚本仅适用于清理后文件名来源")
		}
		switch rule.Kind {
		case "script":
			if rule.Config.Handler != "" || rule.Config.Pattern != "" || rule.Config.Capture != "" || rule.Config.Value != nil {
				return nil, errors.New("脚本规则不能包含内置处理器或正则规则配置")
			}
			if len(rule.Config.Params) > 16384 {
				return nil, errors.New("脚本参数过长")
			}
			if len(rule.Config.Params) > 0 {
				var params map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(rule.Config.Params)))
				decoder.UseNumber()
				if err := decoder.Decode(&params); err != nil || params == nil {
					return nil, errors.New("脚本参数必须是 JSON 对象")
				}
				if _, err := toStar(params); err != nil {
					return nil, err
				}
			}
			c.program, err = compileScript(rule.ID+".star", rule.Config.Code)
			if err != nil {
				return nil, fmt.Errorf("规则 %s 的脚本无效：%w", rule.Name, err)
			}
		case "regex":
			if rule.Config.Handler != "" || rule.Config.Code != "" || len(rule.Config.Params) > 0 {
				return nil, errors.New("正则规则不能包含脚本或内置处理器配置")
			}
			if rule.Config.Pattern == "" || len(rule.Config.Pattern) > 16384 {
				return nil, errors.New("正则不能为空或过长")
			}
			c.regex, err = regexp.Compile(rule.Config.Pattern)
			if err != nil {
				return nil, fmt.Errorf("规则 %s 的正则无效：%s", rule.Name, err)
			}
			if (rule.Config.Capture != "") == (rule.Config.Value != nil) {
				return nil, errors.New("正则规则必须且只能选择捕获组或固定输出值")
			}
			if rule.Config.Capture != "" {
				c.capture = c.regex.SubexpIndex(rule.Config.Capture)
				if c.capture < 1 {
					return nil, errors.New("指定的命名捕获组不存在")
				}
			} else if _, err = convert(rule.Target, rule.Config.Value); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("规则类型必须为 script 或 regex；旧内置规则请迁移为脚本")
		}
		for _, example := range rule.Examples {
			if _, err := prepare(Input{Path: example.Path, Root: example.Root}); err != nil {
				return nil, fmt.Errorf("规则 %s 的测试路径无效", rule.Name)
			}
			if example.Expected == nil {
				return nil, errors.New("测试样例必须包含预期值（未命中使用 null）")
			}
			if string(example.Expected) != "null" {
				if _, err := convert(rule.Target, example.Expected); err != nil {
					return nil, err
				}
			}
		}
		engine.rules = append(engine.rules, c)
	}
	sort.Slice(engine.rules, func(i, j int) bool { return engine.rules[i].rule.Order < engine.rules[j].rule.Order })
	return engine, nil
}
func (e *Engine) Rules() []Rule {
	rules := []Rule{}
	for _, c := range e.rules {
		rules = append(rules, c.rule)
	}
	raw, _ := json.Marshal(rules)
	var copyRules []Rule
	_ = json.Unmarshal(raw, &copyRules)
	return copyRules
}
func (e *Engine) Extract(ctx context.Context, in Input, observer Observer) (Result, error) {
	p, err := prepare(in)
	result := Result{Metadata: Metadata{Episodes: []int{}}, Steps: []Step{}}
	if err != nil {
		return result, err
	}
	for _, c := range e.rules {
		rule := c.rule
		started := time.Now()
		step := Step{RuleID: rule.ID, RuleName: rule.Name, Order: rule.Order, RuleConfig: rule, Target: rule.Target, Inputs: []string{}, Attempts: []Attempt{}, Captures: map[string]string{}, Before: result.Metadata.value(rule.Target), After: result.Metadata.value(rule.Target), StartedAt: started.UTC()}
		switch {
		case ctx.Err() != nil:
			step.Status, step.Reason = "skipped", "执行已取消，未运行此规则"
		case !rule.Enabled:
			step.Status, step.Reason = "disabled", "规则未启用"
		default:
			inputs := p.sources(rule.Source)
			step.Status, step.Reason = "not_matched", "未命中任何输入"
			if len(inputs) == 0 {
				step.Status, step.Reason = "skipped", "解析边界内没有此来源文本"
			}
			for _, text := range inputs {
				step.Inputs = append(step.Inputs, text)
				attempt := Attempt{Input: text}
				match, matchErr := c.match(ctx, text, p, result.Metadata)
				attempt.Trace = match.trace
				if match.value != nil {
					attempt.Matched = true
					attempt.MatchedText = match.text
					attempt.Captures = match.captures
					step.MatchedText, step.Captures = match.text, match.captures
					step.ExtractedValue = match.value
				}
				if matchErr == nil && match.value != nil {
					match.value, matchErr = convert(rule.Target, match.value)
				}
				if matchErr != nil {
					attempt.Error = matchErr.Error()
					step.Attempts = append(step.Attempts, attempt)
					step.Status, step.Reason = "error", matchErr.Error()
					break
				}
				step.Attempts = append(step.Attempts, attempt)
				if match.value == nil {
					continue
				}
				step.Status, step.Reason, step.ExtractedValue = "matched", "提取成功", match.value
				switch {
				case step.Before != nil && Equal(step.Before, match.value):
					step.Reason = "目标已有相同值，保留原值"
				case step.Before != nil && !rule.Overwrite:
					step.Reason = "目标已有值，未允许覆盖"
				default:
					result.Metadata.set(rule.Target, match.value)
					step.Applied = true
					if step.Before != nil {
						step.Reason = "已按配置覆盖原值"
					}
				}
				step.After = result.Metadata.value(rule.Target)
				break
			}
		}
		step.DurationMS = float64(time.Since(started).Microseconds()) / 1000
		// Snapshot the full step, including nested slices, before exposing it.
		raw, _ := json.Marshal(step)
		var snapshot Step
		_ = json.Unmarshal(raw, &snapshot)
		result.Steps = append(result.Steps, snapshot)
		if observer != nil {
			var observed Step
			_ = json.Unmarshal(raw, &observed)
			if err := observer(context.WithoutCancel(ctx), observed); err != nil {
				return result, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
}
func (c compiledRule) match(ctx context.Context, text string, p pathInput, metadata Metadata) (matchValue, error) {
	events := []ScriptEvent{}
	filename := p.filename
	directories := append([]string{}, p.directories...)
	if (c.rule.Target == Episodes || c.rule.Target == Season) && (c.program != nil || c.sourceProgram != nil) {
		protectedText := idRE.ReplaceAllString(text, " ")
		if protectedText != text {
			events = append(events, ScriptEvent{Phase: "input", Operation: "protect", Message: "TMDB ID 标签已屏蔽，不能作为季号或集数", Data: map[string]any{"before": text, "after": protectedText}})
		}
		text = protectedText
		filename = idRE.ReplaceAllString(filename, " ")
		for i := range directories {
			directories[i] = idRE.ReplaceAllString(directories[i], " ")
		}
		metadata.TMDBID = nil
	}
	values := map[string]any{"text": text, "filename": filename, "directories": directories, "hasParent": len(directories) > 0, "source": string(c.rule.Source), "metadata": metadata, "params": map[string]any{}}
	if len(c.rule.Config.Params) > 0 {
		var params any
		decoder := json.NewDecoder(strings.NewReader(string(c.rule.Config.Params)))
		decoder.UseNumber()
		_ = decoder.Decode(&params)
		values["params"] = params
	}
	if c.sourceProgram != nil {
		cleaned, err := executeScript(ctx, c.sourceProgram, values, "source")
		events = append(events, cleaned.trace...)
		if err != nil {
			return matchValue{trace: events}, err
		}
		var ok bool
		text, ok = cleaned.value.(string)
		if !ok {
			return matchValue{trace: events}, errors.New("来源清理脚本必须返回字符串")
		}
		values["text"] = text
	}
	if c.rule.Kind == "script" {
		match, err := executeScript(ctx, c.program, values, "extract")
		match.trace = append(events, match.trace...)
		return match, err
	}
	indexes := c.regex.FindStringSubmatchIndex(text)
	if indexes == nil {
		return matchValue{trace: events}, nil
	}
	start, end := indexes[0], indexes[1]
	value := any(c.rule.Config.Value)
	captures := map[string]string{}
	for i, name := range c.regex.SubexpNames() {
		if i > 0 && name != "" && indexes[2*i] >= 0 {
			captures[name] = text[indexes[2*i]:indexes[2*i+1]]
		}
	}
	if c.rule.Config.Capture != "" {
		start, end = indexes[2*c.capture], indexes[2*c.capture+1]
		if start < 0 {
			return matchValue{}, nil
		}
		value = text[start:end]
	}
	if (c.rule.Target == Episodes || c.rule.Target == Season) && protected(text, start, end) {
		return matchValue{value: value, text: text[indexes[0]:indexes[1]], captures: captures, trace: events}, errors.New("受保护的 TMDB ID 标签不能作为季号或集数")
	}
	return matchValue{value: value, text: text[indexes[0]:indexes[1]], captures: captures, trace: events}, nil
}
