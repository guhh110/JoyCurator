package planning

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
	if task.State == "running" || task.Revision != in.Revision {
		return nil, errors.New("识别结果已变化，请保存并重新预览")
	}
	requests := [][]matching.Item{}
	indexes := map[string]int{}
	count := 0
	for _, item := range task.Items {
		if !item.Selected || item.Removed {
			continue
		}
		if item.State != "ready" || item.Entry == nil {
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
	plan := &Plan{ID: model.Key(), Title: fmt.Sprintf("自动整理 · %d 部作品", len(requests)), Mode: in.Mode, DuplicatePolicy: in.DuplicatePolicy, Rows: []Row{}, Issues: []string{}, Groups: []*Plan{}, Expires: time.Now().Add(15 * time.Minute), RecognitionID: task.ID, Revision: task.Revision}
	targets := map[string]bool{}
	for _, request := range requests {
		group, err := a.buildPreview(ctx, request, in.Mode, in.DuplicatePolicy)
		if err != nil {
			return nil, err
		}
		if plan.Version == 0 {
			plan.Version = group.Version
		} else if plan.Version != group.Version {
			return nil, errors.New("设置已变化，请重新预览")
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
		plan.Purpose = "organize"
		plan.Required += group.Required
		plan.Issues = append(plan.Issues, group.Issues...)
	}
	if len(plan.Issues) == 0 {
		if err := a.ValidateOps(plan.Ops); err != nil {
			plan.Issues = append(plan.Issues, err.Error())
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.storePlan(plan)
	return plan, nil
}
