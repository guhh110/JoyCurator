package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
)

func TestTemplatesAndCategories(t *testing.T) {
	s := model.DefaultSettings()
	s.MovieRoot = "/media/movies"
	s.Rules.Categories = []model.Category{{Name: "科幻", Genres: []string{"科幻"}, YearMin: 2000}, {Name: "其他"}}
	f := model.Source{Path: "/source/film.mkv", Entry: &model.Entry{TMDBID: 1, Type: "movie", Title: "标题:片名", Year: 2020, Genres: []string{"科幻"}}}
	path, err := Target(f, s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "科幻/标题_片名 (2020)") || strings.HasSuffix(path, " - .mkv") {
		t.Fatal(path)
	}
	s.Rules.MovieFile = "{title} - {codec}.{ext}"
	if _, err = Target(f, s); err == nil {
		t.Fatal("custom missing field accepted")
	}
	s.Rules.MovieFile = "../../{title}.{ext}"
	if Validate(s.Rules) == nil {
		t.Fatal("traversal accepted")
	}
	if _, err = Render("{unknown}", map[string]string{}, false); err == nil {
		t.Fatal("unknown token accepted")
	}
	s = model.DefaultSettings()
	s.TVRoot = "/media/tv"
	f.Entry = &model.Entry{TMDBID: 2, Title: "Show", Type: "tv", Year: 2020}
	f.Season = 0
	f.Episodes = []int{1, 2}
	path, err = Target(f, s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, filepath.Join("Season 00", "Show - S00E01E02.mkv")) {
		t.Fatal(path)
	}
	if _, err = Target(model.Source{Path: "/source/x.mkv"}, s); err == nil {
		t.Fatal("unidentified file accepted")
	}
}

func TestDefaultCategories(t *testing.T) {
	categories := model.DefaultCategories()
	for _, tc := range []struct {
		name string
		item model.Entry
		want string
	}{
		{"chinese movie", model.Entry{Type: "movie", Year: 2020, Countries: []string{"CN"}}, "华语电影"},
		{"hong kong movie", model.Entry{Type: "movie", Countries: []string{"HK"}}, "华语电影"},
		{"anime movie", model.Entry{Type: "movie", Genres: []string{"动画"}, Countries: []string{"JP"}}, "动画电影"},
		{"anime movie in english metadata", model.Entry{Type: "movie", Genres: []string{"Animation"}}, "动画电影"},
		{"documentary", model.Entry{Type: "movie", Genres: []string{"Documentary"}, Countries: []string{"US"}}, "纪录片"},
		{"american movie", model.Entry{Type: "movie", Countries: []string{"US"}}, "欧美电影"},
		{"korean show", model.Entry{Type: "tv", Countries: []string{"KR"}}, "日韩剧"},
		{"anime show", model.Entry{Type: "tv", Genres: []string{"动画"}, Countries: []string{"JP"}}, "动漫"},
		{"unknown country", model.Entry{Type: "movie", Countries: []string{"IN"}}, "其他电影"},
		{"no country show", model.Entry{Type: "tv"}, "其他剧集"},
	} {
		if got := Category(tc.item, categories); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if err := Validate(model.DefaultSettings().Rules); err != nil {
		t.Fatalf("default rules rejected: %v", err)
	}
}
