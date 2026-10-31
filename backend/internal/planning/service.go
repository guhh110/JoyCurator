package planning

import (
	"sync"
	"time"

	"joycurator/internal/infra/fsops"
)

type Service struct {
	mu         sync.Mutex
	plans      map[string]*Plan
	settings   Settings
	access     *fsops.FS
	sources    Sources
	workspaces Workspaces
	ownership  Ownership
	verifier   Verifier
	submit     func(string, bool) (any, error)
}

type Dependencies struct {
	Settings   Settings
	Access     *fsops.FS
	Sources    Sources
	Workspaces Workspaces
	Ownership  Ownership
	Verifier   Verifier
	Submit     func(string, bool) (any, error)
}

func New(d Dependencies) *Service {
	return &Service{plans: map[string]*Plan{}, settings: d.Settings, access: d.Access, sources: d.Sources, workspaces: d.Workspaces, ownership: d.Ownership, verifier: d.Verifier, submit: d.Submit}
}
func (a *Service) Get(id string) *Plan {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.plans[id]
}
func (a *Service) Remove(id string) { a.mu.Lock(); defer a.mu.Unlock(); delete(a.plans, id) }
func (a *Service) Invalidate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.plans = map[string]*Plan{}
}
func (a *Service) storePlan(plan *Plan) {
	for id, p := range a.plans {
		if time.Now().After(p.Expires) {
			delete(a.plans, id)
		}
	}
	if len(a.plans) >= 32 {
		a.plans = map[string]*Plan{}
	}
	a.plans[plan.ID] = plan
}
