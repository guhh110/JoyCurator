package app

import (
	"context"
	"io/fs"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/recognition"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSingleTargetReadsFullParentTreeAndSharesTrace(t *testing.T) {
	a, root := fixture(t)
	s, err := a.dependencies.Config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "source", "作品")
	if err = os.MkdirAll(filepath.Join(dir, "子目录"), 0755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "[组]作品.01.第一季.[202].1080p.mkv")
	second := filepath.Join(dir, "[组]作品.03.第一季.[202].1080p.mkv")
	other := filepath.Join(dir, "子目录", "Other.S02E04.mkv")
	for _, p := range []string{first, second, other, filepath.Join(dir, ".hidden.mkv")} {
		if err = os.WriteFile(p, []byte("video"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.dependencies.Workspace.extractContexts(context.Background(), []string{first}, []string{first}, s, "batch", "organize", nil)
	if err != nil {
		t.Fatal(err)
	}
	item := got[first]
	if len(got) != 1 || !reflect.DeepEqual(item.Metadata.Episodes, []int{1}) || item.Metadata.Title == nil || *item.Metadata.Title != "作品" {
		t.Fatal(got)
	}
	_, restored, err := a.dependencies.Recognition.Detail(item.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || len(restored.Documents) != 3 || len(restored.Files) != 1 || len(restored.Files[0].Decisions[recognition.Episodes].Candidates) < 2 {
		t.Fatal(restored, err)
	}
	got, err = a.dependencies.Workspace.extractContexts(context.Background(), []string{dir, first}, []string{first, second}, s, "batch", "organize", nil)
	if err != nil || got[first].TraceID != got[second].TraceID {
		t.Fatal(got, err)
	}
	disjoint := filepath.Join(root, "source", "单独")
	if err = os.MkdirAll(disjoint, 0755); err != nil {
		t.Fatal(err)
	}
	last := filepath.Join(disjoint, "Film.2024.mkv")
	if err = os.WriteFile(last, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = a.dependencies.Workspace.extractContexts(context.Background(), []string{dir, disjoint}, []string{first, last}, s, "batch", "organize", nil)
	if err != nil || got[first].TraceID == got[last].TraceID {
		t.Fatal(got, err)
	}
}

func TestDirectoryImportReusesScanButFileImportReadsSiblings(t *testing.T) {
	a, root := fixture(t)
	s, err := a.dependencies.Config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "source")
	first := filepath.Join(dir, "Show.S01E01.mkv")
	second := filepath.Join(dir, "Show.S01E02.mkv")
	if err = os.WriteFile(first, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	snapshot := map[string]fs.FileInfo{}
	if err = scanner.Walk(context.Background(), a.dependencies.Workspace.files.Access, []string{dir}, nil, func(path string, info fs.FileInfo) error {
		snapshot[path] = info
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// This file arrived after the directory snapshot was captured.
	if err = os.WriteFile(second, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		imports []string
		files   int
	}{
		{[]string{dir}, 1},
		{[]string{first}, 2},
		{[]string{first, dir}, 1},
	} {
		got, err := a.dependencies.Workspace.extractContexts(context.Background(), tc.imports, []string{first}, s, "batch", "organize", snapshot)
		if err != nil {
			t.Fatal(err)
		}
		_, restored, err := a.dependencies.Recognition.Detail(got[first].TraceID)
		if err != nil {
			t.Fatal(err)
		}
		if err != nil || len(restored.Documents) != tc.files || len(restored.Files) != 1 {
			t.Fatalf("imports %v: documents=%d, targets=%d, error=%v", tc.imports, len(restored.Documents), len(restored.Files), err)
		}
	}
}
