package recognition

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// The bounded JS reader produces only this JSON data algebra. Count its exact
// encoding length without allocating an encoded copy of every rule result.
func jsonDataSize(value any) (int, error) {
	switch v := value.(type) {
	case nil:
		return 4, nil
	case bool:
		if v {
			return 4, nil
		}
		return 5, nil
	case string:
		return jsonStringSize(v), nil
	case json.Number:
		return len(v), nil
	case []any:
		size := 2 + max(0, len(v)-1)
		for _, x := range v {
			n, err := jsonDataSize(x)
			if err != nil {
				return 0, err
			}
			size += n
		}
		return size, nil
	case map[string]any:
		size := 2 + max(0, len(v)-1)
		for k, x := range v {
			n, err := jsonDataSize(x)
			if err != nil {
				return 0, err
			}
			size += jsonStringSize(k) + 1 + n
		}
		return size, nil
	default:
		return 0, errors.New("脚本只能返回 JSON 数据")
	}
}
func resultObject(value any, allowed ...string) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	v, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("脚本结果需要对象")
	}
	unknown := ""
	hasUnknown := false
	for key := range v {
		if !contains(allowed, key) {
			if !hasUnknown || key < unknown {
				unknown = key
			}
			hasUnknown = true
		}
	}
	if hasUnknown {
		return nil, fmt.Errorf("未知脚本结果字段 %q", unknown)
	}
	return v, nil
}
func resultString(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	v, ok := value.(string)
	if !ok {
		return "", errors.New("脚本字段需要字符串")
	}
	return v, nil
}
func resultInt(value any) (int, error) {
	if value == nil {
		return 0, nil
	}
	v, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("脚本字段需要整数")
	}
	n, err := v.Int64()
	if err != nil || int64(int(n)) != n {
		return 0, errors.New("脚本字段需要整数")
	}
	return int(n), nil
}
func resultArray[T any](value any, read func(any) (T, error)) ([]T, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("脚本结果需要数组")
	}
	out := make([]T, len(list))
	for i, v := range list {
		var err error
		out[i], err = read(v)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
func resultSpan(value any) (Span, error) {
	var out Span
	v, err := resultObject(value, "sourceId", "start", "end")
	if err != nil {
		return out, err
	}
	if out.SourceID, err = resultString(v["sourceId"]); err != nil {
		return out, err
	}
	if out.Start, err = resultInt(v["start"]); err != nil {
		return out, err
	}
	out.End, err = resultInt(v["end"])
	return out, err
}
func resultToken(value any) (Token, error) {
	var out Token
	v, err := resultObject(value, "id", "sourceId", "parentId", "kind", "text", "start", "end")
	if err != nil {
		return out, err
	}
	for _, field := range []struct {
		name string
		dest *string
	}{{"id", &out.ID}, {"sourceId", &out.SourceID}, {"kind", &out.Kind}, {"text", &out.Text}} {
		if *field.dest, err = resultString(v[field.name]); err != nil {
			return out, err
		}
	}
	if v["parentId"] != nil {
		p, e := resultString(v["parentId"])
		if e != nil {
			return out, e
		}
		out.ParentID = &p
	}
	if out.Start, err = resultInt(v["start"]); err != nil {
		return out, err
	}
	out.End, err = resultInt(v["end"])
	return out, err
}
func resultProposal(value any) (proposal, error) {
	var out proposal
	v, err := resultObject(value, "value", "spans", "strength", "status", "reason")
	if err != nil {
		return out, err
	}
	out.Value = v["value"]
	if out.Spans, err = resultArray(v["spans"], resultSpan); err != nil {
		return out, err
	}
	for _, field := range []struct {
		name string
		dest *string
	}{{"strength", &out.Strength}, {"status", &out.Status}, {"reason", &out.Reason}} {
		if *field.dest, err = resultString(v[field.name]); err != nil {
			return out, err
		}
	}
	return out, nil
}
func resultMetadata(value any) (Metadata, error) {
	var out Metadata
	v, err := resultObject(value, "tmdbId", "title", "mediaType", "year", "season", "episodes")
	if err != nil {
		return out, fmt.Errorf("裁决元数据必须提供完整六个字段：%w", err)
	}
	if len(v) != 6 {
		return out, errors.New("裁决元数据必须提供完整六个字段")
	}
	for _, field := range []struct {
		name string
		dest **int
	}{{"tmdbId", &out.TMDBID}, {"year", &out.Year}, {"season", &out.Season}} {
		if v[field.name] != nil {
			n, e := resultInt(v[field.name])
			if e != nil {
				return out, e
			}
			*field.dest = &n
		}
	}
	for _, field := range []struct {
		name string
		dest **string
	}{{"title", &out.Title}, {"mediaType", &out.MediaType}} {
		if v[field.name] != nil {
			s, e := resultString(v[field.name])
			if e != nil {
				return out, e
			}
			*field.dest = &s
		}
	}
	out.Episodes, err = resultArray(v["episodes"], resultInt)
	return out, err
}
func resultReference(value any) (Reference, error) {
	var out Reference
	v, err := resultObject(value, "fileId", "candidateId")
	if err != nil {
		return out, err
	}
	if out.FileID, err = resultString(v["fileId"]); err != nil {
		return out, err
	}
	out.CandidateID, err = resultString(v["candidateId"])
	return out, err
}
func resultFile(value any) (FileResult, error) {
	var out FileResult
	v, err := resultObject(value, "fileId", "path", "metadata", "decisions")
	if err != nil {
		return out, err
	}
	if out.FileID, err = resultString(v["fileId"]); err != nil {
		return out, err
	}
	if out.Path, err = resultString(v["path"]); err != nil {
		return out, err
	}
	if out.Metadata, err = resultMetadata(v["metadata"]); err != nil {
		return out, err
	}
	if v["decisions"] == nil {
		return out, nil
	}
	decisions, ok := v["decisions"].(map[string]any)
	if !ok {
		return out, errors.New("decisions 必须为对象")
	}
	out.Decisions = make(map[Target]Decision, len(decisions))
	keys := make([]string, 0, len(decisions))
	for key := range decisions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := decisions[key]
		fields, e := resultObject(value, "candidates", "reason")
		if e != nil {
			return out, e
		}
		var d Decision
		if d.Reason, err = resultString(fields["reason"]); err != nil {
			return out, err
		}
		if d.Candidates, err = resultArray(fields["candidates"], resultReference); err != nil {
			return out, err
		}
		out.Decisions[Target(key)] = d
	}
	return out, nil
}
