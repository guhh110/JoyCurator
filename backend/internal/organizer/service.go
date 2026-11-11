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
}

func New(d Dependencies) *Service {
	return &Service{coordinator: d.Coordinator, store: d.Store, settings: d.Settings, previews: d.Previews,
		access: d.Access, workspaces: d.Workspaces, mappings: d.Mappings, sources: d.Sources,
		factory: d.Factory, runner: d.Runner, revision: d.Revision}
}

func (a *Service) ScrapeRecords(page int, filter RunFilter) (RunPage, error) {
	return a.store.RunsByPurpose(page, "scrape", filter)
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
	items, err = a.checkScrapeAvailability(items)
	if err != nil {
		return nil, err
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	lane := a.coordinator.lanes["scrape"]
	for i := range items {
		for _, r := range lane.pending {
			for _, m := range r.Mappings {
				if m.Path == items[i].Path {
					items[i].State = "queued"
				}
			}
		}
		if lane.active != nil {
			for _, m := range lane.active.Mappings {
				if m.Path == items[i].Path {
					items[i].State = "running"
				}
			}
		}
	}
	return items, nil
}

func (a *Service) Record(id string) (Run, error) {
	r, err := a.store.Run(id)
	if err == nil && r.Purpose != "organize" {
		return Run{}, errors.New("整理记录不存在")
	}
	return r, err
}
func (a *Service) Records(page int, filter RunFilter) (RunPage, error) {
	return a.store.RunsByPurpose(page, "organize", filter)
}
func (a *Service) Cancel(id string) error {
	c := a.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, lane := range c.lanes {
		if lane.active != nil && lane.active.ID == id {
			lane.cancel()
			return nil
		}
		for i, r := range lane.pending {
			if r.ID == id {
				if err := a.store.CancelQueued([]string{id}); err != nil {
					return err
				}
				lane.pending = append(lane.pending[:i], lane.pending[i+1:]...)
				c.signalLocked()
				return nil
			}
		}
	}
	return errors.New("任务未在运行或排队")
}
func (a *Service) CancelBatch(id string) error {
	c := a.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	lane := c.lanes["organize"]
	ids := []string{}
	for _, r := range lane.pending {
		if r.BatchID == id {
			ids = append(ids, r.ID)
		}
	}
	active := lane.active != nil && lane.active.BatchID == id
	if len(ids) == 0 && !active {
		return errors.New("批次未在运行或排队")
	}
	if err := a.store.CancelQueued(ids); err != nil {
		return err
	}
	pending := lane.pending[:0]
	for _, r := range lane.pending {
		if r.BatchID != id {
			pending = append(pending, r)
		}
	}
	lane.pending = pending
	if active {
		lane.cancel()
	}
	c.signalLocked()
	return nil
}
func (a *Service) ReadBatch(id string) (Batch, error) {
	runs, err := a.store.BatchSummaries(id)
	b := Batch{ID: id, Runs: runs}
	if err != nil {
		return b, err
	}
	if len(runs) == 0 || runs[0].Purpose != "organize" {
		return b, errors.New("批次不存在")
	}
	b.State = "completed"
	for _, r := range runs {
		if r.State == "running" {
			b.State = "running"
			b.Active = true
			break
		}
		if r.State == "queued" {
			b.State = "queued"
			b.Active = true
		}
		if !b.Active && r.State != "completed" && r.State != "undone" {
			b.State = r.State
		}
	}
	b.Position, err = a.store.BatchPosition(id)
	return b, err
}
