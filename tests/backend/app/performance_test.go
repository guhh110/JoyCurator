package app

import (
	"context"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/trace"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
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
func BenchmarkWorkspace2000(b *testing.B) {
	a, root := fixture(b)
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
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		task, err := a.dependencies.Workspace.Start([]string{filepath.Join(root, "source")}, "")
		if err != nil {
			b.Fatal(err)
		}
		task = waitRecognition(b, a, task.ID)
		if task.State != "completed" || len(task.Items) != 2000 {
			b.Fatal(task.State, len(task.Items), task.Error)
		}
		for _, item := range task.Items {
			if item.State != "ready" {
				b.Fatal(item.Path, item.State, item.Reason)
			}
		}
	}
	b.ReportMetric(float64(provider.calls.Load())/float64(b.N), "provider_calls/batch")
}
