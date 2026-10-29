package organizer

import (
	"context"

	"joycurator/internal/model"
	"joycurator/internal/planning"
)

type Store interface {
	SaveRun(Run) error
	Run(string) (Run, error)
	Runs(int) (map[string]any, error)
	BatchRuns(string) ([]Run, error)
	LatestBatchID() (string, error)
	SaveBatch([]Run) error
	Shared(string, string) (bool, error)
}
type Settings interface {
	Load() (model.Settings, error)
}
type Plans interface {
	Get(string) *planning.Plan
	Remove(string)
	ValidateOps([]model.Work) error
}
type Sources interface {
	SourceAllowed(string, model.Settings) bool
}
type Runner interface {
	Run(context.Context, *model.Operation, func() error) error
	Undo(context.Context, *model.Operation, func() error) error
}
type RunnerFactory func(func(context.Context, string) ([]byte, error)) Runner
