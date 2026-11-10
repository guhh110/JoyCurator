package sqlite

import (
	"context"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"testing"
)

func BenchmarkRecognition2000(b *testing.B) {
	store := openStore(b)
	service, err := recognition.New(store, func() bool { return false })
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	paths := []string{}
	for i := 0; i < 2000; i++ {
		paths = append(paths, fmt.Sprintf("/source/Show/Season 1/Show.S01E%04d.1080p.mkv", i))
	}
	for n := 0; n < b.N; n++ {
		result, id, err := service.ExtractTree(context.Background(), recognition.BatchInput{ContextRoot: "/source/Show", SourceBoundary: "/source", Paths: paths, Targets: paths}, "benchmark")
		if err != nil || len(result.Files) != 2000 {
			b.Fatal(len(result.Files), err)
		}
		b.StopTimer()
		_, steps, err := store.TraceDetail(id)
		if err != nil {
			b.Fatal(err)
		}
		restored, err := recognition.RestoreBatch(steps)
		if err != nil {
			b.Fatal(err)
		}
		if len(restored.Files) != 2000 || len(restored.Documents) != 2000 {
			b.Fatal("batch snapshot lost")
		}
		var snapshots int
		for _, s := range steps {
			if s.RuleConfig.ID != "" {
				snapshots++
			}
		}
		if snapshots != len(service.Current().Rules().Rules()) {
			b.Fatal("duplicate rule snapshots", snapshots)
		}
		b.StartTimer()
	}

}

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
		run.Mappings = append(run.Mappings, model.MediaMapping{Path: path, Source: fmt.Sprintf("/source/%d.mkv", i), RunID: run.ID, Identity: fmt.Sprint(i)})
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
			w := matching.Workspace{ID: "checkpoint", State: "running"}
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
