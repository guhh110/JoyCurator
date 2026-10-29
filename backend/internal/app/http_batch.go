package app

import (
	"net/http"

	"joycurator/internal/planning"
)

func batchRoutes(a Dependencies, add func(string, endpoint)) {
	add("POST /api/batches/preview", func(w http.ResponseWriter, r *http.Request) error {
		var in planning.BatchPreviewRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Planning.BatchPreview(r.Context(), in)
		return respond(w, v, e)
	})
	add("POST /api/batches/execute", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID string `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Planning.Submit(in.ID, true)
		return respond(w, v, e)
	})
	add("GET /api/batches", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.LatestBatch()
		return respond(w, v, e)
	})
	add("GET /api/batches/{id}", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.ReadBatch(r.PathValue("id"))
		return respond(w, v, e)
	})
	add("POST /api/batches/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		return respond(w, map[string]bool{"ok": true}, a.Organizer.CancelBatch(r.PathValue("id")))
	})
}
