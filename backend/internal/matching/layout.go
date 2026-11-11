package matching

import (
	"errors"

	"joycurator/internal/model"
	"path/filepath"
	"regexp"
	"strings"
)

var seasonFolder = regexp.MustCompile(`(?i)^(?:season[ ._-]*[0-9]{1,3}|s[0-9]{1,3}|第[零〇一二两三四五六七八九十百0-9]+季|specials|特别篇)$`)

// InferLayouts only confirms the two supported layouts. Mixed directories remain reviewable.
func InferLayouts(items []Item, target string) {
	dirs := map[string][]int{}
	for i := range items {
		dir := filepath.Dir(items[i].Path)
		dirs[dir] = append(dirs[dir], i)
	}
	for dir, indexes := range dirs {
		same := true
		key := ""
		for _, i := range indexes {
			it := items[i]
			if it.Entry == nil {
				same = false
				continue
			}
			k := EntryGroup(*it.Entry)
			if key != "" && k != key {
				same = false
			}
			key = k
		}
		for _, i := range indexes {
			it := &items[i]
			if it.Entry == nil || it.Layout.Manual {
				continue
			}
			// A valid existing mapping owns its previously confirmed layout.
			if it.Layout.WorkDir != "" {
				if it.Entry.Type == "movie" && !same {
					it.Layout.MoviePrefix = true
				}
				continue
			}
			layout := model.MediaLayout{WorkDir: dir}
			if it.Entry.Type == "movie" {
				layout.MoviePrefix = !same || dir == target
			} else {
				if !same {
					continue
				}
				if !seasonFolder.MatchString(filepath.Base(dir)) {
					nested := false
					for parent := filepath.Dir(dir); withinLayout(target, parent) && parent != target; parent = filepath.Dir(parent) {
						if seasonFolder.MatchString(filepath.Base(parent)) {
							nested = true
							break
						}
					}
					if nested {
						continue
					}
				}
				if seasonFolder.MatchString(filepath.Base(dir)) {
					layout.WorkDir = filepath.Dir(dir)
					layout.SeasonDir = dir
				}
				if layout.WorkDir == target {
					continue
				}
				// More than one work beneath a proposed show root is ambiguous.
				mixed := false
				for _, other := range items {
					if withinLayout(layout.WorkDir, other.Path) && other.Entry != nil && EntryGroup(*other.Entry) != EntryGroup(*it.Entry) {
						mixed = true
						break
					}
				}
				if mixed {
					continue
				}
			}
			if ValidateLayout(layout, it.Path, it.Entry.Type, target) == nil {
				it.Layout = layout
			}
		}
	}
}
func ValidateLayout(v model.MediaLayout, path, kind, target string) error {
	if v.WorkDir == "" || !filepath.IsAbs(v.WorkDir) || filepath.Clean(v.WorkDir) != v.WorkDir || !withinLayout(target, v.WorkDir) || !withinLayout(v.WorkDir, filepath.Dir(path)) {
		return errors.New("请确认 Target 内的作品目录")
	}
	if kind == "movie" && v.SeasonDir != "" {
		return errors.New("电影不能设置季目录")
	}
	if v.SeasonDir != "" && (v.SeasonDir == v.WorkDir || !filepath.IsAbs(v.SeasonDir) || filepath.Clean(v.SeasonDir) != v.SeasonDir || !withinLayout(v.WorkDir, v.SeasonDir) || !withinLayout(v.SeasonDir, filepath.Dir(path))) {
		return errors.New("季目录必须位于作品目录内并包含视频")
	}
	if kind == "tv" && v.MoviePrefix {
		return errors.New("电视剧不能使用电影图片前缀")
	}
	return nil
}
func SameMapping(a, b *model.MediaMapping) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Path != b.Path || a.RunID != b.RunID || a.Source != b.Source || a.Type != b.Type || a.TMDBID != b.TMDBID || a.Season != b.Season || a.Identity != b.Identity || a.Size != b.Size || a.MTime != b.MTime || a.Layout != b.Layout || len(a.Episodes) != len(b.Episodes) {
		return false
	}
	for i := range a.Episodes {
		if a.Episodes[i] != b.Episodes[i] {
			return false
		}
	}
	return true
}
func MovieArtworkPath(m model.MediaMapping, name string) string {
	if m.Layout.MoviePrefix {
		return strings.TrimSuffix(m.Path, filepath.Ext(m.Path)) + "-" + name
	}
	return filepath.Join(m.Layout.WorkDir, name)
}

func withinLayout(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}
