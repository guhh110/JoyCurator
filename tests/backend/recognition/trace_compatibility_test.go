package recognition_test

import (
	"joycurator/internal/trace"
	"reflect"
	"testing"
)

func TestRecognitionTracePreservesParserResults(t *testing.T) {
	for _, path := range []string{
		"/source/流浪地球.2019.2160p.mkv",
		"/source/[Group] 1917.2019.1080p.mkv",
		"/source/Show.S02E03-E05.mkv",
		"/source/Show.S01E01E03.mkv",
		"/source/Show.1x03.mkv",
		"/source/Show [tmdb-2]/第二季/07.mkv",
		"/source/Show/Specials/01.mkv",
		"/source/某剧/第03集.mp4",
		"/source/Show EP03.mkv",
		"/source/Show.S02.1080p.mkv",
		"/source/4k/01.mkv",
	} {
		t.Run(path, func(t *testing.T) {
			log := trace.New()
			got := RecognitionPathTrace(path, "/source", log)
			want := RecognitionPath(path, "/source")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("tracing changed parser: %+v vs %+v", got, want)
			}
			events := log.Events()
			if len(events) < 2 || events[0].Rule != "id-filename" || events[len(events)-1].Rule != "parsed-result" {
				t.Fatal("missing compatibility events")
			}
		})
	}
}
func TestRecognitionTraceExplainsPrecedenceAndBoundary(t *testing.T) {
	log := trace.New()
	p := RecognitionPathTrace("/parent [tmdb-999]/source/Show/Season 2/Show.S01E03 [tmdb-7].mkv", "/parent [tmdb-999]/source", log)
	if p.Season != 1 || p.TMDBID != 7 {
		t.Fatal(p)
	}
	for _, event := range log.Events() {
		if event.Rule == "id-directory" && string(event.Data) == "999" {
			t.Fatal("boundary ignored")
		}
	}
	rules := map[string]bool{}
	for _, event := range log.Events() {
		rules[event.Rule] = true
	}
	if !rules["id-filename"] || !rules["season-folder"] || !rules["season-sxe"] || !rules["title-filename"] {
		t.Fatal(rules)
	}
}
