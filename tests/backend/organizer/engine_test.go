package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

func fixture(t *testing.T, mode string) (*Engine, *model.Operation, []byte) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mkv")
	body := []byte("media bytes for verification")
	if err := os.WriteFile(source, body, 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	access, err := fsops.New([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{FS: access}, &model.Operation{ID: 1, Source: source, Target: filepath.Join(dir, "organized", "target.mkv"), Mode: mode, Kind: "video", Size: info.Size(), MTime: info.ModTime().UnixNano(), Status: "pending"}, body
}
func TestModesAndUndo(t *testing.T) {
	for _, mode := range []string{"copy", "move", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			e, op, body := fixture(t, mode)
			save := func() error { return nil }
			if err := e.Run(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(op.Target)
			if err != nil || string(got) != string(body) {
				t.Fatal("incorrect target", err)
			}
			if mode == "move" {
				if _, err = os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("move retained source")
				}
			}
			if mode == "hardlink" {
				src, _ := os.Stat(op.Source)
				dst, _ := os.Stat(op.Target)
				if !os.SameFile(src, dst) {
					t.Fatal("not a hardlink")
				}
			}
			if err = e.Undo(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("undo retained target")
			}
			got, err = os.ReadFile(op.Source)
			if err != nil || string(got) != string(body) {
				t.Fatal("source not restored", err)
			}
		})
	}
}
func TestNoOverwriteAndExternalChange(t *testing.T) {
	e, op, _ := fixture(t, "copy")
	if err := os.MkdirAll(filepath.Dir(op.Target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("overwrote existing file")
	}
	got, _ := os.ReadFile(op.Target)
	if string(got) != "existing" {
		t.Fatal("existing data changed")
	}
	e, op, _ = fixture(t, "copy")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("external change"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("path-level undo did not remove replacement")
	}
}
func TestInterruptedPublicationNeedsManualReview(t *testing.T) {
	e, op, _ := fixture(t, "move")
	crash := errors.New("simulated crash")
	if err := e.Run(context.Background(), op, func() error {
		if op.Status == "published" {
			return crash
		}
		return nil
	}); !errors.Is(err, crash) {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); !errors.Is(err, errManualReview) {
		t.Fatal("unexpected recovery", err)
	}
	if _, err := os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("direct rename did not remove source")
	}
}
func TestAuthorizedRoots(t *testing.T) {
	e, op, _ := fixture(t, "copy")
	outside := filepath.Join(t.TempDir(), "escaped")
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(filepath.Dir(op.Source), "link")); err != nil {
		t.Fatal(err)
	}
	op.Target = filepath.Join(filepath.Dir(op.Source), "link", "escaped")
	if err := e.Run(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("escaped authorized root")
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("wrote outside root")
	}
}

func TestFastPathRejectsSymlinkTarget(t *testing.T) {
	e, op, _ := fixture(t, "copy")
	if err := os.MkdirAll(filepath.Dir(op.Target), 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(op.Source), "other.mkv")
	if err := os.WriteFile(outside, []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, op.Target); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("symlink target accepted")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "unchanged" {
		t.Fatal("symlink destination changed", err)
	}
}

// stopAfterPublish halts a run right after the target is published, so the
// source file still exists and both sides can be compared.
func stopAfterPublish(t *testing.T, e *Engine, op *model.Operation) {
	t.Helper()
	stop := errors.New("stop after publish")
	if err := e.Run(context.Background(), op, func() error {
		if op.Status == "published" {
			return stop
		}
		return nil
	}); !errors.Is(err, stop) {
		t.Fatalf("run did not stop after publish: %v", err)
	}
}

func TestSameDeviceMoveRenames(t *testing.T) {
	e, op, _ := fixture(t, "move")
	stopAfterPublish(t, e, op)
	if _, err := os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("same-device move retained source")
	}
	if op.Temp != "" {
		t.Fatal("same-device move created a temporary copy")
	}
}

func TestCrossDeviceMoveCopiesBeforeRemovingSource(t *testing.T) {
	e, op, body := fixture(t, "move")
	// Pretend the target lives on another filesystem.
	e.FS.DeviceOf = func(os.FileInfo) uint64 { return ^uint64(0) }
	stopAfterPublish(t, e, op)
	source, err := os.Stat(op.Source)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Stat(op.Target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(source, target) {
		t.Fatal("cross-device move staged through a link instead of copying")
	}
	if err = e.Run(context.Background(), op, func() error { return nil }); !errors.Is(err, errManualReview) {
		t.Fatal("unexpected automatic recovery", err)
	}
	if _, err = os.Stat(op.Source); err != nil {
		t.Fatal("source removed before publication was journaled", err)
	}
	got, err := os.ReadFile(op.Target)
	if err != nil || string(got) != string(body) {
		t.Fatal("target content wrong", err)
	}
}

func TestCrossDeviceMoveAndPathUndo(t *testing.T) {
	e, op, body := fixture(t, "move")
	e.FS.DeviceOf = func(os.FileInfo) uint64 { return ^uint64(0) }
	save := func() error { return nil }
	if err := e.Run(context.Background(), op, save); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source remained after cross-device move")
	}
	if err := e.Undo(context.Background(), op, save); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(op.Source)
	if err != nil || string(got) != string(body) {
		t.Fatal("cross-device undo did not restore source", err)
	}
	if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cross-device undo retained target")
	}
}

func TestOverwriteAndCancel(t *testing.T) {
	e, op, body := fixture(t, "copy")
	if err := os.MkdirAll(filepath.Dir(op.Target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Stat(op.Target)
	op.Replace, op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime = true, fsops.Identity(old), old.Size(), old.ModTime().UnixNano()
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(op.Target)
	if err != nil || string(got) != string(body) {
		t.Fatal("overwrite failed", err)
	}
	if op.ReplaceHold != "" {
		t.Fatal("old file hold not cleaned")
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old file unexpectedly restored")
	}

	e, op, _ = fixture(t, "copy")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = e.Run(ctx, op, func() error {
		if op.Status == "preparing" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || op.Status != "pending" {
		t.Fatal("cancel did not clean owned temporary file", err, op.Status)
	}
	if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancel published target")
	}
}
