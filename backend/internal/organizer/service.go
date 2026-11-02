package organizer

import (
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"

	"joycurator/internal/matching"
)

type Service struct {
	previews    *PreviewCache
	workspaces  Workspaces
	mappings    Mappings
	access      *fsops.FS
	coordinator *Coordinator
	store       Store
	settings    Settings
	sources     Sources
	factory     matching.Factory
	runner      RunnerFactory
	revision    func(string) (int, error)
	finish      func(Run)
}

type Dependencies struct {
	Coordinator *Coordinator
	Store       Store
	Settings    Settings
	Previews    *PreviewCache
	Access      *fsops.FS
	Workspaces  Workspaces
	Mappings    Mappings
	Sources     Sources
	Factory     matching.Factory
	Runner      RunnerFactory
	Revision    func(string) (int, error)
	Finish      func(Run)
}

func New(d Dependencies) *Service {
	return &Service{coordinator: d.Coordinator, store: d.Store, settings: d.Settings, previews: d.Previews,
		access: d.Access, workspaces: d.Workspaces, mappings: d.Mappings, sources: d.Sources,
		factory: d.Factory, runner: d.Runner, revision: d.Revision, finish: d.Finish}
}

func (a *Service) ScrapeRecords(page int) (RunPage, error) {
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

type ScrapeWork struct {
	Type           string `json:"type"`
	TMDBID         int    `json:"tmdbId"`
	Representative string `json:"representative"`
	Total          int    `json:"total"`
	Pending        int    `json:"pending"`
	Failed         int    `json:"failed"`
	Completed      int    `json:"completed"`
}
type ScrapeWorkPage struct {
	Items      []ScrapeWork `json:"items"`
	NextCursor string       `json:"nextCursor"`
}

func (a *Service) ScrapeWorks(search, cursor string) (ScrapeWorkPage, error) {
	return a.mappings.ScrapeWorks(search, cursor)
}
func (a *Service) ScrapeWork(kind string, id int) ([]model.MediaMapping, error) {
	items, err := a.mappings.ScrapeWorkMappings(kind, id)
	if err != nil {
		return nil, err
	}
	return a.checkScrapeAvailability(items)
}

func (a *Service) Record(id string) (Run, error) {
	r, err := a.store.Run(id)
	if err == nil && r.Purpose != "organize" {
		return Run{}, errors.New("整理记录不存在")
	}
	return r, err
}
func (a *Service) Records(page int) (RunPage, error) { return a.store.RunsByPurpose(page, "organize") }
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
	runs, err := a.store.BatchSummaries(id)
	b := Batch{ID: id, Runs: runs}
	if err != nil {
		return b, err
	}
	if len(runs) == 0 {
		return b, errors.New("批次不存在")
	}
	a.coordinator.mu.Lock()
	b.Active = a.coordinator.batch == id
	a.coordinator.mu.Unlock()
	return b, nil
}
