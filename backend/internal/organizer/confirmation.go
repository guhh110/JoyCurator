package organizer

import (
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"slices"
)

// Called under queue admission's coordinator lock; workspace edits share it.
func (a *Service) checkConfirmations(p *Plan) error {
	w, err := a.workspaces.Read(p.RecognitionID)
	if err != nil {
		return err
	}
	if w.Revision != p.Revision || w.State == "running" || w.Purpose != p.Purpose {
		return errors.New("工作区已变化，请重新预览")
	}
	items := map[string]matching.Item{}
	for _, it := range w.Items {
		items[it.Path] = it
	}
	for _, m := range p.Mappings {
		path := m.Source
		if p.Purpose == "scrape" {
			path = m.Path
		}
		it, ok := items[path]
		if !ok || !it.Confirmed() || it.State != "ready" || it.Removed || !it.Selected || it.Entry.Type != m.Type || it.Entry.TMDBID != m.TMDBID || it.Season != m.Season || !slices.Equal(it.Episodes, m.Episodes) {
			return errors.New("文件尚未确认或确认结果已变化，请重新预览")
		}
		info, e := a.access.Stat(path)
		if e != nil || !info.Mode().IsRegular() || fsops.Identity(info) != it.Identity || info.Size() != it.Size || info.ModTime().UnixNano() != it.MTime {
			return errors.New("文件已变化，请重新识别并确认")
		}
	}
	return nil
}
