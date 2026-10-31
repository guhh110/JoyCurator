package app

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"joycurator/internal/organizer"
)

func TestPreviewUsesWorkspaceWithoutTMDBAndRejectsStaleFiles(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "Film.4K.x265.mkv")
	input := previewRequest(t, a, "movie", 1, "copy", "error", file)
	if _, err := testDB(t, a).Exec("DELETE FROM tmdb_cache"); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Planning.Preview(context.Background(), input)
	if err != nil || len(plan.Issues) != 0 || !strings.Contains(plan.Rows[0].Target, "2160p") {
		t.Fatal("offline preview failed", plan, err)
	}
	input.DuplicatePolicy = "skip"
	if _, err = a.dependencies.Planning.Preview(context.Background(), input); err != nil {
		t.Fatal("policy switch queried TMDB", err)
	}
	input.Revision++
	if _, err = a.dependencies.Planning.Preview(context.Background(), input); err == nil {
		t.Fatal("stale revision accepted")
	}
	input.Revision--
	if err = os.WriteFile(file.Path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err = a.dependencies.Planning.Preview(context.Background(), input)
	if err != nil || len(plan.Issues) == 0 {
		t.Fatal("changed source accepted", err)
	}
}

func TestPlanEditInvalidatesOldConfirmationAndDeleteOnlyDiscardsPreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows organizer execution unsupported")
	}
	a, root := fixture(t)
	file := mediaFile(t, a, root, "planned.mkv")
	input := previewRequest(t, a, "movie", 1, "copy", "error", file)
	old, err := a.dependencies.Planning.Preview(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Mode = "hardlink"
	edited, err := a.dependencies.Planning.Edit(context.Background(), old.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID == old.ID || edited.Mode != "hardlink" || a.dependencies.Planning.Get(old.ID) != nil {
		t.Fatal("old confirmation remains valid", edited)
	}
	if _, err = a.dependencies.Planning.Submit(old.ID, false); err == nil {
		t.Fatal("edited plan accepted old confirmation")
	}
	if err = a.dependencies.Planning.Delete(edited.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Planning.Submit(edited.ID, false); err == nil {
		t.Fatal("deleted preview executed")
	}
	records, err := a.dependencies.Organizer.Records(1)
	if err != nil || records["total"] != 0 {
		t.Fatal(records, err)
	}
	// Another plan can execute normally through planning, then has a durable Run.
	current, err := a.dependencies.Planning.Preview(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := a.dependencies.Planning.Submit(current.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := receipt.(organizer.Run)
	if !ok {
		t.Fatalf("unexpected receipt %T", receipt)
	}
	if result := done(t, a, run); result.State != "completed" {
		t.Fatal(result)
	}
	if err = a.dependencies.Planning.Delete(current.ID); err == nil {
		t.Fatal("submitted plan remained editable")
	}
	stored, err := a.dependencies.Organizer.Record(run.ID)
	if err != nil || stored.State != "completed" {
		t.Fatal("preview deletion affected record", stored, err)
	}
}
