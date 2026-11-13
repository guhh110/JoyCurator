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
// scanned contains the completed import walk, when available. A file import
// still needs its parent scanned so sibling evidence is never lost.
func (a *WorkspaceService) extractContexts(ctx context.Context, imports, targets []string, s model.Settings, workspace, purpose string, scanned map[string]fs.FileInfo, progress ...recognition.Progress) (map[string]matching.Item, error) {
	roots := []string{}
	scannedRoots := map[string]bool{}
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
		if info != nil && info.IsDir() {
			scannedRoots[root] = true
		}
		if !a.allowed(purpose, root, s) {
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
	completed := 0
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
		excluded := []string{}
		boundary := s.TargetRoot
		if purpose == "organize" {
			excluded = []string{s.TargetRoot}
			boundary = s.SourceRoot
		}
		visit := func(path string, info fs.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(paths) >= 2000 {
				return errors.New("目录上下文最多包含 2000 个视频，请缩小导入目录")
			}
			if !a.allowed(purpose, path, s) {
				return errors.New("上下文文件超出源目录范围")
			}
			paths = append(paths, path)
			infos[path] = info
			return nil
		}
		var err error
		if scanned != nil && scannedRoots[root] {
			for path, info := range scanned {
				if fsops.Within(root, path) {
					if err = visit(path, info); err != nil {
						break
					}
				}
			}
		} else {
			err = scanner.Walk(ctx, a.files.Access, []string{root}, excluded, visit)
		}
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		batch, trace, err := a.extract.ExtractTree(ctx, recognition.BatchInput{ContextRoot: root, SourceBoundary: boundary, Paths: paths, Targets: wanted}, workspace, func(done, total int) error {
			if len(progress) > 0 && progress[0] != nil {
				return progress[0](completed+done, len(targets))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		completed += len(wanted)
		for _, f := range batch.Files {
			out[f.Path] = itemFromMetadata(f.Path, infos[f.Path], f.Metadata, trace)
		}
	}
	if len(out) != len(targets) {
		return nil, errors.New("部分识别目标不在目录快照内")
	}
	return out, nil
}
