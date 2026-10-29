package app

import (
	"errors"
	"net/http"

	"joycurator/internal/matching"
)

func identificationRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Workspace.Latest()
		return respond(w, v, e)
	})
	add("POST /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Paths []string `json:"paths"`
			ID    string   `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		if len(in.Paths) == 0 {
			return errors.New("请选择文件或目录")
		}
		v, e := a.Workspace.Start(in.Paths, in.ID)
		return respond(w, v, e)
	})
	add("GET /api/recognitions/{id}", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Workspace.Read(r.PathValue("id"))
		return respond(w, v, e)
	})
	add("PUT /api/recognitions/{id}", func(w http.ResponseWriter, r *http.Request) error {
		var in matching.Update
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Workspace.Update(r.Context(), r.PathValue("id"), in)
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/retry", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Workspace.Start(nil, r.PathValue("id"))
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		return respond(w, map[string]bool{"ok": true}, a.Workspace.Cancel(r.PathValue("id")))
	})
}
