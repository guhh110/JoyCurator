package planning

import (
	"context"
	"errors"
	"time"
)

// Submit confirms an existing preview. The injected organizer owns admission,
// final revalidation and execution; its existing Run/Batch receipt passes through.
func (a *Service) Submit(id string, batch bool) (any, error) {
	p := a.Get(id)
	if p == nil || time.Now().After(p.Expires) {
		return nil, errors.New("预览已过期，请重新预览")
	}
	if a.submit == nil {
		return nil, errors.New("整理执行器未配置")
	}
	return a.submit(id, batch)
}

// Edit produces a new preview ID, so an earlier confirmation cannot execute
// newly edited instructions that the user has not reviewed.
func (a *Service) Edit(ctx context.Context, id string, input PreviewRequest) (*Plan, error) {
	old := a.Get(id)
	if old == nil || time.Now().After(old.Expires) {
		return nil, errors.New("预览已过期，请重新预览")
	}
	if len(old.Groups) > 0 {
		return nil, errors.New("批量计划请修改工作区后重新预览")
	}
	plan, err := a.Preview(ctx, input)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.plans[id] != old || time.Now().After(old.Expires) {
		delete(a.plans, plan.ID)
		return nil, errors.New("预览已变化，请重新预览")
	}
	delete(a.plans, id)
	return plan, nil
}

// Delete discards only an unsubmitted in-memory preview. Runs are never removed.
func (a *Service) Delete(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.plans[id] == nil {
		return errors.New("预览不存在或已提交")
	}
	delete(a.plans, id)
	return nil
}
