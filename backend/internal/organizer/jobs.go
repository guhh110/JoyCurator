package organizer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"joycurator/internal/model"
)

func (a *Service) Execute(id string) (Run, error) {
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	p := a.plans.Get(id)
	if p == nil || time.Now().After(p.Expires) {
		return Run{}, errors.New("预览已过期，请重新预览")
	}
	if len(p.Issues) > 0 || len(p.Ops) == 0 {
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
	r := Run{ID: model.Key(), Title: p.Title, Mode: p.Mode, State: "running", Created: time.Now().Format(time.RFC3339), Ops: append([]model.Work(nil), p.Ops...), Total: len(p.Ops)}
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
	if r.State == "running" || r.State == "queued" || r.State == "undone" {
		return r, errors.New("当前记录不能执行此操作")
	}
	if !undo && (r.State == "completed" || r.Undo) {
		return r, errors.New("此记录无需重试；撤销中断后请继续撤销")
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
			shared, e := a.store.Shared(r.ID, op.Target)
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
	_ = a.store.SaveRun(r)
	if r.RecognitionID != "" && !r.Undo {
		a.finish(r)
	}
}

// JSON records keep artifact ownership with the batch, without another table.
