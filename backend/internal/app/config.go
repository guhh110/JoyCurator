package app

import (
	"errors"
	"path/filepath"
	"strings"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/rules"
)

type ConfigStore interface {
	LoadSettings() (model.Settings, error)
	SaveSettings(model.Settings) error
}
type ConfigUpdate struct {
	model.Settings
	TMDBToken *string `json:"tmdbToken"`
}
type ConfigService struct {
	store       ConfigStore
	access      *fsops.FS
	coordinator *organizer.Coordinator
	factory     matching.Factory
	invalidate  func()
}

func NewConfig(store ConfigStore, access *fsops.FS, c *organizer.Coordinator, f matching.Factory, invalidate func()) *ConfigService {
	return &ConfigService{store, access, c, f, invalidate}
}
func (a *ConfigService) Load() (model.Settings, error) { return a.store.LoadSettings() }
func (a *ConfigService) View() (any, error) {
	s, err := a.Load()
	return struct {
		model.Settings
		HasToken bool     `json:"hasToken"`
		Roots    []string `json:"roots"`
	}{s, s.Token != "", a.access.Roots}, err
}
func (a *ConfigService) Update(input ConfigUpdate) error {
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	if a.coordinator.BusyLocked() {
		return organizer.ErrBusy
	}
	old, err := a.Load()
	if err != nil {
		return err
	}
	if input.Version != old.Version {
		return errors.New("设置已更新，请重新加载")
	}
	if err = rules.Validate(input.Rules); err != nil {
		return err
	}
	if input.Language == "" {
		input.Language = "zh-CN"
	}
	if _, err = a.factory(input.Settings, nil); err != nil {
		return err
	}
	for _, p := range []string{input.SourceRoot, input.TargetRoot} {
		if p == "" || !filepath.IsAbs(p) {
			return errors.New("源目录和目标目录必须填写绝对路径")
		}
		info, e := a.access.Stat(p)
		if e != nil || !info.IsDir() {
			return errors.New("目录不存在或未授权：" + p)
		}
	}
	input.SourceRoot, err = filepath.EvalSymlinks(a.access.Canonical(input.SourceRoot))
	if err != nil {
		return errors.New("无法解析源目录")
	}
	input.TargetRoot, err = filepath.EvalSymlinks(a.access.Canonical(input.TargetRoot))
	if err != nil {
		return errors.New("无法解析目标目录")
	}
	for _, path := range []string{input.SourceRoot, input.TargetRoot} {
		if _, err := a.access.Stat(path); err != nil {
			return errors.New("目录解析后超出授权范围")
		}
	}
	if fsops.Within(input.TargetRoot, input.SourceRoot) {
		return errors.New("源目录不能位于目标目录内或与其相同")
	}

	input.Token = old.Token
	if input.TMDBToken != nil {
		input.Token = strings.TrimSpace(*input.TMDBToken)
	}
	input.Version++
	if len(input.Rules.Categories) == 0 {
		input.Rules.Categories = model.DefaultCategories()
	}
	if err = a.store.SaveSettings(input.Settings); err != nil {
		return err
	}
	a.invalidate()
	return nil
}
