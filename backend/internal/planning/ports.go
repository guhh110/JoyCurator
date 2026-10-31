package planning

import (
	"context"

	"joycurator/internal/matching"
	"joycurator/internal/model"
)

type Settings interface {
	Load() (model.Settings, error)
}
type Sources interface {
	SourceAllowed(string, model.Settings) bool
}
type Workspaces interface {
	Read(string) (matching.Workspace, error)
}
type Ownership interface {
	Owned(string) (*model.Work, error)
}

type Verifier interface {
	Check(context.Context, string, string, string) error
}
