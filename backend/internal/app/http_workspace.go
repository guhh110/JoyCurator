package app

import (
	"errors"
	"net/http"

	"joycurator/internal/matching"
)

func identificationRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Workspace.Latest(r.URL.Query().Get("purpose"))
		return respond(w, v, e)
	})
	add("POST /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Purpose string   `json:"purpose"`
			Paths   []string `json:"paths"`
			ID      string   `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		if len(in.Paths) == 0 {
			return errors.New("请选择文件或目录")
		}
		v, e := a.Workspace.Start(in.Paths, in.ID, in.Purpose)
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
	add("POST /api/recognitions/{id}/candidates", func(w http.ResponseWriter, r *http.Request) error {
		var in CandidateRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Workspace.Candidates(r.PathValue("id"), in)
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/confirm", func(w http.ResponseWriter, r *http.Request) error {
		var in matching.ConfirmRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Workspace.Confirm(r.Context(), r.PathValue("id"), in)
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/sources/remove", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Path     string `json:"path"`
			Revision int    `json:"revision"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		if in.Path == "" {
			return errors.New("请指定要删除的来源")
		}
		v, e := a.Workspace.RemoveSource(r.PathValue("id"), in.Path, in.Revision)
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/reidentify", func(w http.ResponseWriter, r *http.Request) error {
		var input struct {
			Revision int      `json:"revision"`
			Paths    []string `json:"paths"`
		}
		if err := read(r, &input); err != nil {
			return err
		}
		v, err := a.Workspace.Reidentify(r.PathValue("id"), input.Revision, input.Paths)
		return respond(w, v, err)
	})
	add("POST /api/recognitions/{id}/retry", func(w http.ResponseWriter, r *http.Request) error {
		task, e := a.Workspace.Read(r.PathValue("id"))
		if e != nil {
			return e
		}
		v, e := a.Workspace.Start(nil, task.ID, task.Purpose)
		return respond(w, v, e)
	})
	add("POST /api/recognitions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		return respond(w, map[string]bool{"ok": true}, a.Workspace.Cancel(r.PathValue("id")))
	})
}
