package organizer

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

func (a *Service) Execute(id string) (Run, error) {
	if runtime.GOOS == "windows" {
		return Run{}, errors.New("Windows 暂不支持执行文件整理")
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	p := a.plans.Get(id)
	if p == nil || time.Now().After(p.Expires) {
		return Run{}, errors.New("预览已过期，请重新预览")
	}
	if len(p.Issues) > 0 || len(p.Mappings) == 0 || p.Purpose == "scrape" {
		return Run{}, errors.New("请先处理预览中的冲突")
	}
	if len(p.Groups) > 0 {
		return Run{}, errors.New("请通过批量整理接口执行此预览")
	}
	s, err := a.settings.Load()
	if err != nil {
		return Run{}, err
	}
	if s.Version != p.Version {
		return Run{}, errors.New("设置已更新，请重新预览")
	}
	if err = a.plans.ValidateOps(p.Ops); err != nil {
		return Run{}, err
	}
	if err = a.validateSkipped(p.Mappings, p.Ops); err != nil {
		return Run{}, err
	}
	r := Run{ID: model.Key(), Title: p.Title, Mode: p.Mode, State: "running", Created: time.Now().Format(time.RFC3339), Ops: append([]model.Work(nil), p.Ops...), Total: len(p.Ops), Purpose: p.Purpose, Mappings: p.Mappings}
	result, err := a.launch(r)
	if err == nil {
		a.plans.Remove(id)
	}
	return result, err
}
func (a *Service) Resume(id string, undo bool) (Run, error) {
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	r, err := a.store.Run(id)
	if err != nil {
		return r, err
	}
	if runtime.GOOS == "windows" {
		return r, errors.New("Windows 暂不支持执行或撤销文件操作")
	}
	if r.State == "running" || r.State == "queued" || r.State == "undone" {
		return r, errors.New("当前记录不能执行此操作")
	}
	if !undo && (r.State == "completed" || r.Undo) {
		return r, errors.New("此记录无需重试；撤销中断后请继续撤销")
	}
	if !undo && r.Purpose == "scrape" {
		settings, e := a.settings.Load()
		if e != nil {
			return r, e
		}
		if settings.Version != r.Version {
			return r, errors.New("设置已变化，请重新预览")
		}
		currentMappings, e := a.mappings.Mappings()
		if e != nil {
			return r, e
		}
		currentByPath := make(map[string]model.MediaMapping, len(currentMappings))
		for _, item := range currentMappings {
			currentByPath[item.Path] = item
		}
		for _, item := range r.Mappings {
			current, ok := currentByPath[item.Path]
			if !ok || current.RunID != item.RunID || current.Identity != item.Identity || current.TMDBID != item.TMDBID {
				return r, errors.New("刮削映射已变化，不能重试")
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
	r.State = "running"
	r.Error = ""
	r.Undo = undo
	r.Completed = 0
	r.Total = len(r.Ops)
	return a.launch(r)
}

// launch is called under mu. The worker owns its Run value; readers use SQLite
// snapshots, never the worker's counters or operation pointers.
func (a *Service) launch(r Run) (Run, error) {
	if a.coordinator.closed {
		return Run{}, errors.New("服务正在关闭")
	}
	if a.coordinator.running != "" || a.coordinator.batch != "" {
		return Run{}, ErrBusy
	}
	if err := a.store.SaveRun(r); err != nil {
		return Run{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.running = r.ID
	a.coordinator.cancel = cancel
	a.coordinator.wg.Add(1)
	go a.work(ctx, cancel, r)
	summary := r
	summary.Ops = nil
	return summary, nil
}
func (a *Service) work(ctx context.Context, cancel context.CancelFunc, r Run) {
	defer a.coordinator.wg.Done()
	defer cancel()
	defer func() {
		a.coordinator.mu.Lock()
		a.coordinator.running = ""
		a.coordinator.cancel = nil
		a.coordinator.mu.Unlock()
	}()
	s, err := a.settings.Load()
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		_ = a.store.SaveRun(r)
		return
	}
	client, err := a.factory(s, nil)
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		_ = a.store.SaveRun(r)
		return
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
		if !r.Undo && blocked[op.Owner] {
			op.Error = "视频未完成，本项待重试"
			if err = a.store.SaveRun(r); err != nil {
				failures = append(failures, err.Error())
				break
			}
			continue
		}
		op.Error = ""
		save := func() error { return a.store.SaveRun(r) }
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
			failures = append(failures, filepath.Base(op.Target)+"："+err.Error())
			if op.Kind == "video" {
				blocked[op.Owner] = true
			}
		} else {
			r.Completed++
		}
		if err = a.store.SaveRun(r); err != nil {
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
	if r.Purpose == "organize" && !r.Undo {
		for index := range r.Mappings {
			mapping := r.Mappings[index]
			mapping.RunID = r.ID
			current, lookupErr := a.store.Mapping(mapping.Path)
			if lookupErr != nil {
				r.State, r.Error = "failed", "刮削待办读取失败："+lookupErr.Error()
				continue
			}
			if current != nil && current.RunID == r.ID {
				r.Mappings[index] = *current
				continue
			}
			if mapping.Identity != "" {
				info, statErr := a.access.Stat(mapping.Path)
				if statErr != nil || !info.Mode().IsRegular() || fsops.Identity(info) != mapping.Identity || info.Size() != mapping.Size || info.ModTime().UnixNano() != mapping.MTime {
					r.State, r.Error = "failed", "跳过的视频已变化，无法登记刮削待办："+mapping.Path
					_ = a.store.SaveRun(r)
					continue
				}
				if e := a.store.SaveMapping(mapping); e != nil {
					r.State, r.Error = "failed", "刮削待办保存失败："+e.Error()
					_ = a.store.SaveRun(r)
				} else {
					r.Mappings[index] = mapping
				}
				continue
			}
			for _, op := range r.Ops {
				if op.Kind == "video" && op.Target == mapping.Path && op.Status == "completed" {
					info, statErr := a.access.Stat(mapping.Path)
					if statErr != nil || !info.Mode().IsRegular() || fsops.Identity(info) != op.Identity {
						r.State, r.Error = "failed", "目标视频已变化，无法登记刮削待办："+mapping.Path
						break
					}
					mapping.Identity, mapping.Size, mapping.MTime = op.Identity, info.Size(), info.ModTime().UnixNano()
					mapping.State = "pending"
					if e := a.store.SaveMapping(mapping); e != nil {
						r.State, r.Error = "failed", "刮削待办保存失败："+e.Error()
						_ = a.store.SaveRun(r)
					} else {
						r.Mappings[index] = mapping
					}
				}
			}
		}
	}
	if r.Purpose == "organize" && r.Undo && r.State == "undone" {
		for _, mapping := range r.Mappings {
			mapping.RunID = r.ID
			if e := a.store.DeleteMapping(mapping); e != nil {
				r.State, r.Error = "failed", "撤销登记失败："+e.Error()
				_ = a.store.SaveRun(r)
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
				_ = a.store.SaveRun(r)
			}
		}
	}
	if r.Purpose == "organize" && r.RecognitionID != "" && !r.Undo {
		a.finish(r)
	}
	_ = a.store.SaveRun(r)
}

// JSON records keep artifact ownership with the batch, without another table.
