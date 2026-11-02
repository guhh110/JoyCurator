package organizer

import (
	"context"
	"joycurator/internal/matching"

	"joycurator/internal/model"
)

type Store interface {
	SaveRun(Run) error
	Run(string) (Run, error)
	BatchSummaries(string) ([]Run, error)
	LatestBatchID() (string, error)
	SaveBatch([]Run) error
	Shared(string, string, string) (bool, error)
	SaveMapping(model.MediaMapping) error
	DeleteMapping(model.MediaMapping) error
	UpdateMappingState(model.MediaMapping, string, string) error
	Mapping(string) (*model.MediaMapping, error)
	RunsByPurpose(int, string) (RunPage, error)
}
type Settings interface {
	Load() (model.Settings, error)
}
type Workspaces interface {
	Read(string) (matching.Workspace, error)
}
type Mappings interface {
	ScrapeWorks(string, string) (ScrapeWorkPage, error)
	ScrapeWorkMappings(string, int) ([]model.MediaMapping, error)
	Mapping(string) (*model.MediaMapping, error)
}
type Sources interface {
	SourceAllowed(string, model.Settings) bool
}
type Runner interface {
	Run(context.Context, *Operation, func() error) error
	Undo(context.Context, *Operation, func() error) error
}
type RunnerFactory func(func(context.Context, string) ([]byte, error)) Runner
