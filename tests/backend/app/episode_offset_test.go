package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/planning"
	"joycurator/internal/recognition"
)

func TestWorkspaceEpisodeZeroValidationAndPreview(t *testing.T) {
	for _, tc := range []struct {
		name    string
		numbers []int
		catalog string
		ready   bool
	}{
		{"episode-zero", []int{0}, `{"episodes":[{"episode_number":0},{"episode_number":1}]}`, true},
		{"merged-from-zero", []int{0, 1}, `{"episodes":[{"episode_number":0},{"episode_number":1}]}`, true},
		{"zero-missing-in-tmdb", []int{0}, `{"episodes":[{"episode_number":1}]}`, false},
		{"negative-even-in-tmdb", []int{-1}, `{"episodes":[{"episode_number":-1}]}`, false},
		{"duplicate-zero", []int{0, 0}, `{"episodes":[{"episode_number":0}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, root := fixture(t)
			file := mediaFile(t, a, root, "Show.S01E01.mkv")
			file.Season, file.Episodes = 1, []int{1}
			task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{file.Path}, Total: 1, Items: []matching.Item{
				{File: file, Metadata: recognition.Metadata{Episodes: []int{1}}, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2, Title: "山海旅人", Year: 2023}, State: "ready", Selected: true},
			}}
			seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
			seedRecognitionCache(t, a, "/tv/2/season/1?language=zh-CN", tc.catalog)
			if err := a.store.SaveWorkspace(&task); err != nil {
				t.Fatal(err)
			}
			in := matching.Update{Revision: task.Revision, Items: []matching.Edit{
				{Path: file.Path, Group: "tv", Type: "tv", TMDBID: 2, Season: 1, Episodes: tc.numbers, Selected: true},
			}}
			updated, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
			if err != nil {
				t.Fatal(err)
			}
			if (updated.Items[0].State == "ready") != tc.ready || updated.Items[0].Selected != tc.ready {
				t.Fatal(updated.Items[0])
			}
			reloaded, err := a.dependencies.Workspace.Read(task.ID)
			if err != nil || !slices.Equal(reloaded.Items[0].Episodes, tc.numbers) || !slices.Equal(reloaded.Items[0].Metadata.Episodes, []int{1}) {
				t.Fatal(reloaded, err)
			}
			plan, err := a.dependencies.Planning.Preview(context.Background(), planning.PreviewRequest{RecognitionID: task.ID, Revision: reloaded.Revision, Paths: []string{file.Path}, Mode: "copy", DuplicatePolicy: "error"})
			if !tc.ready {
				if err == nil {
					t.Fatal("unconfirmed mapping was previewed", plan)
				}
				return
			}
			if err != nil || len(plan.Issues) != 0 {
				t.Fatal(plan, err)
			}
			if tc.ready {
				if len(plan.Rows) != 1 || !strings.Contains(plan.Rows[0].Target, "S01E00") {
					t.Fatal(plan.Rows)
				}
				in.Revision = reloaded.Revision
				repeated, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
				if err != nil || !slices.Equal(repeated.Items[0].Episodes, tc.numbers) {
					t.Fatal(repeated, err)
				}
				batch, err := a.dependencies.Planning.BatchPreview(context.Background(), planning.BatchPreviewRequest{DuplicatePolicy: "error", RecognitionID: task.ID, Revision: repeated.Revision, Mode: "copy"})
				if err != nil || len(batch.Issues) != 0 || len(batch.Rows) != 1 {
					t.Fatal(batch, err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(root, "target"))
			if err != nil || len(entries) != 0 {
				t.Fatal("preview must not organize files", entries, err)
			}
		})
	}
}

func TestWorkspaceEpisodeOffsetPreservesOriginalAndRetry(t *testing.T) {
	a, root := fixture(t)
	first := mediaFile(t, a, root, "Show.S01E01.mkv")
	merged := mediaFile(t, a, root, "Show.S01E02-E03.mkv")
	first.Season, first.Episodes = 1, []int{1}
	merged.Season, merged.Episodes = 1, []int{2, 3}
	task := matching.Workspace{ID: model.Key(), State: "completed", Paths: []string{first.Path, merged.Path}, Total: 2,
		Items: []matching.Item{
			{File: first, Metadata: recognition.Metadata{Episodes: []int{1}}, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "review"},
			{File: merged, Metadata: recognition.Metadata{Episodes: []int{2, 3}}, Group: "tv", Entry: &model.Entry{Type: "tv", TMDBID: 2}, State: "ready", Selected: true},
		}}
	seedRecognitionCache(t, a, "/tv/2?language=zh-CN", `{"seasons":[{"season_number":1}]}`)
	seedRecognitionCache(t, a, "/tv/2/season/1?language=zh-CN", `{"episodes":[{"episode_number":10},{"episode_number":11},{"episode_number":12}]}`)
	if err := a.store.SaveWorkspace(&task); err != nil {
		t.Fatal(err)
	}
	in := matching.Update{Revision: task.Revision, Items: []matching.Edit{
		{Path: first.Path, Group: "tv", Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{10}},
		{Path: merged.Path, Group: "tv", Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{11, 12}, Selected: true},
	}}
	updated, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	for i, original := range [][]int{{1}, {2, 3}} {
		if !slices.Equal(updated.Items[i].Metadata.Episodes, original) || !updated.Items[i].Edited || updated.Items[i].State != "ready" {
			t.Fatalf("lost original metadata or validation: %+v", updated.Items[i])
		}
	}
	if updated.Items[0].Selected || !updated.Items[1].Selected {
		t.Fatal("mapping correction changed organizer selection")
	}
	if _, err = a.dependencies.Workspace.Update(context.Background(), task.ID, in); err == nil {
		t.Fatal("stale offset update accepted")
	}
	// Saving the same absolute mapping again is idempotent, including after a
	// fresh database read. Current episode numbers never become raw metadata.
	in.Revision = updated.Revision
	if _, err = a.dependencies.Workspace.Update(context.Background(), task.ID, in); err != nil {
		t.Fatal(err)
	}
	reloaded, err := a.dependencies.Workspace.Read(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, mapped := range [][]int{{10}, {11, 12}} {
		if !slices.Equal(reloaded.Items[i].Episodes, mapped) || !slices.Equal(reloaded.Items[i].Metadata.Episodes, task.Items[i].Metadata.Episodes) {
			t.Fatalf("repeated offset accumulated or changed its base: %+v", reloaded.Items[i])
		}
	}
	// Retrying recognition must not re-apply the offset or discard the mapping.
	if _, err = a.dependencies.Workspace.Start(nil, task.ID); err != nil {
		t.Fatal(err)
	}
	retried := waitRecognition(t, a, task.ID)
	for i, mapped := range [][]int{{10}, {11, 12}} {
		if !slices.Equal(retried.Items[i].Episodes, mapped) || !slices.Equal(retried.Items[i].Metadata.Episodes, task.Items[i].Metadata.Episodes) {
			t.Fatal(retried.Items[i])
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "target"))
	if err != nil || len(entries) != 0 || a.coordinator.Busy() {
		t.Fatal("updating a mapping started file organization", entries, err)
	}
	// The server remains authoritative even if a client submits an invalid map.
	in.Revision = retried.Revision
	in.Items[0].Episodes, in.Items[0].Selected = []int{999}, true
	invalid, err := a.dependencies.Workspace.Update(context.Background(), task.ID, in)
	if err != nil || invalid.Items[0].State != "review" || invalid.Items[0].Selected {
		t.Fatal(invalid, err)
	}
}
