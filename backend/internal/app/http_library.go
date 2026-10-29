package app

import (
	"net/http"
)

func libraryRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/browse", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Files.Browse(r.URL.Query().Get("path"), r.URL.Query().Get("scope"))
		return respond(w, v, e)
	})
	add("POST /api/scan", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Paths []string `json:"paths"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Files.Scan(r.Context(), in.Paths)
		return respond(w, v, e)
	})
}
