package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPreviewUsesWorkspaceWithoutTMDBAndRejectsStaleFiles(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "Film.4K.x265.mkv")
	input := previewRequest(t, a, "movie", 1, "copy", "error", file)
	if _, err := testDB(t, a).Exec("DELETE FROM tmdb_cache"); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Organizer.BatchPreview(context.Background(), input)
	if err != nil || len(plan.Issues) != 0 || !strings.Contains(plan.Rows[0].Target, "2160p") {
		t.Fatal("offline preview failed", plan, err)
	}
	input.DuplicatePolicy = "skip"
	if _, err = a.dependencies.Organizer.BatchPreview(context.Background(), input); err != nil {
		t.Fatal("policy switch queried TMDB", err)
	}
	input.Revision++
	if _, err = a.dependencies.Organizer.BatchPreview(context.Background(), input); err == nil {
		t.Fatal("stale revision accepted")
	}
	input.Revision--
	if err = os.WriteFile(file.Path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err = a.dependencies.Organizer.BatchPreview(context.Background(), input)
	if err != nil || len(plan.Issues) == 0 {
		t.Fatal("changed source accepted", err)
	}
}
