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
	requests := []PreviewRequest{}
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
			requests = append(requests, PreviewRequest{Type: item.Entry.Type, TMDBID: item.Entry.TMDBID, Mode: in.Mode})
		}
		requests[index].Files = append(requests[index].Files, item.File)
	}
	if count < 1 || count > 1000 {
		return nil, errors.New("每次整理 1–1000 个视频")
	}
	plan := &Plan{ID: model.Key(), Title: fmt.Sprintf("自动整理 · %d 部作品", len(requests)), Mode: in.Mode, Rows: []Row{}, Issues: []string{}, Groups: []*Plan{}, Expires: time.Now().Add(15 * time.Minute), RecognitionID: task.ID, Revision: task.Revision}
	targets := map[string]bool{}
	for _, request := range requests {
		group, err := a.buildPreview(ctx, request)
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
		plan.Groups = append(plan.Groups, group)
		plan.Rows = append(plan.Rows, group.Rows...)
		plan.Ops = append(plan.Ops, group.Ops...)
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
