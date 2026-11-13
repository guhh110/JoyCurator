package app

import (
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"os"
	"path/filepath"
)

func targetAllowed(path string, s model.Settings) bool {
	real, err := filepath.EvalSymlinks(path)
	return err == nil && real == path && s.TargetRoot != "" && fsops.Within(s.TargetRoot, path)
}
func (a *WorkspaceService) allowed(purpose, path string, s model.Settings) bool {
	if purpose == "scrape" {
		return targetAllowed(a.files.Access.Canonical(path), s)
	}
	return purpose == "organize" && a.files.SourceAllowed(path, s)
}
func (a *WorkspaceService) reusedTarget(path string, info os.FileInfo) (*matching.Item, error) {
	m, err := a.store.Mapping(path)
	if err != nil {
		return nil, err
	}
	if m == nil || m.Identity != fsops.Identity(info) || m.Size != info.Size() || m.MTime != info.ModTime().UnixNano() {
		return nil, nil
	}
	entry := m.Entry
	if entry == nil || entry.Type != m.Type || entry.TMDBID != m.TMDBID {
		return nil, errors.New("已登记视频缺少已确认作品快照")
	}
	item := &matching.Item{File: model.File{Path: path, Size: m.Size, MTime: m.MTime, Season: m.Season, Episodes: m.Episodes}, Identity: m.Identity, Layout: m.Layout, Original: m, Entry: entry, Group: matching.EntryGroup(*entry), State: "ready", Selected: true, Reason: "复用已登记信息"}
	item.Confirm("registration", matching.EpisodeBasis{Kind: "manual"})
	return item, nil
}
