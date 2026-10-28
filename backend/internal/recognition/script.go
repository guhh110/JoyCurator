package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

const maxScriptSteps = 100000
const maxScriptEvents = 128
const maxScriptBytes = 32768

type scriptRun struct {
	phase     string
	events    []ScriptEvent
	truncated bool
}

func compileScript(name, code string) (*starlark.Program, error) {
	if strings.TrimSpace(code) == "" || len(code) > maxScriptBytes {
		return nil, errors.New("脚本不能为空或超过 32 KiB")
	}
	file, program, err := starlark.SourceProgramOptions(&syntax.FileOptions{}, name, code, scriptBuiltins(nil).Has)
	if err != nil {
		return nil, err
	}
	found := false
	for _, statement := range file.Stmts {
		if _, ok := statement.(*syntax.LoadStmt); ok {
			return nil, errors.New("脚本不支持 load；请将完整规则写在当前脚本中")
		}
		if def, ok := statement.(*syntax.DefStmt); ok && def.Name.Name == "extract" {
			if len(def.Params) != 1 {
				return nil, errors.New("脚本必须定义 extract(ctx)，且只接受一个参数")
			}
			if _, ok := def.Params[0].(*syntax.Ident); !ok {
				return nil, errors.New("extract 的参数必须为 ctx")
			}
			found = true
		}
	}
	if !found {
		return nil, errors.New("脚本必须定义 extract(ctx)")
	}
	return program, nil
}
func (s *scriptRun) record(thread *starlark.Thread, operation, message string, data any) {
	if len(message) > 4096 {
		message = message[:4096] + "…"
	}
	event := ScriptEvent{Phase: s.phase, Operation: operation, Message: message, Data: data}
	if thread.CallStackDepth() > 1 {
		frame := thread.CallFrame(1)
		event.Line = frame.Pos.Line
		event.Column = frame.Pos.Col
		event.Function = frame.Name
	}
	if len(s.events) >= maxScriptEvents {
		if !s.truncated {
			s.events = append(s.events, ScriptEvent{Phase: s.phase, Operation: "limit", Message: "日志已达 128 条，后续日志省略"})
			s.truncated = true
		}
		return
	}
	raw, err := json.Marshal(data)
	if err != nil || len(raw) > 8192 {
		event.Data = "内容过长，已省略"
	}
	s.events = append(s.events, event)
}
func scriptBuiltins(run *scriptRun) starlark.StringDict {
	result := starlark.StringDict{}
	result["trace"] = starlark.NewBuiltin("trace", func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var message string
		var data starlark.Value = starlark.None
		if err := starlark.UnpackArgs("trace", args, kwargs, "message", &message, "data?", &data); err != nil {
			return nil, err
		}
		value, err := fromStar(data, 0)
		if err != nil {
			return nil, err
		}
		run.record(thread, "trace", message, value)
		return starlark.None, nil
	})
	for _, operation := range []string{"re_search", "re_findall", "re_sub"} {
		operation := operation
		result[operation] = starlark.NewBuiltin(operation, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var pattern, text, replacement string
			names := []any{"pattern", &pattern, "text", &text}
			if operation == "re_sub" {
				names = append(names, "replacement", &replacement)
			}
			if err := starlark.UnpackArgs(operation, args, kwargs, names...); err != nil {
				return nil, err
			}
			if len(pattern) > 16384 || len(text) > 32768 || len(replacement) > 32768 {
				return nil, errors.New("正则参数过长")
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, err
			}
			var output any
			switch operation {
			case "re_sub":
				// Bound replacements before allocating the resulting string.
				if count := len(re.FindAllStringIndex(text, 1025)); count > 1024 || len(text)+count*len(replacement) > 65536 {
					return nil, errors.New("正则替换结果过长")
				}
				output = re.ReplaceAllString(text, replacement)
			case "re_search", "re_findall":
				matches := re.FindAllStringSubmatchIndex(text, 1)
				if operation == "re_findall" {
					matches = re.FindAllStringSubmatchIndex(text, 513)
					if len(matches) > 512 {
						return nil, errors.New("正则命中超过 512 次")
					}
				}
				entries := []any{}
				for _, indexes := range matches {
					groups := []any{}
					captures := map[string]any{}
					for index, name := range re.SubexpNames() {
						value := ""
						if indexes[2*index] >= 0 {
							value = text[indexes[2*index]:indexes[2*index+1]]
						}
						groups = append(groups, value)
						if name != "" {
							captures[name] = value
						}
					}
					entries = append(entries, map[string]any{"text": groups[0], "groups": groups, "captures": captures, "start": indexes[0], "end": indexes[1]})
				}
				if operation == "re_findall" {
					output = entries
				} else if len(entries) > 0 {
					output = entries[0]
				}
			}
			run.record(thread, operation, "正则执行", map[string]any{"pattern": pattern, "input": text, "replacement": replacement, "output": output})
			return toStar(output)
		})
	}
	return result
}
func executeScript(ctx context.Context, program *starlark.Program, values map[string]any, phase string) (matchValue, error) {
	run := &scriptRun{phase: phase, events: []ScriptEvent{}}
	thread := &starlark.Thread{Name: "path-extraction"}
	thread.SetMaxExecutionSteps(maxScriptSteps)
	thread.OnMaxSteps = func(t *starlark.Thread) { t.Cancel("脚本超过 100000 执行步数") }
	thread.Print = func(t *starlark.Thread, message string) { run.record(t, "print", message, nil) }
	deadline, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	stop := context.AfterFunc(deadline, func() { thread.Cancel("脚本已取消或超过 250 ms 执行时间") })
	defer stop()
	finish := func(err error) (matchValue, error) {
		previous := len(run.events)
		run.record(thread, "error", err.Error(), nil)
		if eval, ok := err.(*starlark.EvalError); ok {
			if len(run.events) > previous {
				for depth := 0; depth < len(eval.CallStack); depth++ {
					frame := eval.CallStack.At(depth)
					if frame.Pos.Line > 0 {
						event := &run.events[len(run.events)-1]
						event.Line = frame.Pos.Line
						event.Column = frame.Pos.Col
						event.Function = frame.Name
						break
					}
				}
			}
			err = errors.New(eval.Backtrace())
		}
		return matchValue{trace: run.events}, err
	}
	run.record(thread, "input", "脚本输入", values)
	globals, err := program.Init(thread, scriptBuiltins(run))
	if err != nil {
		return finish(err)
	}
	function, ok := globals["extract"].(*starlark.Function)
	if !ok {
		return finish(errors.New("extract 必须是脚本函数"))
	}
	dictionary := starlark.StringDict{}
	for key, value := range values {
		item, e := toStar(value)
		if e != nil {
			return finish(e)
		}
		item.Freeze()
		dictionary[key] = item
	}
	input := starlarkstruct.FromStringDict(starlark.String("ctx"), dictionary)
	input.Freeze()
	returned, err := starlark.Call(thread, function, starlark.Tuple{input}, nil)
	if err != nil {
		return finish(err)
	}
	value, err := fromStar(returned, 0)
	if err != nil {
		return finish(err)
	}
	run.record(thread, "return", "脚本返回值", value)
	match := matchValue{value: value, trace: run.events, captures: map[string]string{}}
	if fields, ok := value.(map[string]any); ok {
		for key := range fields {
			if key != "value" && key != "matchedText" && key != "captures" {
				return finish(fmt.Errorf("脚本只能返回本规则目标值，不支持结果字段 %s", key))
			}
		}
		var exists bool
		match.value, exists = fields["value"]
		if !exists {
			return finish(errors.New("结果对象必须包含 value"))
		}
		if text, exists := fields["matchedText"]; exists {
			var ok bool
			match.text, ok = text.(string)
			if !ok {
				return finish(errors.New("matchedText 必须是字符串"))
			}
		}
		if raw, exists := fields["captures"]; exists {
			captures, ok := raw.(map[string]any)
			if !ok {
				return finish(errors.New("captures 必须是字符串映射"))
			}
			for key, value := range captures {
				text, ok := value.(string)
				if !ok {
					return finish(errors.New("捕获内容必须是字符串"))
				}
				match.captures[key] = text
			}
		}
	}
	return match, nil
}
func toStar(value any) (starlark.Value, error) {
	switch value := value.(type) {
	case nil:
		return starlark.None, nil
	case string:
		return starlark.String(value), nil
	case bool:
		return starlark.Bool(value), nil
	case int:
		return starlark.MakeInt(value), nil
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return starlark.MakeInt64(n), nil
		}
		return nil, errors.New("脚本参数数字必须为整数")
	case []string:
		items := []starlark.Value{}
		for _, value := range value {
			items = append(items, starlark.String(value))
		}
		return starlark.Tuple(items), nil
	case []any:
		items := []starlark.Value{}
		for _, value := range value {
			item, err := toStar(value)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return starlark.NewList(items), nil
	case map[string]any:
		dictionary := starlark.NewDict(len(value))
		for key, value := range value {
			item, err := toStar(value)
			if err != nil {
				return nil, err
			}
			if err = dictionary.SetKey(starlark.String(key), item); err != nil {
				return nil, err
			}
		}
		return dictionary, nil
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err = decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		return toStar(decoded)
	}
}
func fromStar(value starlark.Value, depth int) (any, error) {
	if depth > 12 {
		return nil, errors.New("脚本结果嵌套过深")
	}
	switch value := value.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.String:
		if len(value) > 32768 {
			return nil, errors.New("脚本结果字符串过长")
		}
		return string(value), nil
	case starlark.Bool:
		return bool(value), nil
	case starlark.Int:
		if _, ok := value.Int64(); !ok {
			return nil, errors.New("脚本整数超出范围")
		}
		return json.Number(value.String()), nil
	case *starlark.Dict:
		if value.Len() > 1024 {
			return nil, errors.New("脚本结果过长")
		}
		result := map[string]any{}
		for _, item := range value.Items() {
			key, ok := starlark.AsString(item[0])
			if !ok {
				return nil, errors.New("脚本对象键必须是字符串")
			}
			child, err := fromStar(item[1], depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = child
		}
		return result, nil
	case starlark.Iterable:
		if starlark.Len(value) > 1024 {
			return nil, errors.New("脚本列表过长")
		}
		result := []any{}
		iterator := value.Iterate()
		defer iterator.Done()
		var item starlark.Value
		for iterator.Next(&item) {
			if len(result) >= 1024 {
				return nil, errors.New("脚本列表过长")
			}
			child, err := fromStar(item, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, child)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("不支持脚本返回类型 %s", value.Type())
	}
}
