package organizer

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"joycurator/internal/infra/fsops"
)

func (a *Service) Resume(id string, undo bool) (Run, error) {
	r, err := a.store.Run(id)
	if err != nil {
		return r, err
	}
	original := r
	if runtime.GOOS == "windows" {
		return r, errors.New("Windows 暂不支持执行或撤销文件操作")
	}
	if r.State == "running" || r.State == "queued" || r.State == "undone" {
		return r, errors.New("当前记录不能执行此操作")
	}
	if !undo && (r.State == "completed" || r.Undo) {
		return r, errors.New("此记录无需重试；撤销中断后请继续撤销")
	}
	if r.Purpose == "organize" {
		s, settingsErr := a.settings.Load()
		if settingsErr != nil {
			return r, settingsErr
		}
		for _, op := range r.Ops {
			if undo && (op.Status == "preparing" || op.Status == "staged" || op.Status == "published" || op.Status == "cleanup" || op.Status == "undoing" || op.Status == "manual_review") {
				return r, errors.New("存在状态不明的整理操作，请先人工核对，不能自动撤销")
			}
			if !withinMedia(s.TargetRoot, op.Target) || (op.Source != "" && !withinMedia(s.SourceRoot, op.Source)) {
				return r, errors.New("整理路径已超出当前媒体目录")
			}
			if !undo && op.Source != "" && op.Status == "pending" && !a.sources.SourceAllowed(op.Source, s) {
				return r, errors.New("来源已变化，请重新添加")
			}
		}
	}
	if !undo && r.Purpose == "scrape" {
		settings, e := a.settings.Load()
		if e != nil {
			return r, e
		}
		if settings.Version != r.Version {
			return r, errors.New("设置已变化，请重新预览")
		}
		for _, item := range r.Mappings {
			if err := a.validateScrapeMapping(item, r.ID); err != nil {
				return r, err
			}
			info, statErr := a.access.Stat(item.Path)
			if !withinMedia(settings.TargetRoot, item.Path) || statErr != nil || !info.Mode().IsRegular() || fsops.Identity(info) != item.Identity || info.Size() != item.Size || info.ModTime().UnixNano() != item.MTime {
				return r, errors.New("目标视频已变化，不能重试刮削：" + item.Path)
			}
		}
	}
	if undo && r.Purpose == "scrape" {
		settings, e := a.settings.Load()
		if e != nil {
			return r, e
		}
		for _, op := range r.Ops {
			if !withinMedia(settings.TargetRoot, op.Target) {
				return r, errors.New("刮削目标已超出当前媒体目录，不能撤销")
			}
		}
	}
	if !undo && r.BatchID != "" {
		s, e := a.settings.Load()
		if e != nil {
			return r, e
		}
		if s.Version != r.Version {
			return r, errors.New("设置已变化，请重新预览")
		}
		for _, op := range r.Ops {
			if op.Source != "" && op.Status != "completed" && op.Status != "published" && !a.sources.SourceAllowed(op.Source, s) {
				return r, errors.New("来源已超出当前源目录，请重新添加")
			}
		}
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	current, err := a.store.Run(id)
	if err != nil {
		return Run{}, err
	}
	if !reflect.DeepEqual(original, current) {
		return Run{}, errors.New("任务状态已更新，请刷新记录")
	}
	if undo {
		settings, e := a.settings.Load()
		if e != nil {
			return Run{}, e
		}
		r.Version = settings.Version
	}
	r.State = "queued"
	r.Error = ""
	r.Undo = undo
	r.Completed = 0
	r.Total = len(r.Ops)
	if err = a.enqueueLocked([]Run{r}); err != nil {
		return Run{}, err
	}
	return summary(r), nil
}

func (a *Service) work(ctx context.Context, r Run) Run {
	s, err := a.settings.Load()
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		return r
	}
	client, err := a.factory(s, nil)
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		return r
	}
	if err = a.persistProgress(&r); err != nil {
		r.State = "failed"
		r.Error = err.Error()
		return r
	}
	engine := a.runner(client.Image)
	failures := []string{}
	blocked := map[string]bool{}
	for step := range r.Ops {
		if ctx.Err() != nil {
			break
		}
		i := step
		if r.Undo {
			i = len(r.Ops) - 1 - step
		}
		op := &r.Ops[i]
		if r.Purpose == "organize" && (op.Kind == "video" || op.Kind == "subtitle") &&
			(op.Status == "preparing" || op.Status == "staged" || op.Status == "published" || op.Status == "cleanup" || op.Status == "undoing" || op.Status == "manual_review") {
			op.Status = "manual_review"
			op.Error = errManualReview.Error()
			failures = append(failures, filepath.Base(op.Target)+"："+op.Error)
			if op.Kind == "video" {
				blocked[op.Owner] = true
			}
			if saveErr := a.store.SaveCheckpoint(r, []int{i}, nil, nil); saveErr != nil {
				failures = append(failures, saveErr.Error())
				break
			}
			continue
		}
		if !r.Undo && blocked[op.Owner] {
			op.Error = "视频未完成，本项待重试"
			if err = a.persistProgress(&r, i); err != nil {
				failures = append(failures, err.Error())
				break
			}
			continue
		}
		op.Error = ""
		save := func() error { return a.persistProgress(&r, i) }
		if r.Undo {
			shared, e := a.store.Shared(r.ID, op.Target, op.Identity)
			if e != nil {
				err = e
			} else if shared && (op.Kind == "nfo" || op.Kind == "image") {
				op.Status = "retained"
				err = save()
			} else {
				err = engine.Undo(ctx, &op.Operation, save)
			}
		} else {
			err = engine.Run(ctx, &op.Operation, save)
		}
		if err != nil {
			op.Error = err.Error()
			if r.Purpose == "organize" && (op.Kind == "video" || op.Kind == "subtitle") && op.Status != "pending" && op.Status != "completed" {
				op.Status = "manual_review"
			}
			failures = append(failures, filepath.Base(op.Target)+"："+err.Error())
			if op.Kind == "video" {
				blocked[op.Owner] = true
			}
		} else {
			r.Completed++
		}
		if err = a.persistProgress(&r, i); err != nil {
			failures = append(failures, "无法保存记录："+err.Error())
			break
		}
	}
	switch {
	case ctx.Err() != nil:
		r.State = "cancelled"
		r.Error = "操作已取消，可重试或撤销"
		if a.coordinator.IsClosing() {
			r.State, r.Error = "interrupted", "服务中断，可继续或撤销"
		}
	case len(failures) > 0:
		r.State = "failed"
		r.Error = strings.Join(failures, "\n")
	case r.Undo:
		r.State = "undone"
		r.Error = ""
	default:
		r.State = "completed"
		r.Error = ""
	}
	if r.Purpose == "organize" && r.Undo && r.State == "undone" {
		for _, mapping := range r.Mappings {
			mapping.RunID = r.ID
			if e := a.store.DeleteMapping(mapping); e != nil {
				r.State, r.Error = "failed", "撤销登记失败："+e.Error()
			}
		}
	}
	if r.Purpose == "scrape" {
		for _, mapping := range r.Mappings {
			state := "completed"
			if r.Undo && r.State == "undone" {
				state = "pending"
				for _, op := range r.Ops {
					if (op.Owner == mapping.Path || slices.Contains(op.Owners, mapping.Path)) && op.Status == "retained" {
						state = "completed"
						break
					}
				}
			} else if r.Undo || r.State != "completed" {
				state = "failed"
			} else {
				found := false
				for _, op := range r.Ops {
					if op.Owner != mapping.Path && !slices.Contains(op.Owners, mapping.Path) {
						continue
					}
					found = true
					if op.Status != "completed" {
						state = "failed"
						break
					}
				}
				if !found {
					state = "failed"
				}
			}
			if e := a.store.UpdateMappingState(mapping, state, r.ID); e != nil {
				r.State, r.Error = "failed", "刮削待办保存失败："+e.Error()
			}
		}
	}
	return r
}
