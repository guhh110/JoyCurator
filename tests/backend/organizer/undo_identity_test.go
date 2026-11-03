package organizer

import (
	"context"
	"os"
	"testing"
)

func TestFastUndoRejectsReplacedFileOfSameSize(t *testing.T) {
	for _, mode := range []string{"copy", "move", "hardlink", "cross-device"} {
		t.Run(mode, func(t *testing.T) {
			actual := mode
			if mode == "cross-device" {
				actual = "move"
			}
			e, op, body := fixture(t, actual)
			if mode == "cross-device" {
				e.FS.DeviceOf = func(os.FileInfo) uint64 { return ^uint64(0) }
			}
			if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			// Retain the old inode, so the filesystem cannot reuse it for the replacement.
			if err := os.Rename(op.Target, op.Target+".old"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(op.Target, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err := e.Undo(context.Background(), op, func() error { return nil }); err == nil {
				t.Fatal("foreign identity accepted")
			}
			if got, err := os.ReadFile(op.Target); err != nil || string(got) != string(body) {
				t.Fatal("foreign file changed", err)
			}
			if actual == "move" {
				if _, err := os.Stat(op.Source); !os.IsNotExist(err) {
					t.Fatal("foreign file restored to source")
				}
			}
		})
	}
}

func TestFastUndoChecksChangesAfterCheckpoint(t *testing.T) {
	e, op, _ := fixture(t, "copy")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	err := e.Undo(context.Background(), op, func() error {
		return os.WriteFile(op.Target, []byte("changed after checkpoint"), 0600)
	})
	if err == nil {
		t.Fatal("checkpoint race accepted")
	}
	if got, _ := os.ReadFile(op.Target); string(got) != "changed after checkpoint" {
		t.Fatal("changed file removed")
	}
}

func TestNFOOverwriteAndUndo(t *testing.T) {
	e, op, _ := overwriteFixture(t, "")
	op.Kind, op.Source, op.Content = "nfo", "", []byte("<tvshow><title>new</title></tvshow>")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(op.Target); string(got) != string(op.Content) {
		t.Fatal("nfo was not replaced")
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(op.Target); !os.IsNotExist(err) {
		t.Fatal("nfo undo retained target", err)
	}
}
