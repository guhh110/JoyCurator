package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/planning"
)

func TestSeasonAndEpisodeArtIsPlannedAndExistingArtIsPreserved(t *testing.T) {
	a, root := fixture(t)
	s, err := a.dependencies.Config.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Images = true
	if _, err := testDB(t, a).Exec("UPDATE config SET value=?", jsonText(s)); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	for key, value := range map[string]string{
		"/tv/2?append_to_response=credits&language=zh-CN": `{"id":2,"name":"山海旅人","overview":"测试剧情","first_air_date":"2023-01-01","origin_country":["CN"],"poster_path":"/show-poster.jpg","backdrop_path":"/show-backdrop.jpg"}`,
		"/tv/2/season/1?language=zh-CN": `{"id":24,"season_number":1,"name":"第一季","overview":"季简介","air_date":"2023-02-01","poster_path":"/season-poster.jpg","episodes":[{"episode_number":1,"still_path":null},{"episode_number":2,"still_path":"/second-still.jpg"},{"episode_number":3,"still_path":"/third-still.jpg"}]}`,
	} {
		if _, err := testDB(t, a).Exec("INSERT OR REPLACE INTO tmdb_cache VALUES(?,?,?)", key, value, expires); err != nil {
			t.Fatal(err)
		}
	}
	merged := mediaFile(t, a, root, "Show.S01E01-E02.mkv")
	third := mediaFile(t, a, root, "Show.S01E03.mkv")
	request := planning.PreviewRequest{DuplicatePolicy: "error", Type: "tv", TMDBID: 2, Mode: "copy", Files: []model.File{merged, third}}
	preview := func() *planning.Plan {
		t.Helper()
		p, err := a.dependencies.Planning.Preview(context.Background(), request)
		if err != nil || len(p.Issues) != 0 {
			t.Fatalf("preview: %+v, %v", p, err)
		}
		return p
	}
	p := preview()
	images := map[string]string{}
	seasonNFOs := 0
	for _, op := range p.Ops {
		if op.Kind == "image" {
			images[op.Target] = op.URL
		}
		if op.Kind == "nfo" && filepath.Base(op.Target) == "season.nfo" {
			seasonNFOs++
			if !strings.Contains(string(op.Content), "<season>1</season>") || !strings.Contains(string(op.Content), "<title>第一季</title>") || !strings.Contains(string(op.Content), "<plot>季简介</plot>") {
				t.Fatalf("season.nfo content: %s", op.Content)
			}
			if filepath.Dir(op.Target) != filepath.Dir(p.Rows[0].Target) {
				t.Fatalf("season.nfo not beside episodes: %s", op.Target)
			}
		}
	}
	if seasonNFOs != 1 {
		t.Fatalf("expected one shared season.nfo, got %d", seasonNFOs)
	}
	for i, url := range []string{"/second-still.jpg", "/third-still.jpg"} {
		stem := strings.TrimSuffix(p.Rows[i].Target, filepath.Ext(p.Rows[i].Target))
		if images[stem+".jpg"] != url {
			t.Fatalf("episode artwork %q: got %q, want %q", stem+".jpg", images[stem+".jpg"], url)
		}
	}
	seasonDir := filepath.Dir(p.Rows[0].Target)
	showDir := filepath.Dir(seasonDir)
	for name, url := range map[string]string{
		filepath.Join(seasonDir, "season01-poster.jpg"): "/season-poster.jpg",
		filepath.Join(showDir, "poster.jpg"):           "/show-poster.jpg",
		filepath.Join(showDir, "fanart.jpg"):           "/show-backdrop.jpg",
	} {
		if images[name] != url {
			t.Fatalf("artwork %q: got %q, want %q", name, images[name], url)
		}
	}
	if len(images) != 5 {
		t.Fatalf("unexpected image operations: %v", images)
	}
	thumb := strings.TrimSuffix(p.Rows[0].Target, filepath.Ext(p.Rows[0].Target)) + ".jpg"
	for _, target := range []string{thumb, filepath.Join(seasonDir, "season01-poster.jpg"), filepath.Join(seasonDir, "season.nfo")} {
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p = preview()
	for _, target := range []string{thumb, filepath.Join(seasonDir, "season01-poster.jpg"), filepath.Join(seasonDir, "season.nfo")} {
		for _, op := range p.Ops {
			if op.Target == target && !op.Reused {
				t.Fatalf("existing artifact would be overwritten: %s", target)
			}
		}
		if !strings.Contains(strings.Join(p.Rows[0].Artifacts, " "), "保留已有 "+filepath.Base(target)) {
			t.Fatalf("existing artifact not reported: %s: %v", target, p.Rows[0].Artifacts)
		}
	}

	s.Images = false
	if _, err := testDB(t, a).Exec("UPDATE config SET value=?", jsonText(s)); err != nil {
		t.Fatal(err)
	}
	p = preview()
	for _, op := range p.Ops {
		if op.Kind == "image" {
			t.Fatalf("images disabled, unexpected operation: %+v", op)
		}
	}
	if !strings.Contains(strings.Join(p.Rows[0].Artifacts, " "), "保留已有 season.nfo") {
		t.Fatalf("NFO setting enabled, expected preserved season.nfo: %v", p.Rows[0].Artifacts)
	}

	s.Images, s.NFO = true, false
	if _, err := testDB(t, a).Exec("UPDATE config SET value=?", jsonText(s)); err != nil {
		t.Fatal(err)
	}
	p = preview()
	for _, op := range p.Ops {
		if op.Kind == "nfo" {
			t.Fatalf("NFO disabled, unexpected operation: %+v", op)
		}
	}
}
