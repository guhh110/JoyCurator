// Package app wires the three business modules and the infrastructure adapters.
package app

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"sync"

	"joycurator/internal/auth"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/sqlite"
	"joycurator/internal/infra/tmdb"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

type App struct {
	store        *sqlite.Store
	coordinator  *organizer.Coordinator
	tmdbPool     *tmdb.Pool
	dependencies Dependencies
	closeOnce    sync.Once
	closeErr     error
}

func Open(dir, sourceRoot, targetRoot string, secure bool) (*App, error) {
	access, err := fsops.New([]string{sourceRoot, targetRoot})
	if err != nil {
		return nil, err
	}
	initial := model.DefaultSettings()
	initial.SourceRoot, initial.TargetRoot = access.Roots[0], access.Roots[1]
	if fsops.Within(initial.TargetRoot, initial.SourceRoot) {
		return nil, errors.New("源目录不能位于目标目录内或与其相同")
	}
	store, err := sqlite.Open(dir, initial)
	if err != nil {
		return nil, err
	}
	coordinator := organizer.NewCoordinator()
	pool := &tmdb.Pool{}
	factory := matching.Factory(func(s model.Settings, log *trace.Recorder) (matching.Provider, error) {
		return pool.Client(store, s.Token, s.ProxyURL, s.Language, log)
	})
	recognize, err := recognition.New(store, coordinator.IsClosing)
	if err != nil {
		store.Close()
		return nil, err
	}
	previews := organizer.NewPreviewCache()
	config := NewConfig(store, access, coordinator, factory, previews.Invalidate)
	files := NewFiles(access, config)
	workspace := NewWorkspace(store, config, factory, recognize, WorkspaceFiles{Access: access, SourceAccess: files}, coordinator)
	match := matching.New(config, factory)
	runner := organizer.RunnerFactory(func(image func(context.Context, string) ([]byte, error)) organizer.Runner {
		return &organizer.Engine{FS: access, Image: image}
	})
	organize := organizer.New(organizer.Dependencies{Coordinator: coordinator, Store: store, Settings: config,
		Previews: previews, Access: access, Workspaces: workspace, Mappings: store, Sources: files, Factory: factory,
		Runner: runner, Revision: workspace.Revision})
	if err := organize.Start(); err != nil {
		coordinator.Close()
		pool.Close()
		store.Close()
		return nil, err
	}
	deps := Dependencies{Auth: auth.New(store), Secure: secure, Health: store.Ping, Maintenance: NewMaintenance(store), Config: config, Matching: match, Recognition: recognize, Workspace: workspace, Files: files, Organizer: organize}
	return &App{store: store, coordinator: coordinator, tmdbPool: pool, dependencies: deps}, nil
}
func (a *App) Handler(frontend fs.FS) http.Handler { return NewHTTP(a.dependencies, frontend) }
func (a *App) Close() error {
	a.closeOnce.Do(func() { a.coordinator.Close(); a.tmdbPool.Close(); a.closeErr = a.store.Close() })
	return a.closeErr
}
