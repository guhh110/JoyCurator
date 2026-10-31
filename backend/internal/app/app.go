// Package app wires the four business modules and the infrastructure adapters.
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
	"joycurator/internal/planning"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

type App struct {
	store        *sqlite.Store
	coordinator  *organizer.Coordinator
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
	factory := matching.Factory(func(s model.Settings, log *trace.Recorder) (matching.Provider, error) {
		client, err := tmdb.New(store, s.Token, s.ProxyURL, s.Language)
		if err == nil {
			client.Trace = log
		}
		return client, err
	})
	recognize, err := recognition.New(store, coordinator.IsClosing)
	if err != nil {
		store.Close()
		return nil, err
	}
	var planner *planning.Service
	config := NewConfig(store, access, coordinator, factory, func() { planner.Invalidate() })
	files := NewFiles(access, config, recognize)
	workspace := NewWorkspace(store, config, factory, recognize, WorkspaceFiles{Access: access, SourceAccess: files}, coordinator)
	var organize *organizer.Service
	match := matching.New(config, factory)
	planner = planning.New(planning.Dependencies{Settings: config, Access: access, Sources: files, Recognition: recognize, Matching: match, Workspaces: workspace, Ownership: store, Verifier: &organizer.Engine{FS: access}, Submit: func(id string, batch bool) (any, error) {
		if batch {
			return organize.ExecuteBatch(id)
		}
		return organize.Execute(id)
	}})
	runner := organizer.RunnerFactory(func(image func(context.Context, string) ([]byte, error)) organizer.Runner {
		return &organizer.Engine{FS: access, Image: image}
	})
	organize = organizer.New(coordinator, store, config, planner, files, factory, runner, workspace.Revision, workspace.FinishRun)
	organize.SetMappings(store)
	organize.SetTargetAccess(access)
	deps := Dependencies{Auth: auth.New(store), Secure: secure, Health: store.Ping, Config: config, Matching: match, Recognition: recognize, Workspace: workspace, Files: files, Planning: planner, Organizer: organize}
	return &App{store: store, coordinator: coordinator, dependencies: deps}, nil
}
func (a *App) Handler(frontend fs.FS) http.Handler { return NewHTTP(a.dependencies, frontend) }
func (a *App) Close() error {
	a.closeOnce.Do(func() { a.coordinator.Close(); a.closeErr = a.store.Close() })
	return a.closeErr
}
