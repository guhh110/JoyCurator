package recognition_test

// Test adapters retain the old fixture representation while exercising the
// new default engine. Production conversion lives only in the app layer.
import (
	"context"
	"path/filepath"
	"strings"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

func MatchTitle(title string) string { return recognition.NormalizeTitle(title) }
func RecognitionPath(path, root string) model.Parsed {
	return RecognitionPathTrace(path, root, nil)
}
func RecognitionPathTrace(path, root string, log *trace.Recorder) model.Parsed {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}
	if root == "" {
		root = "/"
	}
	result, _ := recognition.DefaultEngine().Extract(context.Background(), recognition.Input{Path: path, Root: root}, nil)
	for _, step := range result.Steps {
		log.Add("parse", step.RuleID, step.Status, step.Reason, step)
	}
	p := metadataParsed(result.Metadata)
	log.Add("parse", "parsed-result", "info", "路径规则链执行完成", p)
	return p
}
func Parse(name string) model.Parsed                       { return legacyPath(name, nil) }
func ParsePath(path string) model.Parsed                   { return legacyPath(path, nil) }
func ParsePathIn(path string, roots []string) model.Parsed { return legacyPath(path, roots) }
func legacyPath(path string, roots []string) model.Parsed {
	if !filepath.IsAbs(path) {
		path = "/" + path
	}
	root := "/"
	for _, candidate := range roots {
		candidate = filepath.Clean(candidate)
		rel, err := filepath.Rel(candidate, path)
		if err == nil && filepath.IsLocal(rel) && len(candidate) > len(root) {
			root = candidate
		}
	}
	p := RecognitionPath(path, root)
	if p.Type == "" {
		p.Type = "movie"
	}
	if p.Season < 0 {
		p.Season = 0
	}
	return p
}
func metadataParsed(m recognition.Metadata) model.Parsed {
	p := model.Parsed{Season: -1, Episodes: append([]int{}, m.Episodes...)}
	if m.Title != nil {
		p.Title = *m.Title
	}
	if m.MediaType != nil {
		p.Type = *m.MediaType
	}
	if m.TMDBID != nil {
		p.TMDBID = *m.TMDBID
	}
	if m.Year != nil {
		p.Year = *m.Year
	}
	if m.Season != nil {
		p.Season = *m.Season
	}
	if m.ReleaseTag != nil {
		p.ReleaseTag = strings.TrimSpace(*m.ReleaseTag)
	}
	return p
}
