package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

const maxScriptEvents = 128
const maxScriptBytes = 32768
const scriptTimeout = 250 * time.Millisecond

type scriptRun struct {
	phase     string
	events    []ScriptEvent
	truncated bool
}

func compileScript(name, code string) (*goja.Program, error) {
	if strings.TrimSpace(code) == "" || len(code) > maxScriptBytes {
		return nil, errors.New("脚本不能为空或超过 32 KiB")
	}
	tree, err := parser.ParseFile(nil, name, code, 0, parser.WithDisableSourceMaps)
	if err != nil {
		return nil, err
	}
	found := false
	for _, statement := range tree.Body {
		if decl, ok := statement.(*ast.FunctionDeclaration); ok && decl.Function.Name != nil && decl.Function.Name.Name.String() == "extract" {
			f := decl.Function
			if f.Async || f.Generator || len(f.ParameterList.List) != 1 || f.ParameterList.Rest != nil {
				return nil, errors.New("脚本必须定义同步函数 extract(ctx)，且只接受一个参数")
			}
			if _, ok := f.ParameterList.List[0].Target.(*ast.Identifier); !ok || f.ParameterList.List[0].Initializer != nil {
				return nil, errors.New("extract 必须使用单个普通参数")
			}
			found = true
		}
	}
	if !found {
		return nil, errors.New("脚本必须定义 function extract(ctx)")
	}
	// Strict mode makes writes to frozen input fail instead of silently succeeding.
	return goja.CompileAST(tree, true)
}

func (s *scriptRun) record(vm *goja.Runtime, operation, message string, data any) {
	if len(message) > 4096 {
		message = message[:4096] + "…"
	}
	event := ScriptEvent{Phase: s.phase, Operation: operation, Message: message, Data: data}
	if vm != nil {
		for _, frame := range vm.CaptureCallStack(8, nil) {
			pos := frame.Position()
			if pos.Line > 0 {
				event.Line, event.Column, event.Function = int32(pos.Line), int32(pos.Column), frame.FuncName()
				break
			}
		}
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

func scriptBuiltins(vm *goja.Runtime, run *scriptRun) {
	vm.Set("trace", func(call goja.FunctionCall) goja.Value {
		value, err := scriptValue(call.Argument(1), 0)
		if err != nil {
			panic(vm.NewGoError(err))
		}
		run.record(vm, "trace", call.Argument(0).String(), value)
		return goja.Undefined()
	})
	vm.Set("print", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, v := range call.Arguments {
			parts[i] = v.String()
		}
		run.record(vm, "print", strings.Join(parts, " "), nil)
		return goja.Undefined()
	})
	for _, operation := range []string{"reSearch", "reFindAll", "reSub"} {
		operation := operation
		vm.Set(operation, func(call goja.FunctionCall) goja.Value {
			pattern, text := call.Argument(0).String(), call.Argument(1).String()
			replacement := ""
			if operation == "reSub" {
				replacement = call.Argument(2).String()
			}
			if len(pattern) > 16384 || len(text) > 32768 || len(replacement) > 32768 {
				panic(vm.NewGoError(errors.New("正则参数过长")))
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			var output any
			switch operation {
			case "reSub":
				// Bound replacements before allocating the resulting string.
				if count := len(re.FindAllStringIndex(text, 1025)); count > 1024 || len(text)+count*len(replacement) > 65536 {
					panic(vm.NewGoError(errors.New("正则替换结果过长")))
				}
				output = re.ReplaceAllString(text, replacement)
			case "reSearch", "reFindAll":
				matches := re.FindAllStringSubmatchIndex(text, 1)
				if operation == "reFindAll" {
					matches = re.FindAllStringSubmatchIndex(text, 513)
					if len(matches) > 512 {
						panic(vm.NewGoError(errors.New("正则命中超过 512 次")))
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
					entries = append(entries, map[string]any{"text": groups[0], "groups": groups, "captures": captures, "start": len(utf16.Encode([]rune(text[:indexes[0]]))), "end": len(utf16.Encode([]rune(text[:indexes[1]])))})
				}
				if operation == "reFindAll" {
					output = entries
				} else if len(entries) > 0 {
					output = entries[0]
				}
			}
			run.record(vm, operation, "正则执行", map[string]any{"pattern": pattern, "input": text, "replacement": replacement, "output": output})
			return vm.ToValue(output)
		})
	}
}

// Only JSON data crosses the boundary: scripts never receive reflected Go objects.
const inputFactory = `(function(raw) {
 const value = JSON.parse(raw);
 function freeze(v) {
  if (v !== null && typeof v === "object") {
   Object.keys(v).forEach(k => freeze(v[k]));
   Object.freeze(v);
  }
  return v;
 }
 return freeze(value);
})`

func executeScript(ctx context.Context, program *goja.Program, values map[string]any, phase string) (matchValue, error) {
	run := &scriptRun{phase: phase, events: []ScriptEvent{}}
	vm := goja.New()
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(256)
	deadline, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()
	stop := context.AfterFunc(deadline, func() { vm.Interrupt("脚本已取消或超过 250 ms 执行时间") })
	defer stop()
	scriptBuiltins(vm, run)
	finish := func(err error) (matchValue, error) {
		run.record(nil, "error", err.Error(), nil)
		var stackError interface{ Stack() []goja.StackFrame }
		if errors.As(err, &stackError) {
			for _, frame := range stackError.Stack() {
				pos := frame.Position()
				if pos.Line > 0 && !run.truncated {
					event := &run.events[len(run.events)-1]
					event.Line, event.Column, event.Function = int32(pos.Line), int32(pos.Column), frame.FuncName()
					break
				}
			}
		}
		var exception *goja.Exception
		if errors.As(err, &exception) {
			err = errors.New(exception.String())
		}

		return matchValue{trace: run.events}, err
	}
	if ctx.Err() != nil {
		return finish(ctx.Err())
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return finish(err)
	}
	// Parse and freeze input before user code can replace built-in JSON/Object functions.
	factory, err := vm.RunString(inputFactory)
	if err != nil {
		return finish(err)
	}
	makeInput, _ := goja.AssertFunction(factory)
	input, err := makeInput(goja.Undefined(), vm.ToValue(string(raw)))
	if err != nil {
		return finish(err)
	}
	run.record(nil, "input", "脚本输入", values)
	if _, err = vm.RunProgram(program); err != nil {
		return finish(err)
	}
	extractValue, err := vm.RunString("extract")
	if err != nil {
		return finish(err)
	}
	function, ok := goja.AssertFunction(extractValue)
	if !ok {
		return finish(errors.New("extract 必须是同步脚本函数"))
	}
	returned, err := function(goja.Undefined(), input)
	if err != nil {
		return finish(err)
	}
	var value any
	// Read accessors inside a callable boundary: goja.Try alone does not catch
	// uncatchable interruptions (for example an infinite loop in a getter).
	reader, _ := goja.AssertFunction(vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value, err = scriptValue(call.Argument(0), 0)
		return goja.Undefined()
	}))
	if _, readErr := reader(goja.Undefined(), returned); readErr != nil {
		return finish(readErr)
	}

	if err != nil {
		return finish(err)
	}
	run.record(nil, "return", "脚本返回值", value)
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

// Bounded traversal rejects promises, functions, cyclic and non-finite results.
func scriptValue(value goja.Value, depth int) (any, error) {
	budget := 65536
	return readScriptValue(value, depth, &budget)
}
func readScriptValue(value goja.Value, depth int, budget *int) (any, error) {
	*budget -= 16
	if *budget < 0 {
		return nil, errors.New("脚本结果总量超过 64 KiB")
	}
	if depth > 12 {
		return nil, errors.New("脚本结果嵌套过深")
	}
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil, nil
	}
	if object, ok := value.(*goja.Object); ok {
		switch object.ClassName() {
		case "Array":
			length := object.Get("length").ToInteger()
			if length > 1024 {
				return nil, errors.New("脚本列表过长")
			}
			result := make([]any, 0, length)
			for i := int64(0); i < length; i++ {
				child, err := readScriptValue(object.Get(strconv.FormatInt(i, 10)), depth+1, budget)
				if err != nil {
					return nil, err
				}
				result = append(result, child)
			}
			return result, nil
		case "Object":
			if _, ok := goja.AssertFunction(object.Get("then")); ok {
				return nil, errors.New("脚本不支持异步返回值")
			}
			keys := object.Keys()
			if len(keys) > 1024 {
				return nil, errors.New("脚本对象过长")
			}
			result := map[string]any{}
			for _, key := range keys {
				*budget -= len(key)
				if *budget < 0 {
					return nil, errors.New("脚本结果总量超过 64 KiB")
				}
				if len(key) > 32768 {
					return nil, errors.New("脚本对象键过长")
				}
				child, err := readScriptValue(object.Get(key), depth+1, budget)
				if err != nil {
					return nil, err
				}
				result[key] = child
			}
			return result, nil
		default:
			return nil, fmt.Errorf("不支持脚本返回类型 %s", object.ClassName())
		}
	}
	switch v := value.Export().(type) {
	case string:
		*budget -= len(v)
		if *budget < 0 {
			return nil, errors.New("脚本结果总量超过 64 KiB")
		}
		if len(v) > 32768 {
			return nil, errors.New("脚本结果字符串过长")
		}
		return v, nil
	case bool:
		return v, nil
	case int64:
		if v > 9007199254740991 || v < -9007199254740991 {
			return nil, errors.New("脚本数字必须为有限安全数值")
		}
		return json.Number(strconv.FormatInt(v, 10)), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 9007199254740991 {
			return nil, errors.New("脚本数字必须为有限安全数值")
		}
		return json.Number(strconv.FormatFloat(v, 'f', -1, 64)), nil
	default:
		return nil, errors.New("脚本只能返回 JSON 数据")
	}
}
