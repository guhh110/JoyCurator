package app

import (
	"context"
	"errors"
	"io/fs"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"path/filepath"
	"sort"
)

// Context includes every descendant video. Targets alone are replaced in the workspace.
func (a *WorkspaceService) extractContexts(ctx context.Context, imports, targets []string, s model.Settings, workspace string) (map[string]matching.Item, error) {
	roots := []string{}
	for _, path := range imports {
		relevant := false
		for _, target := range targets {
			if fsops.Within(path, target) || (scanner.IsVideo(path) && fsops.Within(filepath.Dir(path), target)) {
				relevant = true
				break
			}
		}
		if !relevant {
			continue
		}
		info, err := a.files.Access.Stat(path)
		if err != nil && !(errors.Is(err, fs.ErrNotExist) && scanner.IsVideo(path)) {
			return nil, err
		}
		root := path
		if info == nil || !info.IsDir() {
			root = filepath.Dir(path)
		}
		root = a.files.Access.Canonical(root)
		if !a.files.SourceAllowed(root, s) {
			return nil, errors.New("上下文目录超出源目录范围")
		}
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return len(roots[i]) < len(roots[j]) || len(roots[i]) == len(roots[j]) && roots[i] < roots[j]
	})
	collapsed := []string{}
	for _, root := range roots {
		covered := false
		for _, outer := range collapsed {
			if fsops.Within(outer, root) {
				covered = true
				break
			}
		}
		if !covered {
			collapsed = append(collapsed, root)
		}
	}
	sort.Strings(collapsed)
	out := map[string]matching.Item{}
	for _, root := range collapsed {
		wanted := []string{}
		for _, p := range targets {
			if fsops.Within(root, p) {
				wanted = append(wanted, p)
			}
		}
		if len(wanted) == 0 {
			continue
		}
		paths := []string{}
		infos := map[string]fs.FileInfo{}
		err := scanner.Walk(ctx, a.files.Access, []string{root}, []string{s.TargetRoot}, func(path string, info fs.FileInfo) error {
			if len(paths) >= 2000 {
				return errors.New("目录上下文最多包含 2000 个视频，请缩小导入目录")
			}
			if !a.files.SourceAllowed(path, s) {
				return errors.New("上下文文件超出源目录范围")
			}
			paths = append(paths, path)
			infos[path] = info
			return nil
		})
		if err != nil {
			return nil, err
		}
		batch, trace, err := a.extract.ExtractTree(ctx, recognition.BatchInput{ContextRoot: root, SourceBoundary: s.SourceRoot, Paths: paths, Targets: wanted}, workspace)
		if err != nil {
			return nil, err
		}
		for _, f := range batch.Files {
			out[f.Path] = itemFromMetadata(f.Path, infos[f.Path], f.Metadata, trace)
		}
	}
	if len(out) != len(targets) {
		return nil, errors.New("部分识别目标不在目录快照内")
	}
	return out, nil
}
