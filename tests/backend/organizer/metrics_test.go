package organizer

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

func requireMeasured(t *testing.T, op *Operation, written int64) {
	t.Helper()
	if op.DurationMs == nil || *op.DurationMs <= 0 {
		t.Fatal("missing execution duration", op.DurationMs)
	}
	if written == 0 {
		if op.SpeedBps != nil {
			t.Fatal("operation without successful content transfer has a speed", *op.SpeedBps)
		}
		return
	}
	if op.SpeedBps == nil || *op.SpeedBps <= 0 {
		t.Fatal("missing transfer speed", op.SpeedBps)
	}
	gotBytes := *op.SpeedBps * *op.DurationMs / 1000
	if math.Abs(gotBytes-float64(written)) > 0.000001 {
		t.Fatal("speed does not use actual bytes and whole-item duration", gotBytes, written)
	}
}

func TestOperationMetricsModesAndUndo(t *testing.T) {
	for _, mode := range []string{"copy", "cross-device-move", "move", "hardlink", "subtitle", "nfo", "image", "empty"} {
		t.Run(mode, func(t *testing.T) {
			fileMode := mode
			if mode != "move" && mode != "hardlink" {
				fileMode = "copy"
			}
			e, op, body := fixture(t, fileMode)
			written := int64(len(body))
			switch mode {
			case "cross-device-move":
				op.Mode = "move"
				e.FS.DeviceOf = func(os.FileInfo) uint64 { return ^uint64(0) }
			case "move", "hardlink":
				written = 0
			case "subtitle":
				op.Kind = "subtitle"
			case "nfo":
				op.Kind, op.Source, op.Content = "nfo", "", body
				op.Size = 0 // Generated content has no preview source size.
			case "image":
				op.Kind, op.Source, op.Size = "image", "", 0
				e.Image = func(context.Context, string) ([]byte, error) { return body, nil }
			case "empty":
				if err := os.Truncate(op.Source, 0); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(op.Source)
				if err != nil {
					t.Fatal(err)
				}
				op.Size, op.MTime, written = 0, info.ModTime().UnixNano(), 0
			}
			var checkpointTime time.Duration
			save := func() error {
				if op.DurationMs != nil || op.SpeedBps != nil {
					t.Fatal("unfinished operation published final metrics")
				}
				started := time.Now()
				time.Sleep(time.Millisecond)
				checkpointTime += time.Since(started)
				return nil
			}
			if err := e.Run(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
			requireMeasured(t, op, written)
			if *op.DurationMs < float64(checkpointTime)/float64(time.Millisecond) {
				t.Fatal("duration omitted checkpoint time", *op.DurationMs, checkpointTime)
			}
			duration, speed := op.DurationMs, op.SpeedBps
			if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if op.DurationMs != duration || op.SpeedBps != speed {
				t.Fatal("retry replaced a completed item's metrics")
			}
			if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if op.DurationMs != duration || op.SpeedBps != speed {
				t.Fatal("undo replaced forward metrics")
			}
		})
	}
}

func TestOperationMetricsFailureCancelAndRetry(t *testing.T) {
	for _, failure := range []string{"source-changed", "cancelled", "checkpoint"} {
		t.Run(failure, func(t *testing.T) {
			e, op, body := fixture(t, "copy")
			ctx := context.Background()
			save := func() error { return nil }
			switch failure {
			case "source-changed":
				op.Size++
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "checkpoint":
				save = func() error { return errors.New("checkpoint failed") }
			}
			if err := e.Run(ctx, op, save); err == nil {
				t.Fatal("expected failure")
			}
			requireMeasured(t, op, 0)
			previous := op.DurationMs
			// Simulate a retryable item; the checkpoint failure occurred before I/O.
			op.Size, op.Status, op.Temp = int64(len(body)), "pending", ""
			if err := e.Run(context.Background(), op, func() error {
				if op.DurationMs != nil || op.SpeedBps != nil {
					t.Fatal("retry retained the previous attempt's metrics while running")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			requireMeasured(t, op, int64(len(body)))
			if op.DurationMs == previous {
				t.Fatal("retry did not replace failed attempt metrics")
			}
		})
	}
}

func TestOperationMetricsExcludeFailedPublication(t *testing.T) {
	e, op, _ := fixture(t, "copy")
	failure := errors.New("publication checkpoint failed")
	err := e.Run(context.Background(), op, func() error {
		if op.Status == "published" {
			return failure
		}
		return nil
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	requireMeasured(t, op, 0)
}

func TestOperationMetricsPersistAtExistingWorkerCheckpoint(t *testing.T) {
	s, store, plans := executionFixture(t, nil)
	plan := plans.cache.get("batch")
	r := Run{ID: "metrics", Purpose: "organize", State: "running", Ops: plan.Ops}
	missingSource := r.Ops[1].Source
	backup := missingSource + ".backup"
	if err := os.Rename(missingSource, backup); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	s.runner = func(func(context.Context, string) ([]byte, error)) Runner {
		return &Engine{FS: s.access}
	}
	r = s.work(context.Background(), r)
	if r.State != "failed" {
		t.Fatal(r.State, r.Error)
	}
	stored, err := store.Run(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireMeasured(t, &stored.Ops[0].Operation, stored.Ops[0].Size)
	requireMeasured(t, &stored.Ops[1].Operation, 0)
	completedDuration, completedSpeed := *stored.Ops[0].DurationMs, *stored.Ops[0].SpeedBps
	if err := os.Rename(backup, missingSource); err != nil {
		t.Fatal(err)
	}
	r.State, r.Completed = "running", 0
	r = s.work(context.Background(), r)
	if r.State != "completed" {
		t.Fatal(r.State, r.Error)
	}
	stored, err = store.Run(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireMeasured(t, &stored.Ops[1].Operation, stored.Ops[1].Size)
	if *stored.Ops[0].DurationMs != completedDuration || *stored.Ops[0].SpeedBps != completedSpeed {
		t.Fatal("worker retry overwrote the completed item's persisted metrics")
	}
	beforeUndo := snapshot(stored.Ops)
	r.State, r.Undo, r.Completed = "running", true, 0
	r = s.work(context.Background(), r)
	if r.State != "undone" {
		t.Fatal(r.State, r.Error)
	}
	stored, err = store.Run(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range stored.Ops {
		if *stored.Ops[i].DurationMs != *beforeUndo[i].DurationMs || *stored.Ops[i].SpeedBps != *beforeUndo[i].SpeedBps {
			t.Fatal("worker undo overwrote forward metrics", i)
		}
	}
}
