package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

type WorkspaceService struct {
	recognitionFailures sync.Map
	store               WorkspaceStore
	settings            SettingsReader
	factory             matching.Factory
	extract             *recognition.Service
	files               WorkspaceFiles
	coordinator         *organizer.Coordinator
}

func NewWorkspace(store WorkspaceStore, settings SettingsReader, factory matching.Factory, extract *recognition.Service, files WorkspaceFiles, c *organizer.Coordinator) *WorkspaceService {
	return &WorkspaceService{store: store, settings: settings, factory: factory, extract: extract, files: files, coordinator: c}
}
func (a *WorkspaceService) Read(id string) (matching.Workspace, error) {
	task, err := a.store.Workspace(id)
	if err != nil {
		return task, err
	}
	return a.project(task)
}
func (a *WorkspaceService) project(task matching.Workspace) (matching.Workspace, error) {
	if message, ok := a.recognitionFailures.Load(task.ID); ok && task.State == "running" {
		task.State, task.Error = "failed", message.(string)
	}
	states, err := a.store.FileExecutions(task.ID)
	if err != nil {
		return task, err
	}
	for i := range task.Items {
		item := &task.Items[i]
		if item.State == "ready" && !item.Confirmed() {
			item.State, item.Reason, item.Selected = "review", "请确认作品与季集", false
		}
		if state, ok := states[item.Path]; ok {
			copy := state
			item.Execution = &copy
			if state.Locked || state.Completed {
				item.Selected = false
			}
			if state.Completed {
				item.Removed = true
			}
		}
	}
	return task, nil
}
func (a *WorkspaceService) Latest(purpose string) (*matching.Workspace, error) {
	if purpose != "organize" && purpose != "scrape" {
		return nil, errors.New("工作区用途无效")
	}
	task, err := a.store.LatestWorkspace(purpose)
	if err != nil || task == nil {
		return task, err
	}
	value, err := a.project(*task)
	return &value, err
}
func (a *WorkspaceService) Revision(id string) (int, error) {
	w, err := a.store.Workspace(id)
	return w.Revision, err
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func (a *WorkspaceService) Start(paths []string, retryID, purpose string) (matching.Workspace, error) {
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	if a.coordinator.ClosingLocked() {
		return matching.Workspace{}, errors.New("服务正在关闭")
	}
	if a.coordinator.RecognitionActiveLocked() {
		return matching.Workspace{}, errors.New("已有识别任务运行中，请等待完成或取消")
	}
	s, err := a.settings.Load()
	if err != nil {
		return matching.Workspace{}, err
	}

	if purpose != "organize" && purpose != "scrape" {
		return matching.Workspace{}, errors.New("工作区用途无效")
	}
	task := matching.Workspace{Purpose: purpose, ID: model.Key(), State: "running", Paths: paths, Items: []matching.Item{}, Created: time.Now().Format(time.RFC3339)}
	if retryID != "" {
		task, err = a.Read(retryID)
		if err != nil {
			return task, err
		}
		if task.Purpose != purpose {
			return task, errors.New("工作区用途不符")
		}
		if len(paths) > 0 {
			for _, item := range task.Items {
				if item.State == "reidentifying" {
					return task, errors.New("请先重试未完成的重新识别，再添加来源")
				}
			}
		}
		task.State, task.Error = "running", ""
		task.Completed, task.Total = 0, 0
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
		rerunning := hasReidentification(task.Items)
		for i := range task.Items {
			if task.Items[i].State == "failed" && !rerunning {
				task.Items[i].State = "pending"
			}
		}
	}
	if !hasReidentification(task.Items) {
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
			if !a.allowed(task.Purpose, path, s) {
				return task, errors.New("路径不在当前工作区允许的目录内")
			}
		}
	}
	if task.Purpose == "scrape" {
		if len(task.Paths) != 1 {
			return task, errors.New("请选择一个目标目录")
		}
		info, e := a.files.Access.Stat(task.Paths[0])
		if e != nil || !info.IsDir() {
			return task, errors.New("请选择可访问的目标目录")
		}
	}
	if err = a.store.SaveWorkspace(&task); err != nil {
		return task, err
	}
	a.recognitionFailures.Delete(task.ID)
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.TrackRecognitionLocked(task.ID, cancel)
	var workerTask matching.Workspace
	_ = json.Unmarshal([]byte(jsonText(task)), &workerTask)
	go a.recognize(ctx, cancel, workerTask, s)
	return task, nil
}
func (a *WorkspaceService) Cancel(id string) error { return a.coordinator.CancelRecognition(id) }
