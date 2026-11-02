package organizer

import (
	"errors"
	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

func (a *Service) persistProgress(r *Run, positions ...int) error {
	registered := []model.MediaMapping{}
	changedMappings := []int{}
	previous := map[int]model.MediaMapping{}
	committed := false
	defer func() {
		if !committed {
			for i, m := range previous {
				r.Mappings[i] = m
			}
		}
	}()
	if r.Purpose == "organize" && !r.Undo {
		completed := map[string]string{}
		for _, op := range r.Ops {
			if op.Kind == "video" && op.Status == "completed" {
				completed[op.Target] = op.Identity
			}
		}
		for i, m := range r.Mappings {
			if m.RunID == r.ID {
				continue
			}
			identity, ready := completed[m.Path]
			if ready {
				m.Identity = identity
			}
			// Skipped, pre-existing videos have an identity in the accepted plan.
			if !ready && m.Identity == "" {
				continue
			}
			info, err := a.access.Stat(m.Path)
			if err != nil || !info.Mode().IsRegular() || m.Identity == "" || fsops.Identity(info) != m.Identity {
				return errors.New("目标视频已变化，无法登记刮削待办：" + m.Path)
			}
			m.Size, m.MTime, m.RunID = info.Size(), info.ModTime().UnixNano(), r.ID
			previous[i] = r.Mappings[i]
			r.Mappings[i] = m
			registered = append(registered, m)
			changedMappings = append(changedMappings, i)
		}
	}
	err := a.store.SaveCheckpoint(*r, positions, changedMappings, registered)
	if err != nil {
		return err
	}
	committed = true
	a.progress(*r)
	return nil
}
