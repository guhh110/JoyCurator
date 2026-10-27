package organizer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"joycurator/internal/fsops"
	"joycurator/internal/model"
)

func fixture(t *testing.T, mode string) (*Engine, *model.Operation, []byte) {
	t.Helper()
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
	if err := e.Undo(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("undo removed modified file")
	}
}
func TestRecoveryAfterPublishBeforeJournal(t *testing.T) {
	e, op, _ := fixture(t, "move")
	crash := errors.New("simulated crash")
	var durable []byte
	if err := e.Run(context.Background(), op, func() error {
		if op.Status == "published" {
			return crash
		}
		durable, _ = json.Marshal(op)
		return nil
	}); !errors.Is(err, crash) {
		t.Fatal(err)
	}
	if err := json.Unmarshal(durable, op); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if op.Status != "completed" {
		t.Fatal(op.Status)
	}
	if _, err := os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source not removed after recovery")
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

func TestSameDeviceMoveUsesLinkStaging(t *testing.T) {
	e, op, _ := fixture(t, "move")
	stopAfterPublish(t, e, op)
	source, err := os.Stat(op.Source)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Stat(op.Target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(source, target) {
		t.Fatal("same-device move did not stage through a link")
	}
}

func TestCrossDeviceMoveCopiesBeforeRemovingSource(t *testing.T) {
	e, op, body := fixture(t, "move")
	// Pretend the target lives on another filesystem.
	e.FS.DeviceOf = func(os.FileInfo) uint64 { return 1 }
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
	if err = e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("source kept after cross-device move")
	}
	got, err := os.ReadFile(op.Target)
	if err != nil || string(got) != string(body) {
		t.Fatal("target content wrong", err)
	}
}
