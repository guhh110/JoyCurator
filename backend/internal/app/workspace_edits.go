package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"joycurator/internal/matching"
	"joycurator/internal/organizer"
)

func (a *WorkspaceService) Update(ctx context.Context, id string, in matching.Update) (matching.Workspace, error) {
	// Network validation happens outside the application mutex. Revision is
	// checked again when committing so two browser tabs cannot lose edits.
	task, err := a.Read(id)
	if err != nil {
		return task, err
	}
	if task.State == "running" {
		return task, errors.New("请等待识别完成后修正")
	}
	if task.Revision != in.Revision {
		return task, errors.New("识别结果已更新，请重新加载")
	}
	if len(in.Items) != len(task.Items) {
		return task, errors.New("文件列表已变化，请重新加载")
	}
	s, err := a.settings.Load()
	if err != nil {
		return task, err
	}
	client, err := a.factory(s, nil)
	if err != nil {
		return task, err
	}
	edits := map[string]matching.Edit{}
	for _, edit := range in.Items {
		if _, ok := edits[edit.Path]; ok {
			return task, errors.New("文件重复")
		}
		if len(edit.Group) > 1024 {
			return task, errors.New("分组名称过长")
		}
		edits[edit.Path] = edit
	}
	memo := matching.NewResolver(client)
	for i := range task.Items {
		item := &task.Items[i]
		edit, ok := edits[item.Path]
		if !ok {
			return task, errors.New("文件列表已变化")
		}
		if !edit.Removed && edit.Selected && !a.files.SourceAllowed(item.Path, s) {
			return task, errors.New("文件已不在当前源目录内，请重新导入")
		}
		if edit.TMDBID < 0 || (edit.TMDBID > 0 && edit.Type != "movie" && edit.Type != "tv") {
			return task, errors.New("作品类型或 ID 无效")
		}
		oldID := 0
		oldType := ""
		if item.Entry != nil {
			oldID, oldType = item.Entry.TMDBID, item.Entry.Type
		}
		if edit.TMDBID != oldID || edit.Type != oldType || edit.Group != item.Group || edit.Season != item.Season || !slices.Equal(edit.Episodes, item.Episodes) || edit.Selected != item.Selected || edit.Removed != item.Removed {
			item.Edited = true
		}
		if edit.TMDBID > 0 {
			changed := item.Entry == nil || item.Entry.TMDBID != edit.TMDBID || item.Entry.Type != edit.Type
			validate := changed || edit.Season != item.Season || !slices.Equal(edit.Episodes, item.Episodes)
			if changed {
				entry, e := memo.Detail(ctx, edit.Type, edit.TMDBID)
				if e != nil {
					return task, e
				}
				item.Entry = entry
				item.Reason = "已手动选择作品"
			}
			item.Season, item.Episodes = edit.Season, edit.Episodes
			if validate {
				memo.Validate(ctx, item)
			}
		} else if item.Entry != nil || edit.Group != item.Group {
			item.Entry = nil
			item.State, item.Reason = "review", "请手动选择作品"
		}
		if edit.Group != "" {
			item.Group = edit.Group
		} else if item.Entry != nil {
			item.Group = matching.EntryGroup(*item.Entry)
		}
		item.Removed, item.Selected = edit.Removed, edit.Selected && item.State == "ready" && !edit.Removed
	}
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	if a.coordinator.BusyLocked() {
		return task, organizer.ErrBusy
	}
	current, err := a.Read(id)
	if err != nil {
		return task, err
	}
	if current.Revision != in.Revision {
		return current, errors.New("识别结果已更新，请重新加载")
	}
	err = a.store.SaveWorkspace(&task)
	return task, err
}
func (a *WorkspaceService) FinishRun(run organizer.Run) {
	completed := map[string]bool{}
	for _, op := range run.Ops {
		if op.Kind == "video" && op.Status == "completed" {
			completed[op.Source] = true
		}
	}
	if len(completed) == 0 {
		return
	}
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	task, err := a.Read(run.RecognitionID)
	if err != nil {
		return
	}
	changed := false
	for i := range task.Items {
		if completed[task.Items[i].Path] && !task.Items[i].Removed {
			task.Items[i].Removed, task.Items[i].Selected = true, false
			changed = true
		}
	}
	if changed {
		_ = a.store.SaveWorkspace(&task)
	}
}

// sourceOwner returns the added path that owns file: the longest added path
// equal to it or containing it. Matches the frontend's source attribution.
func sourceOwner(file string, roots []string) string {
	owner := ""
	for _, root := range roots {
		base := strings.TrimRight(root, "/")
		if base == "" {
			base = "/"
		}
		prefix := base + "/"
		if base == "/" {
			prefix = "/"
		}
		if (file == base || strings.HasPrefix(file, prefix)) && len(base) > len(owner) {
			owner = base
		}
	}
	return owner
}

// RemoveSource drops one added path and the files it owns from the workspace.
// Files on disk are untouched; files owned by another added path stay.
func (a *WorkspaceService) RemoveSource(id, path string, revision int) (matching.Workspace, error) {
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	if a.coordinator.BusyLocked() {
		task, _ := a.Read(id)
		return task, organizer.ErrBusy
	}
	task, err := a.Read(id)
	if err != nil {
		return task, err
	}
	if task.State == "running" {
		return task, errors.New("请等待识别完成后再删除来源")
	}
	if task.Revision != revision {
		return task, errors.New("识别结果已更新，请重新加载")
	}
	target := strings.TrimRight(path, "/")
	if target == "" {
		target = "/"
	}
	index := slices.IndexFunc(task.Paths, func(p string) bool {
		base := strings.TrimRight(p, "/")
		if base == "" {
			base = "/"
		}
		return base == target
	})
	if index < 0 {
		return task, errors.New("来源不在当前识别任务中")
	}
	roots := slices.Clone(task.Paths)
	items := task.Items[:0:0]
	for _, item := range task.Items {
		if sourceOwner(item.Path, roots) != target {
			items = append(items, item)
		}
	}
	task.Paths = slices.Delete(task.Paths, index, index+1)
	task.Items = items
	task.Completed, task.Total = 0, 0
	for _, item := range task.Items {
		task.Total++
		if item.State != "pending" {
			task.Completed++
		}
	}
	err = a.store.SaveWorkspace(&task)
	return task, err
}
