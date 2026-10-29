package app

import (
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
)

type WorkspaceStore interface {
	Workspace(string) (matching.Workspace, error)
	SaveWorkspace(*matching.Workspace) error
	LatestWorkspace() (*matching.Workspace, error)
}
type SettingsReader interface {
	Load() (model.Settings, error)
}
type SourceAccess interface {
	SourceAllowed(string, model.Settings) bool
}
type WorkspaceFiles struct {
	Access *fsops.FS
	SourceAccess
}
