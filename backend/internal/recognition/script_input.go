package recognition

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/dop251/goja"
)

// inputBuilder creates only native JS data. In particular, ToValue never sees
// a Go collection or struct, so no reflected host object crosses the boundary.
// Byte accounting matches encoding/json (including HTML and Unicode escaping).
type inputBuilder struct {
	ctx          context.Context
	vm           *goja.Runtime
	freeze       goja.Callable
	bytes, limit int
}
type inputField struct {
	name  string
	value any
}

func (b *inputBuilder) add(n int) error {
	b.bytes += n
	if b.bytes > b.limit {
		return errors.New("脚本输入超过阶段大小限制")
	}
	return b.ctx.Err()
}
func jsonStringSize(s string) int {
	n := 2
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '"', '\\', '\n', '\r', '\t', '\b', '\f':
				n += 2
			case '<', '>', '&':
				n += 6
			default:
				if c < 0x20 {
					n += 6
				} else {
					n++
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			n += 6
		} else if r == '\u2028' || r == '\u2029' {
			n += 6
		} else {
			n += size
		}
		i += size
	}
	return n
}
func jsonFloatSize(n float64) int {
	var scratch [32]byte
	format := byte('f')
	a := math.Abs(n)
	if a != 0 && (a < 1e-6 || a >= 1e21) {
		format = 'e'
	}
	s := strconv.AppendFloat(scratch[:0], n, format, -1, 64)
	size := len(s)
	if format == 'e' && size >= 4 && s[size-4] == 'e' && s[size-3] == '-' && s[size-2] == '0' {
		size--
	}
	return size
}
func (b *inputBuilder) sealed(o *goja.Object) (goja.Value, error) {
	_, err := b.freeze(goja.Undefined(), o)
	return o, err
}
func (b *inputBuilder) object(fields ...inputField) (goja.Value, error) {
	if err := b.add(2 + max(0, len(fields)-1)); err != nil {
		return nil, err
	}
	o := b.vm.NewObject()
	for _, field := range fields {
		if err := b.add(jsonStringSize(field.name) + 1); err != nil {
			return nil, err
		}
		value, err := b.value(field.value)
		if err != nil {
			return nil, err
		}
		// Define an own data property, including for "__proto__". Set would invoke
		// the inherited legacy setter and would differ from JSON.parse.
		if err = o.DefineDataProperty(field.name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return nil, err
		}
	}
	return b.sealed(o)
}
func inputArray[T any](b *inputBuilder, items []T) (goja.Value, error) {
	if items == nil {
		if err := b.add(4); err != nil {
			return nil, err
		}
		return goja.Null(), nil
	}
	if err := b.add(2 + max(0, len(items)-1)); err != nil {
		return nil, err
	}
	values := make([]interface{}, len(items))
	for i, v := range items {
		value, err := b.value(v)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return b.sealed(b.vm.NewArray(values...))
}
func inputMap[T any](b *inputBuilder, values map[string]T) (goja.Value, error) {
	if values == nil {
		if err := b.add(4); err != nil {
			return nil, err
		}
		return goja.Null(), nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]inputField, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, inputField{key, values[key]})
	}
	return b.object(fields...)
}
func (b *inputBuilder) value(value any) (goja.Value, error) {
	switch v := value.(type) {
	case nil:
		if err := b.add(4); err != nil {
			return nil, err
		}
		return goja.Null(), nil
	case Source:
		return b.value(string(v))
	case Target:
		return b.value(string(v))
	case string:
		if err := b.add(jsonStringSize(v)); err != nil {
			return nil, err
		}
		if !utf8.ValidString(v) {
			v = string([]rune(v))
		}
		return b.vm.ToValue(v), nil
	case *string:
		if v == nil {
			return b.value(nil)
		}
		return b.value(*v)
	case *int:
		if v == nil {
			return b.value(nil)
		}
		return b.value(*v)
	case bool:
		n := 5
		if v {
			n = 4
		}
		if err := b.add(n); err != nil {
			return nil, err
		}
		return b.vm.ToValue(v), nil
	case int:
		if err := b.add(len(strconv.Itoa(v))); err != nil {
			return nil, err
		}
		return b.vm.ToValue(v), nil
	case int64:
		if err := b.add(len(strconv.FormatInt(v, 10))); err != nil {
			return nil, err
		}
		return b.vm.ToValue(v), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("脚本输入包含非法数值")
		}
		if err := b.add(jsonFloatSize(v)); err != nil {
			return nil, err
		}
		return b.vm.ToValue(v), nil
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return nil, err
		}
		if err = b.add(len(v)); err != nil {
			return nil, err
		}
		return b.vm.ToValue(n), nil
	case map[string]any:
		return inputMap(b, v)
	case []any:
		return inputArray(b, v)
	case []string:
		return inputArray(b, v)
	case []int:
		return inputArray(b, v)
	case []TextSource:
		return inputArray(b, v)
	case []Token:
		return inputArray(b, v)
	case []Candidate:
		return inputArray(b, v)
	case []Span:
		return inputArray(b, v)
	case []Document:
		return inputArray(b, v)
	case []TreeNode:
		return inputArray(b, v)
	case TextSource:
		return b.object(inputField{"id", v.ID}, inputField{"kind", v.Kind}, inputField{"text", v.Text}, inputField{"depth", v.Depth})
	case Token:
		return b.object(inputField{"id", v.ID}, inputField{"sourceId", v.SourceID}, inputField{"parentId", v.ParentID}, inputField{"kind", v.Kind}, inputField{"text", v.Text}, inputField{"start", v.Start}, inputField{"end", v.End})
	case Span:
		return b.object(inputField{"sourceId", v.SourceID}, inputField{"start", v.Start}, inputField{"end", v.End})
	case Candidate:
		return b.object(inputField{"id", v.ID}, inputField{"ruleId", v.RuleID}, inputField{"role", v.Role}, inputField{"value", v.Value}, inputField{"spans", v.Spans}, inputField{"strength", v.Strength}, inputField{"status", v.Status}, inputField{"reason", v.Reason})
	case Document:
		return b.object(inputField{"fileId", v.FileID}, inputField{"path", v.Path}, inputField{"directory", v.Directory}, inputField{"sources", v.Sources}, inputField{"tokens", v.Tokens}, inputField{"candidates", v.Candidates})
	case TreeNode:
		return b.object(inputField{"path", v.Path}, inputField{"parent", v.Parent}, inputField{"fileIds", v.FileIDs})
	default:
		return nil, errors.New("不支持的脚本输入类型")
	}
}

func makeScriptInput(ctx context.Context, vm *goja.Runtime, value any, limit int) (goja.Value, int, error) {
	// Capture before any user code runs; scripts may subsequently replace Object.
	freeze, ok := goja.AssertFunction(vm.Get("Object").ToObject(vm).Get("freeze"))
	if !ok {
		return nil, 0, errors.New("缺少输入冻结函数")
	}
	b := inputBuilder{ctx: ctx, vm: vm, freeze: freeze, limit: limit}
	input, err := b.value(value)
	return input, b.bytes, err
}
