package app

import (
	"context"
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
	first := filepath.Join(dir, "[组][作品][202][01][1080p].mkv")
	second := filepath.Join(dir, "[组][作品][202][03][720p].mkv")
	other := filepath.Join(dir, "子目录", "Other.S02E04.mkv")
	for _, p := range []string{first, second, other, filepath.Join(dir, ".hidden.mkv")} {
		if err = os.WriteFile(p, []byte("video"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := a.dependencies.Workspace.extractContexts(context.Background(), []string{first}, []string{first}, s, "batch")
	if err != nil {
		t.Fatal(err)
	}
	item := got[first]
	if len(got) != 1 || !reflect.DeepEqual(item.Metadata.Episodes, []int{1}) || item.Metadata.Title == nil || *item.Metadata.Title != "作品" {
		t.Fatal(got)
	}
	_, steps, err := a.dependencies.Recognition.Detail(item.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := recognition.RestoreBatch(steps)
	if err != nil || len(restored.Documents) != 3 || len(restored.Files) != 1 || len(restored.Files[0].Decisions[recognition.Episodes].Candidates) < 2 {
		t.Fatal(restored, err)
	}
	got, err = a.dependencies.Workspace.extractContexts(context.Background(), []string{dir, first}, []string{first, second}, s, "batch")
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
	got, err = a.dependencies.Workspace.extractContexts(context.Background(), []string{dir, disjoint}, []string{first, last}, s, "batch")
	if err != nil || got[first].TraceID == got[last].TraceID {
		t.Fatal(got, err)
	}
}
