package recognition

import (
	"context"
	"path/filepath"

	"joycurator/internal/model"
)

func ParsedMetadata(metadata Metadata) model.Parsed {
	p := model.Parsed{Season: -1, Episodes: append([]int{}, metadata.Episodes...)}
	if metadata.Title != nil {
		p.Title = *metadata.Title
	}
	if metadata.MediaType != nil {
		p.Type = *metadata.MediaType
	}
	if metadata.TMDBID != nil {
		p.TMDBID = *metadata.TMDBID
	}
	if metadata.Year != nil {
		p.Year = *metadata.Year
	}
	if metadata.Season != nil {
		p.Season = *metadata.Season
	}
	if metadata.ReleaseTag != nil {
		p.ReleaseTag = *metadata.ReleaseTag
	}
	return p
}

// Parse is the common non-persistent entry used by planning and simulation.
func (a *Service) Parse(ctx context.Context, path, root string) (Result, error) {
	return a.Current().Extract(ctx, Input{Path: path, Root: root}, nil)
}

// MetadataFromParsed is only for reading the legacy API and stored workspace DTO.
func MetadataFromParsed(p model.Parsed) Metadata {
	m := Metadata{Episodes: append([]int{}, p.Episodes...)}
	if p.TMDBID > 0 {
		v := p.TMDBID
		m.TMDBID = &v
	}
	if p.Title != "" {
		v := p.Title
		m.Title = &v
	}
	if p.Type != "" {
		v := p.Type
		m.MediaType = &v
	}
	if p.Year > 0 {
		v := p.Year
		m.Year = &v
	}
	if p.Season >= 0 {
		v := p.Season
		m.Season = &v
	}
	if p.ReleaseTag != "" {
		v := p.ReleaseTag
		m.ReleaseTag = &v
	}
	return m
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
