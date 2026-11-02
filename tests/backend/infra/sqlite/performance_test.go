package sqlite

import (
	"context"
	"fmt"
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
	for n := 0; n < b.N; n++ {
		for i := 0; i < 2000; i++ {
			_, _, err := service.ExtractFile(context.Background(), fmt.Sprintf("/source/Show.S01E%03d.1080p.mkv", i), "/source", "benchmark")
			if err != nil {
				b.Fatal(err)
			}
		}
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
	if err := store.SaveBatch(runs); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := store.RunsByPurpose(1, "organize"); err != nil {
			b.Fatal(err)
		}
		if _, err := store.BatchSummaries("99"); err != nil {
			b.Fatal(err)
		}
	}
}
