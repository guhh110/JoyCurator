package organizer

import (
	"context"
	"errors"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"runtime"
	"sort"
	"time"
)

func (a *Service) ScrapeBatchPreview(ctx context.Context, in ScrapeBatchPreviewRequest) (ScrapePlan, error) {
	if in.Content != "all" && in.Content != "nfo" && in.Content != "images" {
		return ScrapePlan{}, errors.New("刮削内容无效")
	}
	task, err := a.workspaces.Read(in.RecognitionID)
	if err != nil {
		return ScrapePlan{}, err
	}
	if task.Purpose != "scrape" || task.State == "running" || task.Revision != in.Revision {
		return ScrapePlan{}, errors.New("请保存刮削工作区后重新预览")
	}
	settings, err := a.settings.Load()
	if err != nil {
		return ScrapePlan{}, err
	}
	groups := map[string][]model.MediaMapping{}
	count := 0
	for _, it := range task.Items {
		if !it.Selected || it.Removed {
			continue
		}
		if it.Execution != nil && (it.Execution.Locked || it.Execution.Completed) {
			return ScrapePlan{}, errors.New("选择中包含已提交视频")
		}
		if !it.Confirmed() || it.State != "ready" || it.Entry == nil {
			return ScrapePlan{}, errors.New("选择中包含待确认视频")
		}
		if err = matching.ValidateLayout(it.Layout, it.Path, it.Entry.Type, settings.TargetRoot); err != nil {
			return ScrapePlan{}, fmt.Errorf("%s：%w", it.Path, err)
		}
		m := model.MediaMapping{Entry: it.Entry, Path: it.Path, Source: it.Path, Type: it.Entry.Type, TMDBID: it.Entry.TMDBID, Season: it.Season, Episodes: it.Episodes, State: "pending", Identity: it.Identity, Size: it.Size, MTime: it.MTime, Layout: it.Layout, Original: it.Original}
		if m.Original != nil {
			m.RunID = m.Original.RunID
			m.Source = m.Original.Source
		}
		if err = a.validateScrapeMapping(m, ""); err != nil {
			return ScrapePlan{}, err
		}
		key := matching.EntryGroup(*it.Entry) + ":" + it.Layout.WorkDir
		groups[key] = append(groups[key], m)
		count++
	}
	if count < 1 || count > 1000 {
		return ScrapePlan{}, errors.New("每次刮削 1–1000 个视频")
	}
	p := &Plan{ID: model.Key(), Purpose: "scrape", Title: fmt.Sprintf("目录刮削 · %d 部作品", len(groups)), RecognitionID: task.ID, Revision: task.Revision, Version: settings.Version, Expires: time.Now().Add(previewTTL), Groups: []*Plan{}, Issues: []string{}}
	result := ScrapePlan{ID: p.ID, Title: p.Title, Content: in.Content, OnlyMissing: in.OnlyMissing, Groups: []ScrapePlan{}, Targets: []string{}, Artifacts: []string{}, Skipped: []string{}, Issues: []string{}}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Include skipped destinations: another work cannot silently claim an existing shared file.
	claimed := map[string]string{}
	for _, key := range keys {
		items := groups[key]
		paths := []string{}
		for _, it := range items {
			paths = append(paths, it.Path)
		}
		group, e := a.scrapePreviewItems(ctx, ScrapePreviewRequest{Paths: paths, Content: in.Content, OnlyMissing: in.OnlyMissing}, items)
		if e != nil {
			return ScrapePlan{}, e
		}
		plan := a.previews.get(group.ID)
		if plan == nil {
			return ScrapePlan{}, errors.New("刮削预览失效")
		}
		a.previews.remove(group.ID)
		for _, path := range append(append([]string{}, group.Artifacts...), group.Skipped...) {
			k := pathKey(path)
			if previous, ok := claimed[k]; ok && previous != key {
				p.Issues = append(p.Issues, "作品之间目标重复："+path)
			}
			claimed[k] = key
		}
		p.Inputs = append(p.Inputs, plan.Inputs...)
		p.Groups = append(p.Groups, plan)
		p.Ops = append(p.Ops, plan.Ops...)
		p.Mappings = append(p.Mappings, plan.Mappings...)
		p.Issues = append(p.Issues, group.Issues...)
		p.Required += group.Required
		result.Groups = append(result.Groups, group)
		result.Targets = append(result.Targets, group.Targets...)
		result.Artifacts = append(result.Artifacts, group.Artifacts...)
		result.Skipped = append(result.Skipped, group.Skipped...)
	}
	// validateOps checks cumulative device capacity across all groups.
	if len(p.Ops) > 0 {
		if err = a.validateOps(p.Ops); err != nil {
			p.Issues = append(p.Issues, err.Error())
		}
	}
	a.coordinator.mu.Lock()
	err = a.checkReservationsLocked([]Run{{Purpose: "scrape", Mappings: p.Mappings}})
	a.coordinator.mu.Unlock()
	if err != nil {
		p.Issues = append(p.Issues, err.Error())
	}
	result.Required = p.Required
	result.Issues = p.Issues
	sort.Strings(result.Artifacts)
	sort.Strings(result.Skipped)
	a.previews.put(p)
	return result, nil
}
func (a *Service) ScrapeExecuteBatch(id string) (Batch, error) {
	if runtime.GOOS == "windows" {
		return Batch{}, errors.New("Windows 暂不支持执行刮削")
	}
	p := a.previews.get(id)
	if p == nil || p.Purpose != "scrape" || len(p.Groups) == 0 || time.Now().After(p.Expires) {
		return Batch{}, errors.New("刮削预览已过期")
	}
	if len(p.Issues) > 0 || len(p.Ops) == 0 {
		return Batch{}, errors.New("预览有冲突或没有需要生成的文件")
	}
	// Keep revision and queue admission atomic with respect to workspace edits.
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.previews.get(id) != p {
		return Batch{}, errors.New("预览已失效或提交")
	}
	s, err := a.settings.Load()
	if err != nil {
		return Batch{}, err
	}
	revision, err := a.revision(p.RecognitionID)
	if err != nil {
		return Batch{}, err
	}
	if s.Version != p.Version || revision != p.Revision {
		return Batch{}, errors.New("设置或工作区已变化，请重新预览")
	}
	if err = a.validateScrapeRun(Run{Mappings: p.Inputs}, s); err != nil {
		return Batch{}, err
	}
	if err = a.checkConfirmations(p); err != nil {
		return Batch{}, err
	}
	batch := Batch{ID: model.Key(), Runs: []Run{}, Active: true, State: "queued"}
	for _, g := range p.Groups {
		if len(g.Ops) == 0 {
			continue
		}
		r := Run{ID: model.Key(), Title: g.Title, Mode: "scrape", State: "queued", Created: time.Now().Format(time.RFC3339), Purpose: "scrape", Version: p.Version, RecognitionID: p.RecognitionID, BatchID: batch.ID, Ops: append([]Work{}, g.Ops...), Mappings: append([]model.MediaMapping{}, g.Mappings...), Total: len(g.Ops)}
		if err = a.validateScrapeRun(r, s); err != nil {
			return Batch{}, err
		}
		batch.Runs = append(batch.Runs, r)
	}
	if err = a.enqueueLocked(batch.Runs); err != nil {
		return Batch{}, err
	}
	a.previews.remove(id)
	for i := range batch.Runs {
		batch.Runs[i] = summary(batch.Runs[i])
	}
	return batch, nil
}
