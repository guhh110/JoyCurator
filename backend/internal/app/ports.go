package app

import (
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
)

type WorkspaceStore interface {
	Workspace(string) (matching.Workspace, error)
	SaveWorkspace(*matching.Workspace) error
	CheckpointWorkspace(*matching.Workspace, []int) error
	LatestWorkspace(string) (*matching.Workspace, error)
	FileExecutions(string) (map[string]matching.FileExecution, error)
	Mapping(string) (*model.MediaMapping, error)
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
