package app

import (
	"joycurator/internal/organizer"
	"net/http"
	"strconv"
	"strings"
)

func scrapeRoutes(a Dependencies, add func(string, endpoint)) {
	add("POST /api/scrape/batches/preview", func(w http.ResponseWriter, r *http.Request) error {
		var in organizer.ScrapeBatchPreviewRequest
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Organizer.ScrapeBatchPreview(r.Context(), in)
		return respond(w, v, e)
	})
	add("POST /api/scrape/batches/execute", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID string `json:"id"`
		}
		if e := read(r, &in); e != nil {
			return e
		}
		v, e := a.Organizer.ScrapeExecuteBatch(in.ID)
		return respond(w, v, e)
	})
	add("GET /api/scrape/works", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.ScrapeWorks(r.URL.Query().Get("q"), r.URL.Query().Get("cursor"))
		return respond(w, v, e)
	})
	add("GET /api/scrape/works/{kind}/{id}", func(w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			return err
		}
		v, e := a.Organizer.ScrapeWork(r.PathValue("kind"), id)
		return respond(w, v, e)
	})
	add("POST /api/scrape/preview", func(w http.ResponseWriter, r *http.Request) error {
		var input organizer.ScrapePreviewRequest
		if err := read(r, &input); err != nil {
			return err
		}
		v, e := a.Organizer.ScrapePreview(r.Context(), input)
		return respond(w, v, e)
	})
	add("POST /api/scrape/execute", func(w http.ResponseWriter, r *http.Request) error {
		var input struct {
			ID string `json:"id"`
		}
		if err := read(r, &input); err != nil {
			return err
		}
		v, e := a.Organizer.ScrapeExecute(input.ID)
		return respond(w, v, e)
	})
	add("GET /api/scrape/runs", func(w http.ResponseWriter, r *http.Request) error {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		v, e := a.Organizer.ScrapeRecords(page, organizer.RunFilter{Search: r.URL.Query().Get("search"), State: r.URL.Query().Get("state")})
		return respond(w, v, e)
	})
	add("GET /api/scrape/runs/{id}", func(w http.ResponseWriter, r *http.Request) error {
		v, e := a.Organizer.ScrapeRecord(r.PathValue("id"))
		return respond(w, v, e)
	})
	for _, action := range []string{"retry", "undo", "cancel"} {
		add("POST /api/scrape/runs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) error {
			id := r.PathValue("id")
			if strings.HasSuffix(r.URL.Path, "/cancel") {
				return respond(w, map[string]bool{"ok": true}, a.Organizer.ScrapeCancel(id))
			}
			v, e := a.Organizer.ScrapeResume(id, strings.HasSuffix(r.URL.Path, "/undo"))
			return respond(w, v, e)
		})
	}
}
