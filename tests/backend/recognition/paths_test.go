package recognition_test

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"joycurator/internal/recognition"
)

func TestChineseEpisodeExample(t *testing.T) {
	p := extractMetadata(t, "/fan人修仙/第一季/第一集.mp4", "/")
	if value(p.Title, "") != "fan人修仙" || value(p.Season, -1) != 1 || !reflect.DeepEqual(p.Episodes, []int{1}) || value(p.MediaType, "") != "tv" {
		t.Fatalf("got %+v", p)
	}
}
func TestRecognitionNeverUsesSourceName(t *testing.T) {
	p := extractMetadata(t, "/library/Show [tmdb-999]/source/Season 2/01.mkv", "/library/Show [tmdb-999]/source")
	if value(p.Title, "") != "01" || p.TMDBID != nil || value(p.Season, -1) != 2 || len(p.Episodes) != 0 {
		t.Fatal(p)
	}
	if recognition.NormalizeTitle("SHOW.Name：星空") != recognition.NormalizeTitle("show name 星空") {
		t.Fatal("punctuation normalization")
	}
}

func TestPathSourcesKeepRootBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, path, root, relative, absolute string
		folders                              []string
	}{
		{"POSIX example", "/outside [tmdb-999]/source/Show [tmdb-7]/Season 2/03.mkv", "/outside [tmdb-999]/source", "Show [tmdb-7]/Season 2/03.mkv", "/outside [tmdb-999]/source/Show [tmdb-7]/Season 2/03.mkv", []string{"Season 2", "Show [tmdb-7]"}},
		{"POSIX normalized", "/source/Show/../Film.mkv", "/source", "Film.mkv", "/source/Film.mkv", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertPathSources(t, tc.path, tc.root, tc.relative, tc.absolute, tc.folders)
		})
	}
}

func TestWindowsPathsAndPOSIXExamples(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-native absolute path behavior")
	}
	for _, tc := range []struct {
		name, path, root, relative, absolute string
		folders                              []string
	}{
		{"drive", `C:\outside [tmdb-999]\source\Show [tmdb-7]\Season 2\03.mkv`, `C:\outside [tmdb-999]\source`, "Show [tmdb-7]/Season 2/03.mkv", `C:\outside [tmdb-999]\source\Show [tmdb-7]\Season 2\03.mkv`, []string{"Season 2", "Show [tmdb-7]"}},
		{"drive default root", `C:\Show\Season 2\03.mkv`, "", "Show/Season 2/03.mkv", `C:\Show\Season 2\03.mkv`, []string{"Season 2", "Show"}},
		{"UNC", `\\server\share\source\Show\03.mkv`, `\\server\share\source`, "Show/03.mkv", `\\server\share\source\Show\03.mkv`, []string{"Show"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertPathSources(t, tc.path, tc.root, tc.relative, tc.absolute, tc.folders)
		})
	}
	p := extractMetadata(t, `C:\outside [tmdb-999]\source\Show [tmdb-7]\Season 2\03.mkv`, `C:\outside [tmdb-999]\source`)
	if value(p.TMDBID, -1) != 7 || value(p.Title, "") != "03" || value(p.Season, -1) != 2 || len(p.Episodes) != 0 {
		t.Fatalf("Windows native extraction: %+v", p)
	}
}

func assertPathSources(t *testing.T, file, root, relative, absolute string, folders []string) {
	t.Helper()
	result, err := recognition.DefaultEngine().ExtractBatch(context.Background(), recognition.BatchInput{ContextRoot: filepath.Dir(absolute), SourceBoundary: root, Paths: []string{file}, Targets: []string{file}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := result.Documents[0]
	got := []string{}
	for _, source := range doc.Sources[1:] {
		got = append(got, source.Text)
	}
	if doc.Path != absolute || !reflect.DeepEqual(got, folders) {
		t.Fatal(doc.Path, got, absolute, folders)
	}

}

func TestPathBoundaryRejectsTraversalAndDifferentKinds(t *testing.T) {
	invalid := []recognition.Input{
		{Path: "/source/../outside.mkv", Root: "/source"},
		{Path: "/source/file.mkv", Root: "/source/file.mkv"},
		{Path: "/source2/file.mkv", Root: "/source"},
		{Path: "/source/file.mkv", Root: "source"},
	}
	if runtime.GOOS == "windows" {
		invalid = append(invalid,
			recognition.Input{Path: `C:\source\..\outside.mkv`, Root: `C:\source`},
			recognition.Input{Path: `C:\source2\file.mkv`, Root: `C:\source`},
			recognition.Input{Path: `C:\source\file.mkv`, Root: `D:\source`},
			recognition.Input{Path: `C:\source\file.mkv`, Root: "/source"},
			recognition.Input{Path: "/source/file.mkv", Root: `C:\source`},
			recognition.Input{Path: `C:source\file.mkv`, Root: `C:\source`},
			recognition.Input{Path: `\source\file.mkv`, Root: `C:\source`},
		)
	}
	for _, in := range invalid {
		if _, err := recognition.DefaultEngine().Extract(context.Background(), in, nil); err == nil {
			t.Fatalf("accepted out-of-bound path %+v", in)
		}
	}
}

func value[T any](v *T, zero T) T {
	if v == nil {
		return zero
	}
	return *v
}
func extractMetadata(t *testing.T, path, root string) recognition.Metadata {
	t.Helper()
	result, err := recognition.DefaultEngine().Extract(context.Background(), recognition.Input{Path: path, Root: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Metadata
}
