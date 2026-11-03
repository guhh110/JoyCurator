package app

import (
	"errors"
	"joycurator/internal/model"
	"time"
)

type MaintenanceStore interface {
	PreviewCleanup(model.CleanupRequest) (model.CleanupPreview, error)
	Cleanup(model.CleanupRequest) (model.CleanupCounts, error)
}
type MaintenanceService struct{ store MaintenanceStore }

func NewMaintenance(store MaintenanceStore) *MaintenanceService {
	return &MaintenanceService{store: store}
}
func (s *MaintenanceService) PreviewCleanup(r model.CleanupRequest) (model.CleanupPreview, error) {
	if err := normalizeCleanup(&r); err != nil {
		return model.CleanupPreview{}, err
	}
	return s.store.PreviewCleanup(r)
}
func (s *MaintenanceService) Cleanup(r model.CleanupRequest) (model.CleanupCounts, error) {
	if err := normalizeCleanup(&r); err != nil {
		return model.CleanupCounts{}, err
	}
	return s.store.Cleanup(r)
}
func normalizeCleanup(r *model.CleanupRequest) error {
	now := time.Now()
	if !r.Traces && !r.Cache {
		return errors.New("请选择需要清理的数据")
	}
	if r.Traces {
		before, err := time.Parse(time.RFC3339Nano, r.Before)
		if err != nil || before.After(now) {
			return errors.New("请选择不晚于当前时间的历史截止时间")
		}
		r.Before = before.UTC().Format(time.RFC3339Nano)
	}
	if r.CacheBefore == 0 {
		r.CacheBefore = now.Unix()
	}
	if r.CacheBefore < 0 || r.CacheBefore > now.Unix() {
		return errors.New("缓存截止时间无效")
	}
	return nil
}
