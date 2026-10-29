package organizer

import (
	"errors"

	"joycurator/internal/matching"
)

type Service struct {
	coordinator *Coordinator
	store       Store
	settings    Settings
	plans       Plans
	sources     Sources
	factory     matching.Factory
	runner      RunnerFactory
	revision    func(string) (int, error)
	finish      func(Run)
}

func New(c *Coordinator, store Store, settings Settings, plans Plans, sources Sources, factory matching.Factory, runner RunnerFactory, revision func(string) (int, error), finish func(Run)) *Service {
	return &Service{c, store, settings, plans, sources, factory, runner, revision, finish}
}
func (a *Service) Record(id string) (Run, error)            { return a.store.Run(id) }
func (a *Service) Records(page int) (map[string]any, error) { return a.store.Runs(page) }
func (a *Service) Cancel(id string) error {
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.coordinator.running != id || a.coordinator.cancel == nil {
		return errors.New("任务未在运行")
	}
	a.coordinator.cancel()
	return nil
}
func (a *Service) CancelBatch(id string) error {
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.coordinator.batch != id || a.coordinator.batchCancel == nil {
		return errors.New("批次未在运行")
	}
	a.coordinator.batchCancel()
	return nil
}
func (a *Service) LatestBatch() (*Batch, error) {
	id, err := a.store.LatestBatchID()
	if err != nil || id == "" {
		return nil, err
	}
	b, err := a.ReadBatch(id)
	return &b, err
}
func (a *Service) ReadBatch(id string) (Batch, error) {
	runs, err := a.store.BatchRuns(id)
	b := Batch{ID: id, Runs: []Run{}}
	if err != nil {
		return b, err
	}
	if len(runs) == 0 {
		return b, errors.New("批次不存在")
	}
	for _, r := range runs {
		r.Ops = nil
		b.Runs = append(b.Runs, r)
	}
	a.coordinator.mu.Lock()
	b.Active = a.coordinator.batch == id
	a.coordinator.mu.Unlock()
	return b, nil
}
