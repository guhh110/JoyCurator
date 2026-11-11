package organizer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
)

func (a *Service) BatchPreview(ctx context.Context, in BatchPreviewRequest) (*Plan, error) {
	task, err := a.workspaces.Read(in.RecognitionID)
	if err != nil {
		return nil, err
	}
	if task.Purpose != "organize" {
		return nil, errors.New("刮削工作区不能提交整理")
	}
	if task.State == "running" || task.Revision != in.Revision {
		return nil, errors.New("识别结果已变化，请保存并重新预览")
	}
	settings, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	requests := [][]matching.Item{}
	indexes := map[string]int{}
	count := 0
	for _, item := range task.Items {
		if !item.Selected || item.Removed || item.Execution != nil && (item.Execution.Locked || item.Execution.Completed) {
			continue
		}
		if !item.Confirmed() || item.State != "ready" || item.Entry == nil {
			return nil, errors.New("选择中包含待确认文件")
		}
		count++
		key := matching.EntryGroup(*item.Entry)
		index, ok := indexes[key]
		if !ok {
			index = len(requests)
			indexes[key] = index
			requests = append(requests, []matching.Item{})
		}
		requests[index] = append(requests[index], item)
	}
	if count < 1 || count > 1000 {
		return nil, errors.New("每次整理 1–1000 个视频")
	}
	plan := &Plan{ID: model.Key(), Title: fmt.Sprintf("自动整理 · %d 部作品", len(requests)), Mode: in.Mode, DuplicatePolicy: in.DuplicatePolicy, Rows: []Row{}, Issues: []string{}, Groups: []*Plan{}, Expires: time.Now().Add(previewTTL), RecognitionID: task.ID, Revision: task.Revision, Version: settings.Version, Purpose: "organize"}
	targets := map[string]bool{}
	for _, request := range requests {
		group, err := a.buildPreview(ctx, request, in.Mode, in.DuplicatePolicy, settings)
		if err != nil {
			return nil, err
		}
		for _, op := range group.Ops {
			target := strings.ToLower(op.Target)
			if targets[target] {
				plan.Issues = append(plan.Issues, "作品之间目标重复："+op.Target)
			}
			targets[target] = true
		}
		for _, row := range group.Rows {
			if row.Action != "skip" {
				continue
			}
			target := strings.ToLower(row.Target)
			if targets[target] {
				plan.Issues = append(plan.Issues, "作品之间目标重复："+row.Target)
			}
			targets[target] = true
		}
		plan.Groups = append(plan.Groups, group)
		plan.Rows = append(plan.Rows, group.Rows...)
		plan.Ops = append(plan.Ops, group.Ops...)
		plan.Mappings = append(plan.Mappings, group.Mappings...)
		plan.Required += group.Required
		plan.Issues = append(plan.Issues, group.Issues...)
	}
	inferMappingLayouts(plan.Mappings, settings.TargetRoot)
	layouts := map[string]model.MediaLayout{}
	for _, m := range plan.Mappings {
		layouts[m.Path] = m.Layout
	}
	for _, group := range plan.Groups {
		for i := range group.Mappings {
			group.Mappings[i].Layout = layouts[group.Mappings[i].Path]
		}
	}
	if len(plan.Issues) == 0 {
		if err := a.validateOps(plan.Ops); err != nil {
			plan.Issues = append(plan.Issues, err.Error())
		}
	}
	a.coordinator.mu.Lock()
	reservationErr := a.checkReservationsLocked([]Run{{Purpose: "organize", RecognitionID: plan.RecognitionID, Mappings: plan.Mappings, Ops: plan.Ops}})
	a.coordinator.mu.Unlock()
	if reservationErr != nil {
		plan.Issues = append(plan.Issues, reservationErr.Error())
	}
	a.previews.put(plan)
	return plan, nil
}
