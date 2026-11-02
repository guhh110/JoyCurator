package app

import (
	"net/http"
	"strconv"

	"joycurator/internal/organizer"
)

func batchRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/queue", func(w http.ResponseWriter, r *http.Request) error { return respond(w, a.Organizer.Queue(), nil) })
	add("GET /api/queue/items", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.QueuePage(r.URL.Query().Get("purpose"), queryPage(r))
		return respond(w, v, e)
	})
	add("POST /api/batches/preview", func(w http.ResponseWriter, r *http.Request) error {
		var in organizer.BatchPreviewRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Organizer.BatchPreview(r.Context(), in)
		return respond(w, v, e)
	})
	add("POST /api/batches/execute", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID string `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Organizer.ExecuteBatch(in.ID)
		return respond(w, v, e)
	})
	add("GET /api/batches", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.Batches(queryPage(r))
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

func queryPage(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		return 1
	}
	if page > 1000000 {
		return 1000000
	}
	return page
}
