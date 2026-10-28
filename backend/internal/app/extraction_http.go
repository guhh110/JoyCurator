package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"joycurator/internal/recognition"
)

type extractionRequest struct {
	Path  string              `json:"path"`
	Root  string              `json:"root,omitempty"`
	Rules *[]recognition.Rule `json:"rules,omitempty"`
}

func (a *App) extractionRoutes(add func(string, endpoint)) {
	add("GET /api/extraction/rules", func(w http.ResponseWriter, r *http.Request) error {
		reply(w, map[string]any{"rules": a.currentExtractor().Rules(), "defaults": recognition.DefaultRules(), "catalog": recognition.Catalog(), "targets": recognition.Targets(), "sources": recognition.Sources()})
		return nil
	})
	add("PUT /api/extraction/rules", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Rules []recognition.Rule `json:"rules"`
		}
		if err := read(r, &in); err != nil {
			return err
		}
		if in.Rules == nil {
			return errors.New("请提供规则数组")
		}
		if err := a.saveExtractionRules(in.Rules); err != nil {
			return err
		}
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	add("POST /api/extraction/test", func(w http.ResponseWriter, r *http.Request) error {
		var in extractionRequest
		if err := read(r, &in); err != nil {
			return err
		}
		engine := a.currentExtractor()
		if in.Rules != nil {
			var err error
			engine, err = recognition.Compile(*in.Rules)
			if err != nil {
				return err
			}
		}
		settings, err := a.settings()
		if err != nil {
			return err
		}
		root := extractionRoot(in.Path, in.Root, settings.SourceRoot)
		result, err := engine.Extract(r.Context(), recognition.Input{Path: in.Path, Root: root}, nil)
		if err != nil {
			return err
		}
		reply(w, map[string]any{"path": in.Path, "root": root, "metadata": result.Metadata, "steps": result.Steps})
		return nil
	})
	add("POST /api/extraction/examples", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Rules []recognition.Rule `json:"rules"`
		}
		if err := read(r, &in); err != nil {
			return err
		}
		engine, err := recognition.Compile(in.Rules)
		if err != nil {
			return err
		}
		items := []map[string]any{}
		for _, rule := range engine.Rules() {
			for _, example := range rule.Examples {
				result, err := engine.Extract(r.Context(), recognition.Input{Path: example.Path, Root: example.Root}, nil)
				if err != nil {
					return err
				}
				var actual any
				for _, step := range result.Steps {
					if step.RuleID == rule.ID {
						actual = step.ExtractedValue
						break
					}
				}
				actualRaw, _ := json.Marshal(actual)
				var actualValue, expected any
				_ = json.Unmarshal(actualRaw, &actualValue)
				_ = json.Unmarshal(example.Expected, &expected)
				items = append(items, map[string]any{"ruleId": rule.ID, "path": example.Path, "expected": expected, "actual": actualValue, "passed": recognition.Equal(actualValue, expected)})
			}
		}
		reply(w, items)
		return nil
	})
	add("GET /api/extraction/traces", func(w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		items, total, err := a.extractionHistory(page, q.Get("path"), q.Get("workspaceId"))
		if err == nil {
			reply(w, map[string]any{"items": items, "total": total, "page": page})
		}
		return err
	})
	add("GET /api/extraction/traces/{id}", func(w http.ResponseWriter, r *http.Request) error {
		item, steps, err := a.extractionDetail(r.PathValue("id"))
		if err == nil {
			reply(w, map[string]any{"trace": item, "metadata": item.Metadata, "steps": steps})
		}
		return err
	})
	add("DELETE /api/extraction/traces/{id}", func(w http.ResponseWriter, r *http.Request) error {
		result, err := a.db.Exec("DELETE FROM extraction_traces WHERE id=? AND status<>'running'", r.PathValue("id"))
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return errors.New("轨迹不存在或尚在运行，不能清理")
		}
		reply(w, map[string]bool{"ok": true})
		return nil
	})
}
