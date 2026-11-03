package organizer

import (
	"context"
	"joycurator/internal/matching"

	"joycurator/internal/model"
)

type Store interface {
	SaveCheckpoint(Run, []int, []int, []model.MediaMapping) error
	Run(string) (Run, error)
	BatchSummaries(string) ([]Run, error)
	Enqueue([]Run) error
	QueuedRuns() ([]Run, error)
	FinishQueued(Run) error
	CancelQueued([]string) error
	QueuePage(string, int) (RunPage, error)
	Batches(int) (BatchPage, error)
	BatchPosition(string) (int, error)
	FileExecutions(string) (map[string]matching.FileExecution, error)
	Shared(string, string, string) (bool, error)
	ScrapeArtifact(string) (*Work, error)
	DeleteMapping(model.MediaMapping) error
	UpdateMappingState(model.MediaMapping, string, string) error
	RunsByPurpose(int, string, RunFilter) (RunPage, error)
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
