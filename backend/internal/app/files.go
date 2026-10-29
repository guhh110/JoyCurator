package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

type FileService struct {
	access   *fsops.FS
	settings SettingsReader
	extract  *recognition.Service
}

func NewFiles(access *fsops.FS, settings SettingsReader, extract *recognition.Service) *FileService {
	return &FileService{access: access, settings: settings, extract: extract}
}
func (a *FileService) Scan(ctx context.Context, paths []string) ([]model.File, error) {
	if len(paths) == 0 || len(paths) > 200 {
		return nil, errors.New("请选择 1–200 个文件或目录")
	}
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if !a.SourceAllowed(path, s) {
			return nil, errors.New("只能添加源目录内的文件或文件夹，目标目录不能作为来源")
		}
	}
	items := []model.File{}
	err = scanner.Walk(ctx, a.access, paths, []string{s.TargetRoot}, func(path string, info os.FileInfo) error {
		if len(items) >= 2000 {
			return errors.New("单次最多导入 2000 个视频，请分批添加")
		}
		extracted, _, extractErr := a.extract.ExtractFile(ctx, path, s.SourceRoot, "")
		if extractErr != nil {
			return extractErr
		}
		p := recognition.ParsedMetadata(extracted.Metadata)
		items = append(items, model.File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano(), Season: p.Season, Episodes: p.Episodes, Probe: scanner.Probe(ctx, a.access, path)})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, err
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
	if scope != "" && scope != "roots" {
		return nil, errors.New("目录浏览范围无效")
	}
	settings, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	roots := []string{settings.SourceRoot}
	if scope == "roots" {
		roots = a.access.Roots
	}
	if path != "" && scope != "roots" && !a.SourceAllowed(path, settings) {
		return nil, errors.New("只能浏览源目录")
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
			if strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0 || scope != "roots" && !a.SourceAllowed(filepath.Join(path, e.Name()), settings) {
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
