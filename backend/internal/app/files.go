package app

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/model"
)

type FileService struct {
	access   *fsops.FS
	settings SettingsReader
}

func NewFiles(access *fsops.FS, settings SettingsReader) *FileService {
	return &FileService{access, settings}
}
func (a *FileService) SourceAllowed(path string, s model.Settings) bool {
	path = a.access.Canonical(path)
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return false
	}
	return s.SourceRoot != "" && fsops.Within(a.access.Canonical(s.SourceRoot), path) && (s.TargetRoot == "" || !fsops.Within(a.access.Canonical(s.TargetRoot), path))
}
func (a *FileService) Browse(inputPath, scopeInput string) (any, error) {
	type item struct {
		Path string `json:"path"`
		Dir  bool   `json:"dir"`
	}
	items := []item{}
	path := inputPath
	scope := scopeInput
	if scope != "" && scope != "roots" && scope != "target" {
		return nil, errors.New("目录浏览范围无效")
	}
	settings, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	roots := []string{settings.SourceRoot}
	if scope == "target" {
		roots = []string{settings.TargetRoot}
	}
	if scope == "roots" {
		roots = a.access.Roots
	}
	allowed := func(p string) bool {
		if scope == "roots" {
			return true
		}
		if scope == "target" {
			return targetAllowed(a.access.Canonical(p), settings)
		}
		return a.SourceAllowed(p, settings)
	}
	if path != "" && !allowed(path) {
		return nil, errors.New("目录不在允许的浏览范围内")
	}
	if path == "" {
		for _, p := range roots {
			items = append(items, item{p, true})
		}
	} else {
		root, rel, err := a.access.Root(path)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		stat, err := root.Lstat(rel)
		if err != nil || !stat.IsDir() {
			return nil, errors.New("不是可访问的目录")
		}
		f, err := root.Open(rel)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		entries, err := f.ReadDir(-1)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0 || !allowed(filepath.Join(path, e.Name())) {
				continue
			}
			if e.IsDir() || scanner.IsVideo(e.Name()) {
				items = append(items, item{filepath.Join(path, e.Name()), e.IsDir()})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Dir != items[j].Dir {
			return items[i].Dir
		}
		return items[i].Path < items[j].Path
	})
	return items, nil
}
