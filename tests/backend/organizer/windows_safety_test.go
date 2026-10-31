package organizer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"joycurator/internal/model"
)

func TestWindowsRejectsUnverifiableFileOperations(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific file identity guard")
	}
	target := filepath.Join(t.TempDir(), "existing.mkv")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	op := &model.Operation{Target: target, Status: "completed"}
	save := func() error {
		t.Fatal("rejected operation must not save changes")
		return nil
	}
	e := &Engine{}
	if err := e.Run(context.Background(), op, save); err == nil {
		t.Fatal("Windows must reject an operation without stable file identity")
	}
	if err := e.Undo(context.Background(), op, save); err == nil {
		t.Fatal("Windows must reject undo without stable file identity")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep" {
		t.Fatalf("rejected operations modified target: %q, %v", data, err)
	}
}
