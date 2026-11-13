package app

import (
	"context"
	"encoding/json"
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
)

func (a *WorkspaceService) Reidentify(id string, revision int, paths []string) (matching.Workspace, error) {
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	var empty matching.Workspace
	if a.coordinator.ClosingLocked() {
		return empty, errors.New("服务正在关闭")
	}
	if a.coordinator.RecognitionActiveLocked() {
		return empty, errors.New("请等待当前识别完成或取消")
	}
	if len(paths) == 0 || len(paths) > 2000 {
		return empty, errors.New("请选择 1–2000 个工作区文件")
	}
	task, err := a.Read(id)
	if err != nil {
		return task, err
	}
	if task.State == "running" || task.Revision != revision {
		return task, errors.New("工作区已更新，请刷新后重试")
	}
	s, err := a.settings.Load()
	if err != nil {
		return task, err
	}
	wanted := map[string]bool{}
	for _, path := range paths {
		if wanted[path] {
			return task, errors.New("文件重复")
		}
		wanted[path] = true
	}
	for _, item := range task.Items {
		if item.State == "reidentifying" {
			return task, errors.New("存在未完成的重新识别，请先重试")
		}
	}
	positions := []int{}
	for i, item := range task.Items {
		if !wanted[item.Path] {
			continue
		}
		if item.Removed || item.Execution != nil && (item.Execution.Locked || item.Execution.Completed) {
			return task, errors.New("已移出、已提交或整理完成的文件不能重新识别：" + item.Path)
		}
		if !a.allowed(task.Purpose, item.Path, s) {
			return task, errors.New("文件已不在当前源目录内：" + item.Path)
		}
		info, err := a.files.Access.Stat(item.Path)
		if err != nil || !info.Mode().IsRegular() {
			return task, errors.New("来源文件不可用：" + item.Path)
		}
		positions = append(positions, i)
	}
	if len(positions) != len(paths) {
		return task, errors.New("文件不在当前工作区")
	}
	for _, i := range positions {
		task.Items[i].State = "reidentifying"
	}
	task.State, task.Error = "running", ""
	task.Completed, task.Total = 0, len(positions)
	data, err := json.Marshal(task)
	if err != nil {
		return task, err
	}
	var workerTask matching.Workspace
	if err = json.Unmarshal(data, &workerTask); err != nil {
		return task, err
	}
	if err := a.store.CheckpointWorkspace(&task, positions); err != nil {
		return task, err
	}
	a.recognitionFailures.Delete(task.ID)
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.TrackRecognitionLocked(id, cancel)
	workerTask.Revision = task.Revision
	go a.recognize(ctx, cancel, workerTask, s)
	return task, nil
}

// Only marked rows are read and checkpointed. The marker survives cancellation
// and service interruption, so the ordinary retry entry can finish the same work.
func (a *WorkspaceService) recognizeSelected(ctx context.Context, task matching.Workspace, s model.Settings) {
	dirty := []int{}
	persist := func() error {
		a.coordinator.Lock()
		defer a.coordinator.Unlock()
		if err := a.store.CheckpointWorkspace(&task, dirty); err != nil {
			a.recognitionFailures.Store(task.ID, err.Error())
			return err
		}
		a.recognitionFailures.Delete(task.ID)
		dirty = nil
		return nil
	}
	failure := func(err error) {
		task.State, task.Error = "failed", err.Error()
		if ctx.Err() != nil {
			task.State, task.Error = "cancelled", "重新识别已取消，可重试未完成项"
			if a.coordinator.IsClosing() {
				task.State, task.Error = "interrupted", "服务中断，可重试未完成项"
			}
		}
		_ = persist()
	}
	wanted := []string{}
	for _, item := range task.Items {
		if item.State == "reidentifying" {
			wanted = append(wanted, item.Path)
		}
	}
	task.Completed, task.Total = 0, len(wanted)
	freshItems, extractErr := a.extractContexts(ctx, task.Paths, wanted, s, task.ID, task.Purpose, nil, func(done, total int) error {
		if done%10 == 0 || done == total {
			return a.store.RecognitionProgress(task.ID, done, total)
		}
		return nil
	})
	if extractErr != nil {
		failure(extractErr)
		return
	}
	processed := 0
	refreshed := map[int]bool{}
	for i, old := range task.Items {
		if old.State != "reidentifying" {
			continue
		}
		if err := ctx.Err(); err != nil {
			failure(err)
			return
		}
		if !a.allowed(task.Purpose, old.Path, s) {
			failure(errors.New("文件已不在当前源目录内：" + old.Path))
			return
		}
		info, err := a.files.Access.Stat(old.Path)
		if err != nil || !info.Mode().IsRegular() {
			failure(errors.New("来源文件不可用：" + old.Path))
			return
		}
		fresh := freshItems[old.Path]
		fresh.Original = old.Original
		fresh.Identity, fresh.Size, fresh.MTime = fsops.Identity(info), info.Size(), info.ModTime().UnixNano()
		if task.Purpose == "scrape" {
			if old.Layout.Manual {
				fresh.Layout = old.Layout
			}
		}
		// Reidentification refreshes evidence only; manual drafts and selection survive.
		fresh.Entry, fresh.Group = old.Entry, old.Group
		fresh.Season, fresh.Episodes = old.Season, old.Episodes
		fresh.Edited, fresh.Selected, fresh.Removed = old.Edited, old.Selected, old.Removed
		fresh.State, fresh.Reason = "review", ""
		if fresh.Identity == old.Identity && fresh.Size == old.Size && fresh.MTime == old.MTime {
			fresh.Confirmation = old.Confirmation
			if fresh.Confirmed() {
				fresh.State = "ready"
			}
		} else {
			fresh.Selected = false
			fresh.Reason = "文件已变化，请重新确认"
		}
		task.Completed++
		task.Items[i] = fresh
		refreshed[i] = true
		dirty = append(dirty, i)
		processed++
		if processed%10 == 0 {
			if err := persist(); err != nil {
				failure(err)
				return
			}
		}
	}
	if task.Purpose == "scrape" {
		layouts := append([]matching.Item(nil), task.Items...)
		matching.InferLayouts(layouts, s.TargetRoot)
		for i := range task.Items {
			if refreshed[i] {
				task.Items[i].Layout = layouts[i].Layout
				dirty = append(dirty, i)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		failure(err)
		return
	}
	task.State, task.Error = "completed", ""
	if err := persist(); err != nil {
		failure(err)
	}
}

func hasReidentification(items []matching.Item) bool {
	for _, item := range items {
		if item.State == "reidentifying" {
			return true
		}
	}
	return false
}
