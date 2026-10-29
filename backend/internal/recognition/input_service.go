package recognition

import (
	"context"
	"path/filepath"
)

// Parse is the common non-persistent entry used by planning and simulation.
func (a *Service) Parse(ctx context.Context, path, root string) (Result, error) {
	return a.Current().Extract(ctx, Input{Path: path, Root: root}, nil)
}

func Root(path, root, source string) string {
	if root != "" {
		return root
	}
	if source != "" {
		rel, err := filepath.Rel(source, filepath.Clean(path))
		if err == nil && filepath.IsLocal(rel) {
			return source
		}
	}
	return "/"
}
