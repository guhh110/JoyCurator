package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

type workspaceStore struct{ value matching.Workspace }

func (s *workspaceStore) Workspace(string) (matching.Workspace, error) { return s.value, nil }
func (s *workspaceStore) SaveWorkspace(w *matching.Workspace) error {
	w.Revision++
	s.value = *w
	return nil
}
func (s *workspaceStore) LatestWorkspace() (*matching.Workspace, error) { return &s.value, nil }

func TestCancelledWorkerPersistsTaskAndReleasesRegistration(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "shutdown"}[shutdown], func(t *testing.T) {
			root := t.TempDir()
			source, target := filepath.Join(root, "source"), filepath.Join(root, "target")
			for _, dir := range []string{source, target} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			access, err := fsops.New([]string{source, target})
			if err != nil {
				t.Fatal(err)
			}
			store := &workspaceStore{}
			coordinator := organizer.NewCoordinator()
			service := &WorkspaceService{store: store, coordinator: coordinator, files: WorkspaceFiles{Access: access}}
			ctx, cancel := context.WithCancel(context.Background())
			coordinator.Lock()
			coordinator.TrackRecognitionLocked("workspace", cancel)
			coordinator.Unlock()
			closed := make(chan struct{})
			if shutdown {
				go func() { coordinator.Close(); close(closed) }()
				deadline := time.Now().Add(time.Second)
				for !coordinator.IsClosing() && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if !coordinator.IsClosing() {
					t.Fatal("shutdown not started")
				}
			} else {
				cancel()
			}
			service.recognize(ctx, cancel, matching.Workspace{ID: "workspace", State: "running", Paths: []string{filepath.Join(source, "cancel.mkv")}}, model.Settings{}, nil)
			want := "cancelled"
			if shutdown {
				want = "interrupted"
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("shutdown remained blocked")
				}
			} else {
				coordinator.Close()
			}
			if store.value.State != want || coordinator.RecognitionActive("workspace") {
				t.Fatal(store.value)
			}
		})
	}
}
