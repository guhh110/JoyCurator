package app

import (
	"context"
	"encoding/json"
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"strings"
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
	if s.Token == "" {
		return task, errors.New("请先配置 TMDB API Read Access Token")
	}
	client, err := a.factory(s, nil)
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
	task.Completed, task.Total = len(task.Items)-len(positions), len(task.Items)
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
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.TrackRecognitionLocked(id, cancel)
	workerTask.Revision = task.Revision
	go a.recognize(ctx, cancel, workerTask, s, client)
	return task, nil
}

// Only marked rows are read and checkpointed. The marker survives cancellation
// and service interruption, so the ordinary retry entry can finish the same work.
func (a *WorkspaceService) recognizeSelected(ctx context.Context, task matching.Workspace, s model.Settings, client matching.Provider) {
	dirty := []int{}
	persist := func() error {
		a.coordinator.Lock()
		defer a.coordinator.Unlock()
		if err := a.store.CheckpointWorkspace(&task, dirty); err != nil {
			return err
		}
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
	freshItems, extractErr := a.extractContexts(ctx, task.Paths, wanted, s, task.ID, task.Purpose)
	if extractErr != nil {
		failure(extractErr)
		return
	}
	memo := matching.NewResolver(client)
	resolved := map[string]matching.Item{}
	processed := 0
	failures := []string{}
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
		if task.Purpose == "scrape" {
			fresh.Identity = fsops.Identity(info)
			if old.Layout.Manual {
				fresh.Layout = old.Layout
			}
		}
		if old.Edited || old.Confirmed() {
			// Refresh raw metadata/trace while keeping every manual decision and snapshot.
			fresh.Entry, fresh.Candidates, fresh.Group = old.Entry, old.Candidates, old.Group
			fresh.Season, fresh.Episodes = old.Season, old.Episodes
			fresh.Edited, fresh.Selected, fresh.Removed = true, old.Selected, old.Removed
			if fresh.Identity == old.Identity && fresh.Size == old.Size && fresh.MTime == old.MTime {
				fresh.Confirmation = old.Confirmation
			}
			fresh.State, fresh.Reason = "review", "已保留人工修正，请确认作品"
			if fresh.Entry != nil {
				if fresh.Entry.Type == "tv" && (old.Season < 0 || len(old.Episodes) == 0) {
					fresh.Reason = "已保留人工修正，季集仍需确认"
				} else {
					memo.Validate(ctx, &fresh)
				}
			}
			if !fresh.Confirmed() && fresh.State != "failed" {
				fresh.State, fresh.Reason = "review", "请确认作品与季集"
			}
			if fresh.State != "ready" {
				fresh.Selected = false
			}
		} else {
			match, ok := resolved[fresh.Group]
			if !ok {
				match = fresh
				memo.Suggest(ctx, &match)
				resolved[fresh.Group] = match
			}
			fresh.Entry, fresh.Candidates, fresh.State, fresh.Reason = match.Entry, match.Candidates, match.State, match.Reason
			if fresh.Entry != nil {
				fresh.Group = matching.EntryGroup(*fresh.Entry)
				memo.Validate(ctx, &fresh)
			}
			fresh.Selected = fresh.State == "ready"
		}
		// If cancellation interrupted matching, leave this row marked for retry.
		if err := ctx.Err(); err != nil {
			failure(err)
			return
		}
		if fresh.State == "failed" {
			failures = append(failures, fresh.Reason)
			fresh.State = "reidentifying"
		} else {
			task.Completed++
		}
		task.Items[i] = fresh
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
		matching.InferLayouts(task.Items, s.TargetRoot)
		for i := range task.Items {
			dirty = append(dirty, i)
		}
	}
	task.State, task.Error = "completed", ""
	if len(failures) > 0 {
		task.State, task.Error = "failed", strings.Join(failures, "\n")
	}
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
