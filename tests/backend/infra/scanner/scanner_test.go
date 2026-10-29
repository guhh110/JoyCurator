package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"joycurator/internal/infra/fsops"
)

func TestWalkExclusionsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "library")
	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"film.mkv", "partial.mkv.part", ".hidden.mkv", "library/organized.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("media"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "private.mkv"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.mkv"), filepath.Join(dir, "link.mkv")); err != nil {
		t.Fatal(err)
	}
	access, err := fsops.New([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	err = Walk(context.Background(), access, []string{dir}, []string{output}, func(path string, info fs.FileInfo) error { names = append(names, filepath.Base(path)); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"film.mkv"}) {
		t.Fatal(names)
	}
}
