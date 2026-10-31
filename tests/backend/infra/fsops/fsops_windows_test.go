package fsops_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"joycurator/internal/infra/fsops"
)

func TestWindowsConfinedOperations(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific handle semantics")
	}
	root := t.TempDir()
	outside := t.TempDir()
	f, err := fsops.New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := f.Link(source, link); err != nil {
		t.Fatal(err)
	}
	s, _ := os.Stat(source)
	d, _ := os.Stat(link)
	if !os.SameFile(s, d) {
		t.Fatal("hardlink changed file identity")
	}
	target := filepath.Join(root, "target.txt")
	if err := f.Publish(link, target); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove: %v", err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := f.Publish(source, target); err == nil {
		t.Fatal("published over existing target")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "existing" {
		t.Fatalf("target overwritten: %q, %v", got, err)
	}
	if _, err := f.Open(filepath.Join(outside, "escape")); err == nil {
		t.Fatal("opened outside root")
	}
	if err := os.Symlink(outside, filepath.Join(root, "junction")); err != nil {
		t.Logf("creating junction unavailable: %v", err)
		return
	}
	if err := f.Publish(source, filepath.Join(root, "junction", "escape")); err == nil {
		t.Fatal("published through junction")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped root: %v", err)
	}
}

func TestWindowsCheckTargetAndIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific identity semantics")
	}
	root := t.TempDir()
	f, err := fsops.New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, "missing", "target.txt")
	free, dev, err := f.CheckTarget(name)
	if err != nil || free <= 0 || dev == 0 {
		t.Fatalf("check target: %d %d %v", free, dev, err)
	}
	if _, _, err := f.CheckTarget(filepath.Join(t.TempDir(), "outside")); err == nil {
		t.Fatal("outside target allowed")
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fsops.Identity(info) != "" {
		t.Fatal("unsafe synthetic Windows file identity")
	}
}
