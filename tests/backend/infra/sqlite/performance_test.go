package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"os"
	"path/filepath"
	"testing"
)

// All modes use precisely the same 2000 paths. PureRecognition and SnapshotWrite
// isolate execution and persistence; LightImport and FullDebug measure their
// respective complete output modes.
func BenchmarkRecognition2000(b *testing.B) {
	benchmarkRecognition2000(b, recognition.DefaultPipeline())
}

// Frozen pre-optimization scripts model saved user rules: executor changes apply
// to them without replacing their source with the new default templates.
func BenchmarkRecognitionSavedRules2000(b *testing.B) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "tests", "backend", "recognition", "testdata", "execution-baseline-pipeline.json"))
	if err != nil {
		b.Fatal(err)
	}
	var pipeline recognition.Pipeline
	if err = json.Unmarshal(raw, &pipeline); err != nil {
		b.Fatal(err)
	}
	benchmarkRecognition2000(b, pipeline)
}

func benchmarkRecognition2000(b *testing.B, pipeline recognition.Pipeline) {
	engine, err := recognition.Compile(pipeline)
	if err != nil {
		b.Fatal(err)
	}
	paths := []string{}
	for i := 0; i < 2000; i++ {
		paths = append(paths, fmt.Sprintf("/source/Show/Season 1/Show.S01E%04d.1080p.mkv", i))
	}
	input := recognition.BatchInput{ContextRoot: "/source/Show", SourceBoundary: "/source", Paths: paths, Targets: paths}
	b.Run("PureRecognition", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, err := engine.Snapshot(context.Background(), input)
			if err != nil || len(result.Files) != len(paths) {
				b.Fatal("recognition failed", err)
			}
		}
	})
	b.Run("SnapshotWrite", func(b *testing.B) {
		store := openStore(b)
		result, err := engine.Snapshot(context.Background(), input)
		if err != nil {
			b.Fatal(err)
		}
		var dbPath, dbName string
		var dbSequence int
		if err = store.db.QueryRow("PRAGMA database_list").Scan(&dbSequence, &dbName, &dbPath); err != nil {
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
		var beforeRows, afterRows int64
		if err = store.db.QueryRow("SELECT total_changes()").Scan(&beforeRows); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			trace := recognition.Trace{ID: fmt.Sprint(n), Status: "running"}
			if err := store.StartTrace(trace); err != nil {
				b.Fatal(err)
			}
			trace.Status = "completed"
			if err := store.FinishTrace(trace, result); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if err = store.db.QueryRow("SELECT total_changes()").Scan(&afterRows); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(afterRows-beforeRows)/float64(b.N), "sqlite_rows/op")
		b.ReportMetric(float64(footprint()-beforeBytes)/float64(b.N), "db_growth_bytes/op")
	})
	b.Run("LightImport", func(b *testing.B) {
		store := openStore(b)
		service, err := recognition.New(store, func() bool { return false })
		if err != nil {
			b.Fatal(err)
		}
		if err = service.Save(pipeline); err != nil {
			b.Fatal(err)
		}
		var path, name string
		var sequence int
		if err = store.db.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
			b.Fatal(err)
		}
		footprint := func() int64 {
			var size int64
			for _, p := range []string{path, path + "-wal"} {
				if info, e := os.Stat(p); e == nil {
					size += info.Size()
				}
			}
			return size
		}
		var before, after int64
		if err = store.db.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
			b.Fatal(err)
		}
		bytesBefore := footprint()
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, id, err := service.ExtractTree(context.Background(), input, "benchmark")
			if err != nil || len(result.Files) != 2000 {
				b.Fatal(len(result.Files), err)
			}
			b.StopTimer()
			_, restored, err := store.TraceDetail(id)
			if err != nil || len(restored.Files) != 2000 || len(restored.Documents) != 2000 {
				b.Fatal("snapshot lost", err)
			}
			b.StartTimer()
		}
		b.StopTimer()
		if err = store.db.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(after-before)/float64(b.N), "sqlite_rows/op")
		b.ReportMetric(float64(footprint()-bytesBefore)/float64(b.N), "db_growth_bytes/op")
	})
	b.Run("FullDebug", func(b *testing.B) {
		writer := &benchmarkWriter{}
		b.ReportAllocs()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, err := engine.ExtractBatch(context.Background(), input, nil)
			if err != nil || len(result.Files) != 2000 || len(result.Steps) == 0 {
				b.Fatal("debug failed", err)
			}
			if err = json.NewEncoder(writer).Encode(result); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(writer.bytes)/float64(b.N), "response_bytes/op")
		b.ReportMetric(0, "sqlite_rows/op")
	})
}

type benchmarkWriter struct{ bytes int64 }

func (w *benchmarkWriter) Write(p []byte) (int, error) { w.bytes += int64(len(p)); return len(p), nil }

func BenchmarkRunHistory(b *testing.B) {
	store := openStore(b)
	ops := make([]organizer.Work, 100)
	for i := range ops {
		ops[i] = organizer.Work{Operation: organizer.Operation{Kind: "video", Source: fmt.Sprintf("/source/%d.mkv", i), Target: fmt.Sprintf("/target/%d.mkv", i), Status: "completed"}}
	}
	runs := make([]organizer.Run, 1000)
	for i := range runs {
		runs[i] = organizer.Run{ID: fmt.Sprint(i), Purpose: "organize", State: "completed", BatchID: fmt.Sprint(i / 10), Ops: ops}
	}
	for _, run := range runs {
		if err := store.SaveRun(run); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := store.RunsByPurpose(1, "organize", organizer.RunFilter{}); err != nil {
			b.Fatal(err)
		}
		if _, err := store.BatchSummaries("99"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFileExecutionProjection1000(b *testing.B) {
	store := openStore(b)
	run := organizer.Run{ID: "active", Purpose: "organize", State: "running", RecognitionID: "workspace"}
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/target/%d.mkv", i)
		run.Ops = append(run.Ops, organizer.Work{Operation: organizer.Operation{Kind: "video", Target: path, Status: "completed"}})
		run.Mappings = append(run.Mappings, model.MediaMapping{Entry: &model.Entry{Type: "movie", TMDBID: 1, Title: "fixture"}, Path: path, Source: fmt.Sprintf("/source/%d.mkv", i), RunID: run.ID, Identity: fmt.Sprint(i)})
	}
	if err := store.SaveRun(run); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		states, err := store.FileExecutions("workspace")
		if err != nil || len(states) != 1000 || !states["/source/999.mkv"].Completed {
			b.Fatal(len(states), err)
		}
	}
}

func BenchmarkRunCheckpoint(b *testing.B) {
	for _, size := range []int{10, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := openStore(b)
			r := organizer.Run{ID: "checkpoint", Purpose: "organize", State: "running"}
			for i := 0; i < size; i++ {
				r.Ops = append(r.Ops, organizer.Work{Operation: organizer.Operation{Target: fmt.Sprintf("/target/%d", i), Status: "pending"}})
			}
			if err := s.SaveRun(r); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				r.Ops[0].Status = "published"
				if err := s.SaveCheckpoint(r, []int{0}, nil, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWorkspaceCheckpoint(b *testing.B) {
	for _, size := range []int{10, 2000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := openStore(b)
			w := matching.Workspace{Purpose: "organize", ID: "checkpoint", State: "running"}
			for i := 0; i < size; i++ {
				w.Items = append(w.Items, matching.Item{File: model.File{Path: fmt.Sprintf("/source/%d", i)}, State: "pending"})
			}
			if err := s.SaveWorkspace(&w); err != nil {
				b.Fatal(err)
			}
			positions := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				if err := s.CheckpointWorkspace(&w, positions); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
