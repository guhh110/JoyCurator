package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"joycurator/internal/infra/fsops"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true, ".ts": true, ".m2ts": true, ".wmv": true, ".webm": true, ".mpg": true, ".mpeg": true, ".flv": true}

func IsVideo(name string) bool { return videoExt[strings.ToLower(filepath.Ext(name))] }

func Walk(ctx context.Context, access *fsops.FS, sources, excluded []string, visit func(string, fs.FileInfo) error) error {
	canonicalExcluded := make([]string, len(excluded))
	for i, path := range excluded {
		if path != "" {
			canonicalExcluded[i] = access.Canonical(path)
		}
	}
	excluded = canonicalExcluded
	seen := map[string]bool{}
	for _, source := range sources {
		source = access.Canonical(source)
		root, rel, err := access.Root(source)
		if err != nil {
			return err
		}
		err = fs.WalkDir(root.FS(), filepath.ToSlash(rel), func(name string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			abs := filepath.Join(root.Name(), filepath.FromSlash(name))
			if d.Type()&fs.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				for _, e := range excluded {
					if e != "" && fsops.Within(e, abs) {
						return fs.SkipDir
					}
				}
				if strings.HasPrefix(d.Name(), ".") && abs != source {
					return fs.SkipDir
				}
				return nil
			}
			if seen[abs] || !videoExt[strings.ToLower(filepath.Ext(name))] || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			seen[abs] = true
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			return visit(abs, info)
		})
		root.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
