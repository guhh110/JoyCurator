package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUndoResumesExactIsolatedLeaf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows execution unsupported")
	}
	e, op, _ := fixture(t, "copy")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	op.Status = "undoing"
	op.RemoveHold = filepath.Join(filepath.Dir(op.Target), ".joycurator-remove-test.part")
	if err := e.FS.IsolateOwned(op.Target, op.RemoveHold, op.Identity); err != nil {
		t.Fatal(err)
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if op.Status != "undone" || op.RemoveHold != "" {
		t.Fatalf("journal not cleared: %+v", op)
	}
	if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("target survived undo")
	}
}
