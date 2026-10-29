package app

import (
	"net/http"
	"strconv"
	"strings"

	"joycurator/internal/planning"
)

func organizationRoutes(a Dependencies, add func(string, endpoint)) {
	add("POST /api/preview", func(w http.ResponseWriter, r *http.Request) error {
		var in planning.PreviewRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Planning.Preview(r.Context(), in)
		return respond(w, v, e)
	})
	add("POST /api/execute", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID string `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Planning.Submit(in.ID, false)
		return respond(w, v, e)
	})
	add("GET /api/runs", func(w http.ResponseWriter, r *http.Request) error {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		if page > 1000000 {
			page = 1000000
		}
		v, e := a.Organizer.Records(page)
		return respond(w, v, e)
	})
	add("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.Record(r.PathValue("id"))
		return respond(w, v, e)
	})
	for _, action := range []string{"retry", "undo"} {
		add("POST /api/runs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) error {
			v, e := a.Organizer.Resume(r.PathValue("id"), strings.HasSuffix(r.URL.Path, "/undo"))
			return respond(w, v, e)
		})
	}
	add("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		return respond(w, map[string]bool{"ok": true}, a.Organizer.Cancel(r.PathValue("id")))
	})
}
