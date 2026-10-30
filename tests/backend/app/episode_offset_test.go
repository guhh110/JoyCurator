package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

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
