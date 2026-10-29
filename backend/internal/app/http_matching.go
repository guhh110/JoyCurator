package app

import (
	"net/http"
	"strconv"
)

func catalogRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/search", func(w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()
		year, _ := strconv.Atoi(q.Get("year"))
		v, e := a.Matching.Search(r.Context(), q.Get("type"), q.Get("q"), year)
		return respond(w, v, e)
	})
	add("GET /api/seasons", func(w http.ResponseWriter, r *http.Request) error {
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		v, e := a.Matching.Seasons(r.Context(), id)
		return respond(w, v, e)
	})
	add("GET /api/episodes", func(w http.ResponseWriter, r *http.Request) error {
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		season, _ := strconv.Atoi(r.URL.Query().Get("season"))
		v, e := a.Matching.Episodes(r.Context(), id, season)
		return respond(w, v, e)
	})
}
