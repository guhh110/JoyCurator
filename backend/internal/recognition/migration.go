package recognition

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MigrateLegacyRules only translates saved model. There is no builtin
// execution path; new calls always execute the visible, editable script snapshot.
func MigrateLegacyRules(rules []Rule) ([]Rule, bool, error) {
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, false, err
	}
	var migrated []Rule
	if err = json.Unmarshal(raw, &migrated); err != nil {
		return nil, false, err
	}
	changed := false
	for index, rule := range migrated {
		if rule.Kind != "builtin" {
			continue
		}
		template, ok := descriptor(rule.Config.Handler)
		if !ok || template.Target != rule.Target {
			return nil, false, fmt.Errorf("旧规则 %s 的处理器与目标不匹配，无法迁移", rule.Name)
		}
		supported := false
		for _, source := range template.Sources {
			supported = supported || source == rule.Source
		}
		if !supported {
			return nil, false, errors.New("旧规则来源无效")
		}
		if rule.Config.Pattern != "" || rule.Config.Capture != "" || rule.Config.Value != nil || rule.Config.Code != "" {
			return nil, false, errors.New("旧规则配置混用了其他类型的字段")
		}
		config := template.Config
		if rule.Config.Handler == "year" {
			params := struct {
				Min int `json:"min"`
				Max int `json:"max"`
			}{1900, 2099}
			if len(rule.Config.Params) > 0 && string(rule.Config.Params) != "null" {
				decoder := json.NewDecoder(strings.NewReader(string(rule.Config.Params)))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&params); err != nil {
					return nil, false, errors.New("旧年份规则参数无效")
				}
			}
			if params.Min < 1 || params.Max < params.Min || params.Max > 9999 {
				return nil, false, errors.New("旧年份规则参数超出范围")
			}
			config.Params, _ = json.Marshal(params)
		} else if len(rule.Config.Params) > 0 && string(rule.Config.Params) != "null" {
			var params map[string]any
			if err := json.Unmarshal(rule.Config.Params, &params); err != nil || len(params) > 0 {
				return nil, false, errors.New("旧处理器包含未知参数")
			}
		}
		rule.Kind, rule.Config = "script", config
		migrated[index] = rule
		changed = true
	}
	return migrated, changed, nil
}
