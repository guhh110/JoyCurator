package recognition

import (
	"errors"
	"sync"
)

type Service struct {
	mu      sync.RWMutex
	engine  *Engine
	store   Store
	closing func() bool
}

func New(store Store, closing func() bool) (*Service, error) {
	rules, err := store.LoadRules()
	if err != nil {
		return nil, err
	}
	rules, changed, err := MigrateLegacyRules(rules)
	if err != nil {
		return nil, err
	}
	engine, err := Compile(rules)
	if err != nil {
		return nil, err
	}
	if changed {
		if err = store.SaveRules(engine.Rules()); err != nil {
			return nil, err
		}
	}
	return &Service{store: store, engine: engine, closing: closing}, nil
}
func (a *Service) Current() *Engine { a.mu.RLock(); defer a.mu.RUnlock(); return a.engine }
func (a *Service) Save(rules []Rule) error {
	engine, err := Compile(rules)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing() {
		return errors.New("服务正在关闭")
	}
	if err = a.store.SaveRules(engine.Rules()); err != nil {
		return err
	}
	a.engine = engine
	return nil
}
func (a *Service) History(page int, path, workspace string) ([]Trace, int, error) {
	return a.store.TraceHistory(page, path, workspace)
}
func (a *Service) Detail(id string) (Trace, []Step, error) {
	return a.store.TraceDetail(id)
}
func (a *Service) Delete(id string) error { return a.store.DeleteTrace(id) }
