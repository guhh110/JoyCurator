package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
)

func TestTemplatesAndCategories(t *testing.T) {
	s := model.DefaultSettings()
	s.TargetRoot = "/target"
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
	s.TargetRoot = "/target"
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

func TestNestedCategoryPaths(t *testing.T) {
	settings := model.DefaultSettings()
	settings.TargetRoot = "/target"
	file := model.Source{Path: "/source/file.mkv", Entry: &model.Entry{TMDBID: 1, Type: "tv", Title: "剧名", Year: 2024, Countries: []string{"CN"}}, Season: 1, Episodes: []int{1}}
	target, err := Target(file, settings)
	if err != nil || !strings.HasPrefix(target, "/target/电视剧/国产剧/") {
		t.Fatal(target, err)
	}
	show, err := ShowRoot(file, settings)
	if err != nil || filepath.Dir(target) != filepath.Join(show, "Season 01") {
		t.Fatal(show, target, err)
	}
	for _, name := range []string{"../escape", "/absolute", "电影/../escape", "电影//其他", "电影/", "电影/.", `电影\其他`} {
		settings.Rules.Categories = []model.Category{{Name: name}}
		if Validate(settings.Rules) == nil {
			t.Fatal("unsafe category accepted", name)
		}
		if _, err := Target(file, settings); err == nil {
			t.Fatal("unsafe path rendered", name)
		}
	}
	settings = model.DefaultSettings()
	settings.TargetRoot = "/target"
	settings.Rules.TVFile = "{category}-{title}.{ext}"
	if _, err := Target(file, settings); err == nil {
		t.Fatal("nested category allowed in filename")
	}
}

func TestDefaultCategories(t *testing.T) {
	categories := model.DefaultCategories()
	for _, tc := range []struct {
		name string
		item model.Entry
		want string
	}{
		{"chinese movie", model.Entry{Type: "movie", Year: 2020, Countries: []string{"CN"}}, "电影/华语电影"},
		{"hong kong movie", model.Entry{Type: "movie", Countries: []string{"HK"}}, "电影/华语电影"},
		{"anime movie", model.Entry{Type: "movie", Genres: []string{"动画"}, Countries: []string{"JP"}}, "电影/动画电影"},
		{"anime movie in english metadata", model.Entry{Type: "movie", Genres: []string{"Animation"}}, "电影/动画电影"},
		{"documentary", model.Entry{Type: "movie", Genres: []string{"Documentary"}, Countries: []string{"US"}}, "电影/纪录片"},
		{"american movie", model.Entry{Type: "movie", Countries: []string{"US"}}, "电影/欧美电影"},
		{"korean show", model.Entry{Type: "tv", Countries: []string{"KR"}}, "电视剧/日韩剧"},
		{"anime show", model.Entry{Type: "tv", Genres: []string{"动画"}, Countries: []string{"JP"}}, "电视剧/日番"},
		{"unknown country", model.Entry{Type: "movie", Countries: []string{"IN"}}, "电影/其他电影"},
		{"Chinese animation", model.Entry{Type: "tv", Genres: []string{"动画"}, Countries: []string{"CN"}}, "电视剧/国漫"},
		{"Chinese drama", model.Entry{Type: "tv", Countries: []string{"CN"}}, "电视剧/国产剧"},
		{"Japanese drama", model.Entry{Type: "tv", Countries: []string{"JP"}}, "电视剧/日韩剧"},
		{"American drama", model.Entry{Type: "tv", Countries: []string{"US"}}, "电视剧/欧美剧"},
		{"kids before anime and country", model.Entry{Type: "tv", Genres: []string{"动画", "儿童"}, Countries: []string{"CN"}}, "电视剧/儿童"},
		{"English kids", model.Entry{Type: "tv", Genres: []string{"Animation", "Kids"}, Countries: []string{"JP"}}, "电视剧/儿童"},
		{"family is not automatically kids", model.Entry{Type: "tv", Genres: []string{"Family"}, Countries: []string{"US"}}, "电视剧/欧美剧"},
		{"movie kids unaffected", model.Entry{Type: "movie", Genres: []string{"儿童"}, Countries: []string{"CN"}}, "电影/华语电影"},
		{"unknown country show", model.Entry{Type: "tv", Countries: []string{"IN"}}, "电视剧/其他剧集"},
		{"no country show", model.Entry{Type: "tv"}, "电视剧/其他剧集"},
	} {
		if got := Category(tc.item, categories); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if err := Validate(model.DefaultSettings().Rules); err != nil {
		t.Fatalf("default rules rejected: %v", err)
	}
}
