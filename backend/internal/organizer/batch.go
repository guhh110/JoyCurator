package organizer

import (
	"context"
	"errors"
	"runtime"
	"time"

	"joycurator/internal/model"
)

func (a *Service) ExecuteBatch(id string) (Batch, error) {
	if runtime.GOOS == "windows" {
		return Batch{}, errors.New("Windows 暂不支持执行文件整理")
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.coordinator.closed {
		return Batch{}, errors.New("服务正在关闭")
	}
	if a.coordinator.running != "" || a.coordinator.batch != "" {
		return Batch{}, ErrBusy
	}
	p := a.plans.Get(id)
	if p == nil || time.Now().After(p.Expires) {
		return Batch{}, errors.New("预览已过期，请重新预览")
	}
	if len(p.Groups) == 0 || len(p.Mappings) == 0 || len(p.Issues) > 0 {
		return Batch{}, errors.New("请先处理预览中的冲突")
	}
	revision, err := a.revision(p.RecognitionID)
	if err != nil {
		return Batch{}, err
	}
	if revision != p.Revision {
		return Batch{}, errors.New("识别结果已修改，请重新预览")
	}
	s, err := a.settings.Load()
	if err != nil {
		return Batch{}, err
	}
	if s.Version != p.Version {
		return Batch{}, errors.New("设置已更新，请重新预览")
	}
	if err = a.plans.ValidateOps(p.Ops); err != nil {
		return Batch{}, err
	}
	if err = a.validateSkipped(p.Mappings, p.Ops); err != nil {
		return Batch{}, err
	}
	batch := Batch{ID: model.Key(), Runs: []Run{}, Active: true}
	for _, group := range p.Groups {
		if len(group.Mappings) == 0 {
			continue
		}
		run := Run{ID: model.Key(), Title: group.Title, Mode: p.Mode, State: "queued", Created: time.Now().Format(time.RFC3339), Ops: append([]model.Work(nil), group.Ops...), Total: len(group.Ops), BatchID: batch.ID, Version: p.Version, RecognitionID: p.RecognitionID, Purpose: "organize", Mappings: group.Mappings}

		batch.Runs = append(batch.Runs, run)
	}
	if err = a.store.SaveBatch(batch.Runs); err != nil {
		return Batch{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.coordinator.batch, a.coordinator.batchCancel = batch.ID, cancel
	a.coordinator.wg.Add(1)
	go a.runBatch(ctx, cancel, batch)
	a.plans.Remove(id)
	batch.Runs = append([]Run(nil), batch.Runs...)
	for i := range batch.Runs {
		batch.Runs[i].Ops = nil
	}
	return batch, nil
}
func (a *Service) runBatch(ctx context.Context, cancel context.CancelFunc, batch Batch) {
	defer a.coordinator.wg.Done()
	defer cancel()
	defer func() {
		a.coordinator.mu.Lock()
		a.coordinator.batch = ""
		a.coordinator.batchCancel = nil
		a.coordinator.mu.Unlock()
	}()
	for _, run := range batch.Runs {
		if ctx.Err() != nil {
			run.State, run.Error = "cancelled", "整批操作已取消，可独立重试"
			if a.coordinator.IsClosing() {
				run.State, run.Error = "interrupted", "服务中断，可独立重试或撤销"
			}
			_ = a.store.SaveRun(run)
			continue
		}
		s, err := a.settings.Load()
		if err == nil && s.Version != run.Version {
			err = errors.New("设置已变化，请重新预览")
		}
		if err == nil {
			for _, op := range run.Ops {
				if op.Source != "" && !a.sources.SourceAllowed(op.Source, s) {
					err = errors.New("来源已超出当前源目录")
					break
				}
			}
		}
		if err == nil {
			err = a.plans.ValidateOps(run.Ops)
		}
		if err == nil {
			err = a.validateSkipped(run.Mappings, run.Ops)
		}
		if err != nil {
			run.State, run.Error = "failed", err.Error()
			_ = a.store.SaveRun(run)
			continue
		}
		child, stop := context.WithCancel(ctx)
		a.coordinator.mu.Lock()
		run.State = "running"
		if err = a.store.SaveRun(run); err == nil {
			a.coordinator.running, a.coordinator.cancel = run.ID, stop
			a.coordinator.wg.Add(1)
		}
		a.coordinator.mu.Unlock()
		if err != nil {
			stop()
			return
		}
		// work persists progress and releases the current-run slot. batch remains
		// reserved until the complete queue finishes, including gaps between runs.
		a.work(child, stop, run)
	}
}
