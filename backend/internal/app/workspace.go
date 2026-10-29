package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

type WorkspaceService struct {
	store       WorkspaceStore
	settings    SettingsReader
	factory     matching.Factory
	extract     *recognition.Service
	files       WorkspaceFiles
	coordinator *organizer.Coordinator
}

func NewWorkspace(store WorkspaceStore, settings SettingsReader, factory matching.Factory, extract *recognition.Service, files WorkspaceFiles, c *organizer.Coordinator) *WorkspaceService {
	return &WorkspaceService{store, settings, factory, extract, files, c}
}
func (a *WorkspaceService) Read(id string) (matching.Workspace, error) { return a.store.Workspace(id) }
func (a *WorkspaceService) Latest() (*matching.Workspace, error)       { return a.store.LatestWorkspace() }
func (a *WorkspaceService) Revision(id string) (int, error) {
	w, err := a.Read(id)
	return w.Revision, err
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func (a *WorkspaceService) Start(paths []string, retryID string) (matching.Workspace, error) {
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	if a.coordinator.ClosingLocked() {
		return matching.Workspace{}, errors.New("服务正在关闭")
	}
	if a.coordinator.BusyLocked() {
		return matching.Workspace{}, organizer.ErrBusy
	}
	if a.coordinator.RecognitionActiveLocked() {
		return matching.Workspace{}, errors.New("已有识别任务运行中，请等待完成或取消")
	}
	s, err := a.settings.Load()
	if err != nil {
		return matching.Workspace{}, err
	}
	client, err := a.factory(s, nil)
	if err != nil {
		return matching.Workspace{}, err
	}
	if s.Token == "" {
		return matching.Workspace{}, errors.New("请先配置 TMDB API Read Access Token")
	}
	task := matching.Workspace{ID: model.Key(), State: "running", Paths: paths, Items: []matching.Item{}, Created: time.Now().Format(time.RFC3339)}
	if retryID != "" {
		task, err = a.Read(retryID)
		if err != nil {
			return task, err
		}
		if task.State == "running" {
			return task, errors.New("识别任务仍在运行")
		}
		task.State, task.Error = "running", ""
		if len(paths) > 0 {
			seen := map[string]bool{}
			for _, path := range task.Paths {
				seen[path] = true
			}
			for _, path := range paths {
				path = a.files.Access.Canonical(path)
				if !seen[path] {
					task.Paths = append(task.Paths, path)
					seen[path] = true
				}
			}
		}
		for i := range task.Items {
			if task.Items[i].State == "failed" {
				task.Items[i].State = "pending"
			}
		}
	}
	remainingPaths := []string{}
	for _, path := range task.Paths {
		_, statErr := a.files.Access.Stat(path)
		completed := false
		for _, item := range task.Items {
			if item.Path == a.files.Access.Canonical(path) && item.Removed {
				completed = true
				break
			}
		}
		if completed && errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		remainingPaths = append(remainingPaths, path)
	}
	task.Paths = remainingPaths
	if len(task.Paths) == 0 || len(task.Paths) > 200 {
		return task, errors.New("请选择 1–200 个文件或目录")
	}
	for _, path := range task.Paths {
		if !a.files.SourceAllowed(path, s) {
			return task, errors.New("只能添加源目录内的文件或文件夹，目标目录不能作为来源")
		}
	}
	if err = a.store.SaveWorkspace(&task); err != nil {
		return task, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.TrackRecognitionLocked(task.ID, cancel)
	var workerTask matching.Workspace
	_ = json.Unmarshal([]byte(jsonText(task)), &workerTask)
	go a.recognize(ctx, cancel, workerTask, s, client)
	return task, nil
}
func (a *WorkspaceService) Cancel(id string) error { return a.coordinator.CancelRecognition(id) }
