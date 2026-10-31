package organizer

import (
	"errors"
	"joycurator/internal/model"
	"sync"
	"time"

	"joycurator/internal/matching"
)

type Service struct {
	scrapeMu    sync.Mutex
	scrapePlans map[string]scrapePreview
	mappings    interface {
		Mappings() ([]model.MediaMapping, error)
		SaveMapping(model.MediaMapping) error
	}
	access      TargetAccess
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
	return &Service{coordinator: c, store: store, settings: settings, plans: plans, sources: sources, factory: factory, runner: runner, revision: revision, finish: finish, scrapePlans: map[string]scrapePreview{}}
}

func (a *Service) SetMappings(m interface {
	Mappings() ([]model.MediaMapping, error)
	SaveMapping(model.MediaMapping) error
})                                                     { a.mappings = m }
func (a *Service) SetTargetAccess(access TargetAccess) { a.access = access }

func (a *Service) ScrapeRecords(page int) (map[string]any, error) {
	return a.store.RunsByPurpose(page, "scrape")
}

func (a *Service) ScrapeRecord(id string) (Run, error) {
	r, e := a.store.Run(id)
	if e == nil && r.Purpose != "scrape" {
		return r, errors.New("刮削记录不存在")
	}
	return r, e
}

func (a *Service) ScrapeResume(id string, undo bool) (Run, error) {
	r, e := a.ScrapeRecord(id)
	if e != nil {
		return r, e
	}
	return a.Resume(id, undo)
}

func (a *Service) OrganizeResume(id string, undo bool) (Run, error) {
	r, e := a.Record(id)
	if e != nil {
		return r, e
	}
	return a.Resume(id, undo)
}

func (a *Service) ScrapeCancel(id string) error {
	if _, e := a.ScrapeRecord(id); e != nil {
		return e
	}
	return a.Cancel(id)
}

func (a *Service) OrganizeCancel(id string) error {
	if _, e := a.Record(id); e != nil {
		return e
	}
	return a.Cancel(id)
}

func (a *Service) ScrapeQueue() ([]model.MediaMapping, error) {
	return a.scrapeQueue()
}

type scrapePreview struct {
	plan     ScrapePlan
	ops      []model.Work
	version  int
	expires  time.Time
	mappings []model.MediaMapping
}

func (a *Service) Record(id string) (Run, error) {
	r, err := a.store.Run(id)
	if err == nil && r.Purpose != "organize" {
		return Run{}, errors.New("整理记录不存在")
	}
	return r, err
}
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
