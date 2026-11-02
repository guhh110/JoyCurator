package organizer

import (
	"errors"
	"runtime"
	"time"

	"joycurator/internal/model"
)

func (a *Service) ExecuteBatch(id string) (Batch, error) {
	if runtime.GOOS == "windows" {
		return Batch{}, errors.New("Windows 暂不支持执行文件整理")
	}
	p := a.previews.get(id)
	if p == nil || time.Now().After(p.Expires) {
		return Batch{}, errors.New("预览已过期，请重新预览")
	}
	if p.Purpose != "organize" || len(p.Groups) == 0 || len(p.Mappings) == 0 || len(p.Issues) > 0 {
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
	if err = a.validateOps(p.Ops); err != nil {
		return Batch{}, err
	}
	if err = a.validateSkipped(p.Mappings, p.Ops); err != nil {
		return Batch{}, err
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.coordinator.closed {
		return Batch{}, errors.New("服务正在关闭")
	}
	if a.previews.get(id) != p {
		return Batch{}, errors.New("预览已过期或已提交")
	}
	current, err := a.settings.Load()
	if err != nil {
		return Batch{}, err
	}
	revision, err = a.revision(p.RecognitionID)
	if err != nil {
		return Batch{}, err
	}
	if current.Version != p.Version || revision != p.Revision {
		return Batch{}, errors.New("设置或工作区已变化，请重新预览")
	}
	batch := Batch{ID: model.Key(), Runs: []Run{}, Active: true, State: "queued"}
	for _, group := range p.Groups {
		if len(group.Mappings) == 0 {
			continue
		}
		run := Run{ID: model.Key(), Title: group.Title, Mode: p.Mode, State: "queued", Created: time.Now().Format(time.RFC3339), Ops: append([]Work(nil), group.Ops...), Total: len(group.Ops), BatchID: batch.ID, Version: p.Version, RecognitionID: p.RecognitionID, Purpose: "organize", Mappings: append([]model.MediaMapping(nil), group.Mappings...)}

		batch.Runs = append(batch.Runs, run)
	}
	if err = a.enqueueLocked(batch.Runs); err != nil {
		return Batch{}, err
	}
	a.previews.remove(id)
	batch.Runs = append([]Run(nil), batch.Runs...)
	for i := range batch.Runs {
		batch.Runs[i].Ops = nil
		batch.Runs[i].Mappings = nil
	}
	return batch, nil
}
