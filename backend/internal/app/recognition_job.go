package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

func (a *WorkspaceService) recognize(ctx context.Context, cancel context.CancelFunc, task matching.Workspace, s model.Settings, client matching.Provider) {
	defer cancel()
	defer a.coordinator.FinishRecognition(task.ID)
	if hasReidentification(task.Items) {
		a.recognizeSelected(ctx, task, s, client)
		return
	}
	dirty := []int{}
	fullSave := true
	persist := func() error {
		a.coordinator.Lock()
		defer a.coordinator.Unlock()
		var err error
		if fullSave {
			err = a.store.SaveWorkspace(&task)
		} else {
			err = a.store.CheckpointWorkspace(&task, dirty)
		}
		if err == nil {
			dirty = nil
		}
		return err
	}
	defer func() {
		if task.State == "running" {
			task.State, task.Error = "failed", "无法保存识别进度，请重试"
			_ = persist()
		}
	}()
	// A cancelled scan can be resumed without losing already resolved files.
	previous := map[string]matching.Item{}
	for _, item := range task.Items {
		previous[item.Path] = item
	}
	scanned := []matching.Item{}
	needsExtraction := []string{}
	preserved := map[string]bool{}
	for _, item := range task.Items {
		if item.Execution != nil && (item.Execution.Locked || item.Execution.Completed) {
			scanned = append(scanned, item)
			preserved[item.Path] = true
		}
	}
	reservations, reservationErr := a.store.FileExecutions(task.ID)
	if reservationErr != nil {
		task.State = "failed"
		task.Error = reservationErr.Error()
		_ = persist()
		return
	}
	excluded := []string{}
	if task.Purpose == "organize" {
		excluded = []string{s.TargetRoot}
	}
	scanErr := scanner.Walk(ctx, a.files.Access, task.Paths, excluded, func(path string, info fs.FileInfo) error {
		if preserved[path] {
			return nil
		}
		if reservations[path].Locked && previous[path].Path == "" {
			return nil
		}
		if len(scanned) >= 2000 {
			return errors.New("单次最多导入 2000 个视频，请分批添加")
		}
		if !a.allowed(task.Purpose, path, s) {
			if _, err := a.files.Access.Stat(path); errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return errors.New("扫描文件超出源目录范围")
		}
		if item, ok := previous[path]; ok && item.Identity == fsops.Identity(info) && item.Size == info.Size() && item.MTime == info.ModTime().UnixNano() && (item.State != "pending" || item.Edited) {
			scanned = append(scanned, item)
			return nil
		}
		if task.Purpose == "scrape" {
			item, e := a.reusedTarget(ctx, path, info, client)
			if e != nil {
				return e
			}
			if item != nil {
				scanned = append(scanned, *item)
				return nil
			}
		}
		needsExtraction = append(needsExtraction, path)
		scanned = append(scanned, matching.Item{File: model.File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano()}})
		return nil
	})
	if scanErr == nil && len(needsExtraction) > 0 {
		var fresh map[string]matching.Item
		fresh, scanErr = a.extractContexts(ctx, task.Paths, needsExtraction, s, task.ID, task.Purpose)
		if scanErr == nil {
			for i, item := range scanned {
				if next, ok := fresh[item.Path]; ok {
					if old, ok := previous[item.Path]; ok && old.Edited {
						next.Entry, next.Season, next.Episodes, next.Group, next.Edited = old.Entry, old.Season, old.Episodes, old.Group, true
						next.Confirmation = nil
						next.State, next.Reason, next.Selected = "review", "文件已变化，请重新确认", false
					}
					scanned[i] = next
				}
			}
		}
	}
	if scanErr != nil {
		task.State, task.Error = "failed", scanErr.Error()
		if ctx.Err() != nil {
			task.State, task.Error = "cancelled", "识别已取消，可重试"
			if a.coordinator.IsClosing() {
				task.State, task.Error = "interrupted", "服务中断，可重试未完成项"
			}
		}
		_ = persist()
		return
	}
	sort.Slice(scanned, func(i, j int) bool { return scanned[i].Path < scanned[j].Path })
	if task.Purpose == "scrape" {
		for i := range scanned {
			it := &scanned[i]
			info, e := a.files.Access.Stat(it.Path)
			if e != nil {
				task.State, task.Error = "failed", e.Error()
				_ = persist()
				return
			}
			it.Identity = fsops.Identity(info)
			if it.Original == nil {
				it.Original, e = a.store.Mapping(it.Path)
				if e != nil {
					task.State, task.Error = "failed", e.Error()
					_ = persist()
					return
				}
			}
		}
	}
	task.Items, task.Total, task.Completed = scanned, len(scanned), 0
	if err := persist(); err != nil {
		return
	}
	fullSave = false
	memo := matching.NewResolver(client)
	resolved := map[string]matching.Item{}
	for i := range task.Items {
		if ctx.Err() != nil {
			break
		}
		item := &task.Items[i]
		if item.State == "pending" {
			if item.Entry == nil {
				group := item.Group
				match, ok := resolved[group]
				if !ok {
					match = *item
					memo.Suggest(ctx, &match)
					resolved[group] = match
				}
				item.Entry, item.Candidates, item.State, item.Reason = match.Entry, match.Candidates, match.State, match.Reason
				if item.Entry != nil {
					item.Group = matching.EntryGroup(*item.Entry)
				}
			}
			if item.Entry != nil {
				memo.Validate(ctx, item)
				if !item.Confirmed() {
					item.State, item.Reason = "review", "请确认作品与季集"
				}
			}
			if !item.Edited {
				item.Selected = item.State == "ready" && !item.Removed
			} else if item.State != "ready" || item.Removed {
				item.Selected = false
			}
		}
		dirty = append(dirty, i)
		task.Completed++
		// Bound write amplification for large groups while still exposing progress.
		if i%10 == 0 || i+1 == len(task.Items) {
			if err := persist(); err != nil {
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
	task.State = "completed"
	if ctx.Err() != nil {
		task.State, task.Error = "cancelled", "识别已取消，可重试未完成项"
		if a.coordinator.IsClosing() {
			task.State, task.Error = "interrupted", "服务中断，可重试未完成项"
		}
	}
	_ = persist()
}

func itemFromMetadata(path string, info fs.FileInfo, p recognition.Metadata, traceID string) matching.Item {
	title, mediaType, year, tmdbID := "", "", 0, 0
	if p.Title != nil {
		title = *p.Title
	}
	if p.MediaType != nil {
		mediaType = *p.MediaType
	}
	if p.Year != nil {
		year = *p.Year
	}
	if p.TMDBID != nil {
		tmdbID = *p.TMDBID
	}
	season := -1
	if p.Season != nil {
		season = *p.Season
	}
	group := fmt.Sprintf("query:%s:%s:%d:%d", mediaType, recognition.NormalizeTitle(title), year, tmdbID)
	if title == "" && tmdbID == 0 {
		group = "path:" + path
	}
	return matching.Item{Identity: fsops.Identity(info), File: model.File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano(), Season: season, Episodes: p.Episodes}, Metadata: p, Group: group, Candidates: []model.Entry{}, State: "pending", TraceID: traceID}
}
