package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"joycurator/internal/model"
	"joycurator/internal/planning"
)

func TestBatchDuplicatePolicySkipAndOverwrite(t *testing.T) {
	for _, policy := range []string{"skip", "overwrite"} {
		t.Run(policy, func(t *testing.T) {
			a, _, task, initial := batchFixture(t)
			target := initial.Rows[0].Target
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("old video"), 0644); err != nil {
				t.Fatal(err)
			}
			plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy", DuplicatePolicy: policy})
			if err != nil || len(plan.Issues) > 0 {
				t.Fatal(plan, err)
			}
			if plan.Rows[0].Action != policy || plan.DuplicatePolicy != policy {
				t.Fatal(plan.Rows[0])
			}
			if policy == "skip" && len(plan.Groups[0].Ops) != 0 {
				t.Fatal("skipped video produced side effects")
			}
			batch, err := a.dependencies.Organizer.ExecuteBatch(plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			batch = waitBatch(t, a, batch.ID)
			for _, run := range batch.Runs {
				if run.State != "completed" {
					t.Fatal(run)
				}
			}
			if policy == "skip" {
				if len(batch.Runs) != 1 {
					t.Fatal("created empty run")
				}
				got, _ := os.ReadFile(target)
				if string(got) != "old video" {
					t.Fatal("skip modified existing video")
				}
				current, _ := a.dependencies.Workspace.Read(task.ID)
				if current.Items[0].Removed || !current.Items[0].Selected {
					t.Fatal("skip removed source from workspace")
				}
			} else {
				if len(batch.Runs) != 2 {
					t.Fatal(batch)
				}
				run, err := a.dependencies.Organizer.Resume(batch.Runs[0].ID, true)
				if err != nil {
					t.Fatal(err)
				}
				if done(t, a, run).State != "undone" {
					t.Fatal("undo failed")
				}
				got, _ := os.ReadFile(target)
				if string(got) != "old video" {
					t.Fatal("old video not restored")
				}
			}
		})
	}
}

func TestDuplicatePoliciesDoNotAllowInBatchCollisions(t *testing.T) {
	for _, policy := range []string{"skip", "overwrite"} {
		t.Run(policy, func(t *testing.T) {
			a, root, task, _ := batchFixture(t)
			s, _ := a.dependencies.Config.Load()
			s.NFO, s.Images = false, false
			s.Rules.MovieFolder, s.Rules.TVFolder = "collision", "collision"
			s.Rules.MovieFile, s.Rules.TVFile = "video.{ext}", "video.{ext}"
			s.Version++
			if _, err := testDB(t, a).Exec("UPDATE config SET value=?", jsonText(s)); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "target", "collision", "video.mkv")
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("existing"), 0644); err != nil {
				t.Fatal(err)
			}
			plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy", DuplicatePolicy: policy})
			if err != nil || !strings.Contains(strings.Join(plan.Issues, " "), "作品之间目标重复") {
				t.Fatal(plan, err)
			}
			if _, err = a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
				t.Fatal("colliding batch executed")
			}
		})
	}
}

func TestDuplicateSubtitleAndNonregularTargetPolicies(t *testing.T) {
	for _, policy := range []string{"skip", "overwrite"} {
		t.Run(policy, func(t *testing.T) {
			a, root := fixture(t)
			file := mediaFile(t, a, root, "film.mkv")
			subtitle := strings.TrimSuffix(file.Path, ".mkv") + ".srt"
			if err := os.WriteFile(subtitle, []byte("new subtitle"), 0644); err != nil {
				t.Fatal(err)
			}
			in := planning.PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{file}, DuplicatePolicy: policy}
			first, err := a.dependencies.Planning.Preview(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			target := strings.TrimSuffix(first.Rows[0].Target, ".mkv") + ".srt"
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(target, []byte("old subtitle"), 0644); err != nil {
				t.Fatal(err)
			}
			plan, err := a.dependencies.Planning.Preview(context.Background(), in)
			if err != nil || len(plan.Issues) > 0 {
				t.Fatal(plan, err)
			}
			run := executePlan(t, a, plan)
			want := "old subtitle"
			if policy == "overwrite" {
				want = "new subtitle"
			}
			if got, _ := os.ReadFile(target); string(got) != want {
				t.Fatal("unexpected subtitle")
			}
			undo, err := a.dependencies.Organizer.Resume(run.ID, true)
			if err != nil || done(t, a, undo).State != "undone" {
				t.Fatal("undo failed", err)
			}
			if got, _ := os.ReadFile(target); string(got) != "old subtitle" {
				t.Fatal("original subtitle lost")
			}
		})
		for _, kind := range []string{"directory", "symlink"} {
			t.Run(policy+"-"+kind, func(t *testing.T) {
				a, root := fixture(t)
				file := mediaFile(t, a, root, "film.mkv")
				in := planning.PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{file}, DuplicatePolicy: policy}
				initial, err := a.dependencies.Planning.Preview(context.Background(), in)
				if err != nil {
					t.Fatal(err)
				}
				target := initial.Rows[0].Target
				if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					err = os.Mkdir(target, 0755)
				} else {
					err = os.Symlink(file.Path, target)
				}
				if err != nil {
					t.Fatal(err)
				}
				plan, err := a.dependencies.Planning.Preview(context.Background(), in)
				if err != nil || !strings.Contains(strings.Join(plan.Issues, " "), "不是普通文件") {
					t.Fatal(plan, err)
				}
			})
		}
	}
}

func TestDuplicatePolicyRevalidatesTargetAndRejectsInvalidPolicy(t *testing.T) {
	a, _, task, initial := batchFixture(t)
	in := planning.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy", DuplicatePolicy: "overwrite"}
	target := initial.Rows[0].Target
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old video"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Planning.BatchPreview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed since preview"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
		t.Fatal("changed target was overwritten")
	}
	for _, policy := range []string{"", "invalid"} {
		in.DuplicatePolicy = policy
		if _, err = a.dependencies.Planning.BatchPreview(context.Background(), in); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}

func TestAllSkippedBatchDoesNotCreateRuns(t *testing.T) {
	a, _, task, initial := batchFixture(t)
	for _, row := range initial.Rows {
		if err := os.MkdirAll(filepath.Dir(row.Target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(row.Target, []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "move", DuplicatePolicy: "skip"})
	if err != nil || len(plan.Issues) != 0 || len(plan.Ops) != 0 || plan.Required != 0 {
		t.Fatal(plan, err)
	}
	if _, err = a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
		t.Fatal("empty batch accepted")
	}
	for _, item := range task.Items {
		if _, err = os.Stat(item.Path); err != nil {
			t.Fatal("skipped move changed source", err)
		}
	}
}

func TestDuplicatePoliciesDoNotOverwriteNewlyAppearedTargets(t *testing.T) {
	for _, policy := range []string{"skip", "overwrite"} {
		t.Run(policy, func(t *testing.T) {
			a, _, task, _ := batchFixture(t)
			plan, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{RecognitionID: task.ID, Revision: task.Revision, Mode: "copy", DuplicatePolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			target := plan.Rows[0].Target
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(target, []byte("created after preview"), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err = a.dependencies.Organizer.ExecuteBatch(plan.ID); err == nil {
				t.Fatal("executed stale conflict decision")
			}
			if got, _ := os.ReadFile(target); string(got) != "created after preview" {
				t.Fatal("newly appeared file changed")
			}
		})
	}
}

func TestSkipStillDetectsSameWorkTargetCollision(t *testing.T) {
	a, root := fixture(t)
	first := mediaFile(t, a, root, "first.mkv")
	second := mediaFile(t, a, root, "second.mkv")
	in := planning.PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{first, second}, DuplicatePolicy: "skip"}
	initial, err := a.dependencies.Planning.Preview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	target := initial.Rows[0].Target
	if target != initial.Rows[1].Target {
		t.Fatal("fixture did not collide")
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, []byte("existing video"), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := a.dependencies.Planning.Preview(context.Background(), in)
	if err != nil || !strings.Contains(strings.Join(plan.Issues, " "), "目标重复") {
		t.Fatal(plan, err)
	}
}

func TestOverwriteRestoresIdentityForEarlierRunUndo(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "film.mkv")
	in := planning.PreviewRequest{Type: "movie", TMDBID: 1, Mode: "copy", Files: []model.File{file}, DuplicatePolicy: "error"}
	first, err := a.dependencies.Planning.Preview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	oldRun := executePlan(t, a, first)
	in.DuplicatePolicy = "overwrite"
	second, err := a.dependencies.Planning.Preview(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	newRun := executePlan(t, a, second)
	undo, err := a.dependencies.Organizer.Resume(newRun.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, undo).State != "undone" {
		t.Fatal("replacement undo failed")
	}
	undo, err = a.dependencies.Organizer.Resume(oldRun.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done(t, a, undo).State != "undone" {
		t.Fatal("restored original identity no longer matches earlier record")
	}
}
