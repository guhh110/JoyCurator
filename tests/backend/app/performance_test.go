package app

import (
	"context"
	"encoding/json"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkPreview1000(b *testing.B) {
	a, root := fixture(b)
	files := make([]model.File, 1000)
	for i := range files {
		files[i] = mediaFile(b, a, root, fmt.Sprintf("Show.S01E%03d.mkv", i))
	}
	input := previewRequest(b, a, "tv", 2, "copy", "error", files...)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := a.dependencies.Organizer.BatchPreview(context.Background(), organizer.BatchPreviewRequest{RecognitionID: input.RecognitionID, Revision: input.Revision, Mode: input.Mode, DuplicatePolicy: input.DuplicatePolicy}); err != nil {
			b.Fatal(err)
		}
	}
}

type benchmarkCatalog struct {
	matching.Provider
	calls    atomic.Int64
	episodes []model.Episode
}

func (p *benchmarkCatalog) Detail(context.Context, string, int) (model.Entry, error) {
	p.calls.Add(1)
	return model.Entry{Type: "tv", TMDBID: 2, Title: "Show", OriginalTitle: "Show"}, nil
}
func (p *benchmarkCatalog) Seasons(context.Context, int) ([]model.Season, error) {
	p.calls.Add(1)
	return []model.Season{{Number: 1, Count: 2000}}, nil
}
func (p *benchmarkCatalog) SeasonEpisodes(context.Context, int, int) ([]model.Episode, error) {
	p.calls.Add(1)
	return p.episodes, nil
}

// Use the same benchmark name (and temporary path prefix) for saved-rule comparisons.
func BenchmarkWorkspace2000(b *testing.B) {
	benchmarkWorkspace2000(b, os.Getenv("JOY_BENCH_RULES") == "saved")
}
func BenchmarkWorkspaceSavedRules2000(b *testing.B) {
	benchmarkWorkspace2000(b, true)
}
func benchmarkWorkspace2000(b *testing.B, savedRules bool) {
	a, root := fixture(b)
	if savedRules {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "backend", "recognition", "testdata", "execution-baseline-pipeline.json"))
		if err != nil {
			b.Fatal(err)
		}
		var pipeline recognition.Pipeline
		if err = json.Unmarshal(raw, &pipeline); err != nil {
			b.Fatal(err)
		}
		if err = a.dependencies.Recognition.Save(pipeline); err != nil {
			b.Fatal(err)
		}
	}
	for i := 1; i <= 2000; i++ {
		if err := os.WriteFile(filepath.Join(root, "source", fmt.Sprintf("Show [tmdb-2].S01E%04d.mkv", i)), []byte("video"), 0644); err != nil {
			b.Fatal(err)
		}
	}
	provider := &benchmarkCatalog{episodes: make([]model.Episode, 2000)}
	for i := range provider.episodes {
		provider.episodes[i] = model.Episode{Season: 1, Number: i + 1}
	}
	a.dependencies.Workspace.factory = func(model.Settings, *trace.Recorder) (matching.Provider, error) { return provider, nil }
	db := testDB(b, a)
	var dbPath, dbName string
	var sequence int
	if err := db.QueryRow("PRAGMA database_list").Scan(&sequence, &dbName, &dbPath); err != nil {
		b.Fatal(err)
	}
	footprint := func() int64 {
		var size int64
		for _, p := range []string{dbPath, dbPath + "-wal"} {
			if info, e := os.Stat(p); e == nil {
				size += info.Size()
			}
		}
		return size
	}
	beforeBytes := footprint()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		task, err := a.dependencies.Workspace.Start([]string{filepath.Join(root, "source")}, "", "organize")
		if err != nil {
			b.Fatal(err)
		}
		task = waitRecognitionWithin(b, a, task.ID, 60*time.Second)
		if task.State != "completed" || len(task.Items) != 2000 {
			b.Fatal(task.State, len(task.Items), task.Error)
		}
		if provider.calls.Load() != 0 {
			b.Fatal("recognition called provider")
		}
		for _, item := range task.Items {
			if item.State != "review" || item.Confirmed() || item.Selected {
				b.Fatal(item.Path, item.State, item.Reason)
			}
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(footprint()-beforeBytes)/float64(b.N), "db_growth_bytes/op")
	b.ReportMetric(float64(provider.calls.Load())/float64(b.N), "provider_calls/batch")
}
