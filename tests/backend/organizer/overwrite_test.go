package organizer

import (
	"context"
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func overwriteFixture(t *testing.T, mode string) (*Engine, *model.Operation, []byte) {
	t.Helper()
	e, op, body := fixture(t, mode)
	if err := os.MkdirAll(filepath.Dir(op.Target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("old library file"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(op.Target)
	if err != nil {
		t.Fatal(err)
	}
	op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime = fsops.Identity(info), info.Size(), info.ModTime().UnixNano()
	op.Replace = true
	return e, op, body
}
func TestOverwriteWithoutBackupAndUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	for _, mode := range []string{"copy", "move", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			e, op, body := overwriteFixture(t, mode)
			if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(op.Target); string(got) != string(body) {
				t.Fatal("replacement missing")
			}
			if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(op.Target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("undo retained replacement")
			}
			if got, _ := os.ReadFile(op.Source); string(got) != string(body) {
				t.Fatal("source lost")
			}
		})
	}
}
func TestOverwriteRejectsChangedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	e, op, _ := overwriteFixture(t, "copy")
	if err := os.WriteFile(op.Target, []byte("externally changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("changed target overwritten")
	}
	if got, _ := os.ReadFile(op.Target); string(got) != "externally changed" {
		t.Fatal("changed target lost")
	}
}
