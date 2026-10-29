package app

import (
	"errors"
	"net/http"
	"strconv"

	"joycurator/internal/recognition"
)

func extractionRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/extraction/rules", func(w http.ResponseWriter, r *http.Request) error {
		reply(w, map[string]any{"rules": a.Recognition.Current().Rules(), "defaults": recognition.DefaultRules(), "catalog": recognition.Catalog(), "targets": recognition.Targets(), "sources": recognition.Sources(), "sourceScripts": recognition.SourceScripts(), "scriptLanguage": "javascript"})
		return nil
	})
	add("PUT /api/extraction/rules", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Rules []recognition.Rule `json:"rules"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		if in.Rules == nil {
			return errors.New("请提供规则数组")
		}
		return respond(w, map[string]bool{"ok": true}, a.Recognition.Save(in.Rules))
	})
	add("POST /api/extraction/test", func(w http.ResponseWriter, r *http.Request) error {
		var in recognition.TestRequest
		if e := read(r, &in); e != nil {
			return e
		}
		s, e := a.Config.Load()
		if e != nil {
			return e
		}
		v, e := a.Recognition.Test(r.Context(), in, s.SourceRoot)
		return respond(w, v, e)
	})
	add("POST /api/extraction/examples", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Rules []recognition.Rule `json:"rules"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := recognition.Examples(r.Context(), in.Rules)
		return respond(w, v, e)
	})
	add("GET /api/extraction/traces", func(w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		items, total, e := a.Recognition.History(page, q.Get("path"), q.Get("workspaceId"))
		return respond(w, map[string]any{"items": items, "total": total, "page": page}, e)
	})
	add("GET /api/extraction/traces/{id}", func(w http.ResponseWriter, r *http.Request) error {
		v, steps, e := a.Recognition.Detail(r.PathValue("id"))
		return respond(w, map[string]any{"trace": v, "metadata": v.Metadata, "steps": steps}, e)
	})
	add("DELETE /api/extraction/traces/{id}", func(w http.ResponseWriter, r *http.Request) error {
		return respond(w, map[string]bool{"ok": true}, a.Recognition.Delete(r.PathValue("id")))
	})
}
