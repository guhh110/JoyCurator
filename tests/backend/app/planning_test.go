package app

import (
	"context"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/planning"
)

func TestPlanEditInvalidatesOldConfirmationAndDeleteOnlyDiscardsPreview(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "planned.mkv")
	input := planning.PreviewRequest{DuplicatePolicy: "error", Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{file}}
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
