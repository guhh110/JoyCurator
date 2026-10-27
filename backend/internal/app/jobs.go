package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"joycurator/internal/organizer"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) execute(id string) (Run, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.plans[id]
	if p == nil || time.Now().After(p.Expires) {
		return Run{}, errors.New("预览已过期，请重新预览")
	}
	if len(p.Issues) > 0 || len(p.Ops) == 0 {
		return Run{}, errors.New("请先处理预览中的冲突")
	}
	s, err := a.settings()
	if err != nil {
		return Run{}, err
	}
	if s.Version != p.Version {
		return Run{}, errors.New("设置已更新，请重新预览")
	}
	if err = a.validateOps(p.Ops); err != nil {
		return Run{}, err
	}
	r := Run{ID: key(), Title: p.Title, Mode: p.Mode, State: "running", Created: time.Now().Format(time.RFC3339), Ops: append([]Work(nil), p.Ops...), Total: len(p.Ops)}
	result, err := a.launch(r)
	if err == nil {
		delete(a.plans, id)
	}
	return result, err
}
func (a *App) resume(id string, undo bool) (Run, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, err := a.record(id)
	if err != nil {
		return r, err
	}
	if r.State == "running" || r.State == "undone" {
		return r, errors.New("当前记录不能执行此操作")
	}
	if !undo && (r.State == "completed" || r.Undo) {
		return r, errors.New("此记录无需重试；撤销中断后请继续撤销")
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
func (a *App) launch(r Run) (Run, error) {
	if a.closed {
		return Run{}, errors.New("服务正在关闭")
	}
	if a.running != "" {
		return Run{}, errBusy
	}
	if err := a.save(r); err != nil {
		return Run{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.running = r.ID
	a.cancel = cancel
	a.wg.Add(1)
	go a.work(ctx, cancel, r)
	summary := r
	summary.Ops = nil
	return summary, nil
}
func (a *App) work(ctx context.Context, cancel context.CancelFunc, r Run) {
	defer a.wg.Done()
	defer cancel()
	defer func() { a.mu.Lock(); a.running = ""; a.cancel = nil; a.mu.Unlock() }()
	s, err := a.settings()
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		_ = a.save(r)
		return
	}
	client, err := a.client(s)
	if err != nil {
		r.State = "failed"
		r.Error = err.Error()
		_ = a.save(r)
		return
	}
	engine := organizer.Engine{FS: a.access, Image: client.Image}
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
			if err = a.save(r); err != nil {
				failures = append(failures, err.Error())
				break
			}
			continue
		}
		op.Error = ""
		save := func() error { return a.save(r) }
		if r.Undo {
			shared, e := a.shared(r.ID, op.Target)
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
		if err = a.save(r); err != nil {
			failures = append(failures, "无法保存记录："+err.Error())
			break
		}
	}
	switch {
	case ctx.Err() != nil:
		r.State = "cancelled"
		r.Error = "操作已取消，可重试或撤销"
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
	_ = a.save(r)
}

// JSON records keep artifact ownership with the batch, without another table.
func (a *App) owned(target string) (*Work, error) {
	var raw string
	err := a.db.QueryRow(`SELECT op.value FROM runs, json_each(runs.value,'$.ops') op
 WHERE json_extract(runs.value,'$.state')<>'undone' AND json_extract(op.value,'$.target')=?
 AND json_extract(op.value,'$.status') IN ('completed','published') LIMIT 1`, target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var work Work
	err = json.Unmarshal([]byte(raw), &work)
	return &work, err
}
func (a *App) shared(id, target string) (bool, error) {
	var n int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM runs, json_each(runs.value,'$.ops') op
 WHERE runs.id<>? AND json_extract(runs.value,'$.state')<>'undone' AND json_extract(op.value,'$.target')=?
 AND json_extract(op.value,'$.status') IN ('completed','published')`, id, target).Scan(&n)
	return n > 0, err
}
