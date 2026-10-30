package organizer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

func overwriteFixture(t *testing.T, mode string) (*Engine, *model.Operation, []byte) {
	t.Helper()
	e, op, body := fixture(t, mode)
	if err := os.MkdirAll(filepath.Dir(op.Target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("original library video"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(op.Target)
	if err != nil {
		t.Fatal(err)
	}
	op.Backup = &model.Operation{Kind: "backup", Mode: "move", Source: op.Target, Target: op.Target + ".backup", Size: info.Size(), MTime: info.ModTime().UnixNano(), SourceIdentity: fsops.Identity(info), Status: "pending"}
	return e, op, body
}

func TestOverwriteAndUndoRestoreBothFiles(t *testing.T) {
	for _, mode := range []string{"copy", "move", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			e, op, body := overwriteFixture(t, mode)
			save := func() error { return nil }
			if err := e.Run(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(op.Target)
			old, _ := os.ReadFile(op.Backup.Target)
			if string(got) != string(body) || string(old) != "original library video" {
				t.Fatal("replacement or backup lost")
			}
			if err := e.Undo(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
			got, _ = os.ReadFile(op.Target)
			source, _ := os.ReadFile(op.Source)
			if string(got) != "original library video" || string(source) != string(body) {
				t.Fatal("undo did not restore both files")
			}
			if _, err := os.Stat(op.Backup.Target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("backup not consumed")
			}
			if err := e.Undo(context.Background(), op, save); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOverwriteCrashRecoveryAndUndo(t *testing.T) {
	for _, phase := range []string{"backup-staged", "backup-published", "backup-completed", "preparing", "staged", "published", "completed"} {
		for _, undo := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "-resume", true: "-undo"}[undo], func(t *testing.T) {
				e, op, body := overwriteFixture(t, "move")
				crash := errors.New("crash")
				durable, _ := json.Marshal(op)
				err := e.Run(context.Background(), op, func() error {
					current := op.Status
					if op.Backup.Status != "completed" || op.Status == "pending" {
						current = "backup-" + op.Backup.Status
					}
					if current == phase {
						return crash
					}
					durable, _ = json.Marshal(op)
					return nil
				})
				if !errors.Is(err, crash) {
					t.Fatal("did not reach crash point", err)
				}
				op = &model.Operation{}
				if err = json.Unmarshal(durable, op); err != nil {
					t.Fatal(err)
				}
				if undo {
					err = e.Undo(context.Background(), op, func() error { return nil })
				} else {
					err = e.Run(context.Background(), op, func() error { return nil })
				}
				if err != nil {
					t.Fatal(err)
				}
				if !undo {
					if got, _ := os.ReadFile(op.Target); string(got) != string(body) {
						t.Fatal("incorrect replacement")
					}
					err = e.Undo(context.Background(), op, func() error { return nil })
				}
				if err != nil {
					t.Fatal(err)
				}
				if got, _ := os.ReadFile(op.Target); string(got) != "original library video" {
					t.Fatal("lost original")
				}
			})
		}
	}
}

func TestOverwriteCancelAndUndoRecovery(t *testing.T) {
	for _, phase := range []string{"undoing", "undone", "backup-undoing", "backup-undone"} {
		t.Run(phase, func(t *testing.T) {
			e, op, _ := overwriteFixture(t, "move")
			if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			durable, _ := json.Marshal(op)
			crash := errors.New("undo crash")
			err := e.Undo(context.Background(), op, func() error {
				current := op.Status
				if op.Backup.Status == "undoing" || op.Backup.Status == "undone" {
					current = "backup-" + op.Backup.Status
				}
				if current == phase {
					return crash
				}
				durable, _ = json.Marshal(op)
				return nil
			})
			if !errors.Is(err, crash) {
				t.Fatal("missed undo crash", err)
			}
			op = &model.Operation{}
			if err = json.Unmarshal(durable, op); err != nil {
				t.Fatal(err)
			}
			if err = e.Undo(context.Background(), op, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(op.Target); string(got) != "original library video" {
				t.Fatal("old video lost")
			}
		})
	}
	e, op, _ := overwriteFixture(t, "copy")
	ctx, cancel := context.WithCancel(context.Background())
	err := e.Run(ctx, op, func() error {
		if op.Backup.Status == "completed" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if err = e.Undo(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(op.Target); string(got) != "original library video" {
		t.Fatal("cancelled backup not restored")
	}
}

func TestOverwriteUndoChecksBackupBeforeRemovingReplacement(t *testing.T) {
	e, op, body := overwriteFixture(t, "copy")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Backup.Target, []byte("damaged backup"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("undo accepted damaged backup")
	}
	if got, _ := os.ReadFile(op.Target); string(got) != string(body) {
		t.Fatal("undo destroyed replacement before checking backup")
	}
}

func TestOverwriteRefusesChangedOriginalAndModifiedReplacement(t *testing.T) {
	e, op, _ := overwriteFixture(t, "copy")
	if err := os.WriteFile(op.Target, []byte("external change"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("overwrote changed original")
	}
	if got, _ := os.ReadFile(op.Target); string(got) != "external change" {
		t.Fatal("modified original lost")
	}
	e, op, _ = overwriteFixture(t, "copy")
	if err := e.Run(context.Background(), op, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(op.Target, []byte("external replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := e.Undo(context.Background(), op, func() error { return nil }); err == nil {
		t.Fatal("undo removed modified replacement")
	}
	if got, _ := os.ReadFile(op.Backup.Target); string(got) != "original library video" {
		t.Fatal("backup lost")
	}
}
