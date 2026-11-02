package organizer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"joycurator/internal/matching"
	"joycurator/internal/model"
)

func summary(r Run) Run          { r.Ops = nil; r.Mappings = nil; return r }
func pathKey(path string) string { return strings.ToLower(filepath.Clean(path)) }

func organizeTargets(r Run) map[string]bool {
	targets := map[string]bool{}
	for _, op := range r.Ops {
		targets[pathKey(op.Target)] = true
	}
	for _, m := range r.Mappings {
		targets[pathKey(m.Path)] = true
	}
	return targets
}

// Start restores durable waiting tasks. Workers own their Run snapshots.
func (a *Service) Start() error {
	c := a.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	if c.closed {
		return errors.New("服务正在关闭")
	}
	runs, err := a.store.QueuedRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		if r.State != "queued" || c.lanes[r.Purpose] == nil {
			return errors.New("队列与任务状态不一致")
		}
		c.lanes[r.Purpose].pending = append(c.lanes[r.Purpose].pending, r)
	}
	c.started = true
	for _, purpose := range []string{"organize", "scrape"} {
		c.wg.Add(1)
		go a.drain(purpose)
	}
	return nil
}
func (a *Service) enqueueLocked(runs []Run) error {
	c := a.coordinator
	if c.closed {
		return errors.New("服务正在关闭")
	}
	if !c.started {
		return errors.New("任务队列尚未启动")
	}
	if err := a.checkReservationsLocked(runs); err != nil {
		return err
	}
	if err := a.store.Enqueue(runs); err != nil {
		return err
	}
	for _, r := range runs {
		c.lanes[r.Purpose].pending = append(c.lanes[r.Purpose].pending, r)
	}
	c.signalLocked()
	return nil
}
func (a *Service) checkReservationsLocked(incoming []Run) error {
	workspaces := map[string]map[string]matching.FileExecution{}
	for _, r := range incoming {
		if r.Purpose == "organize" {
			states, ok := workspaces[r.RecognitionID]
			if !ok {
				var err error
				states, err = a.store.FileExecutions(r.RecognitionID)
				if err != nil {
					return err
				}
				workspaces[r.RecognitionID] = states
			}
			for _, m := range r.Mappings {
				if state := states[m.Source]; state.ManualReview && state.RunID != r.ID {
					return errors.New("来源文件状态不明，请先人工核对：" + m.Source)
				}
				if state := states[m.Source]; state.Completed && state.RunID != r.ID {
					return errors.New("文件已由任务 " + state.RunID + " 整理完成：" + m.Source)
				}
			}
		}
		for _, lane := range a.coordinator.lanes {
			previous := lane.pending
			if lane.active != nil {
				previous = append(append([]Run{}, previous...), *lane.active)
			}
			for _, other := range previous {
				if other.ID == r.ID {
					return errors.New("任务已在队列中")
				}
				if r.Purpose != other.Purpose {
					continue
				}
				if r.Purpose == "organize" {
					targets := organizeTargets(other)
					if lane.active != nil && other.ID == lane.active.ID {
						targets = lane.targets
					}
					for target := range organizeTargets(r) {
						if targets[target] {
							return fmt.Errorf("目标已由任务 %s 占用：%s", other.ID, target)
						}
					}
				}
				for _, m := range r.Mappings {
					for _, old := range other.Mappings {
						if r.Purpose == "organize" && (pathKey(m.Source) == pathKey(old.Source) || pathKey(m.Path) == pathKey(old.Path)) {
							return fmt.Errorf("文件已由任务 %s 占用：%s", other.ID, m.Source)
						}
						if r.Purpose == "scrape" && pathKey(m.Path) == pathKey(old.Path) {
							return fmt.Errorf("视频已有刮削任务 %s：%s", other.ID, m.Path)
						}
					}
				}
			}
		}
	}
	return nil
}

// Claims are acquired together. A completed, durably registered video no longer
// needs a write claim, allowing the scrape lane to consume it immediately.
func claimsFor(r Run) map[string]bool {
	result := map[string]bool{}
	add := func(path string, write bool) {
		if path != "" {
			key := pathKey(path)
			result[key] = result[key] || write
		}
	}
	if r.Purpose == "scrape" {
		for _, m := range r.Mappings {
			add(m.Path, false)
		}
	}
	for _, op := range r.Ops {
		if r.Undo {
			if op.Status == "undone" || op.Status == "retained" {
				continue
			}
			add(op.Target, true)
			if op.Mode == "move" {
				add(op.Source, true)
			}
			continue
		}
		registered := op.Kind != "video"
		if !registered {
			for _, m := range r.Mappings {
				if m.Path == op.Target && m.RunID == r.ID && m.Identity != "" {
					registered = true
					break
				}
			}
		}
		if op.Status == "completed" && registered {
			if r.Purpose == "scrape" {
				add(op.Target, false)
			}
			continue
		}
		add(op.Target, true)
		add(op.Source, op.Mode == "move")
	}
	if r.Purpose == "organize" && !r.Undo {
		for _, m := range r.Mappings {
			if m.RunID != r.ID && m.Identity != "" {
				add(m.Path, true)
			}
		}
	}
	return result
}
func (c *Coordinator) conflictLocked(purpose string, claims map[string]bool) string {
	for other, lane := range c.lanes {
		if other == purpose || lane.active == nil {
			continue
		}
		for path, write := range claims {
			if old, ok := lane.claims[path]; ok && (write || old) {
				return "等待任务「" + lane.active.Title + "」释放文件"
			}
		}
	}
	return ""
}
func (a *Service) progress(r Run) {
	c := a.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	lane := c.lanes[r.Purpose]
	if lane.active != nil && lane.active.ID == r.ID {
		// Only mappings are retained for admission; operation arrays belong to the worker.
		value := summary(r)
		value.Mappings = append([]model.MediaMapping(nil), r.Mappings...)
		lane.active = &value
		lane.claims = claimsFor(r)
		c.signalLocked()
	}
}
func (a *Service) drain(purpose string) {
	c := a.coordinator
	defer c.wg.Done()
	for {
		c.mu.Lock()
		lane := c.lanes[purpose]
		if c.closed {
			c.mu.Unlock()
			return
		}
		changed := c.changed
		if len(lane.pending) == 0 {
			lane.wait = ""
			c.mu.Unlock()
			<-changed
			continue
		}
		r := lane.pending[0]
		claims := claimsFor(r)
		if reason := c.conflictLocked(purpose, claims); reason != "" {
			lane.wait = reason
			c.mu.Unlock()
			<-changed
			continue
		}
		lane.wait = ""
		ctx, cancel := context.WithCancel(context.Background())
		r.State = "running"
		if err := a.store.SaveCheckpoint(r, nil, nil, nil); err != nil {
			lane.wait = "无法启动任务：" + err.Error()
			cancel()
			c.mu.Unlock()
			<-changed
			continue
		}
		lane.pending = lane.pending[1:]
		active := summary(r)
		active.Mappings = append([]model.MediaMapping(nil), r.Mappings...)
		lane.active = &active
		lane.cancel = cancel
		lane.claims = claims
		lane.targets = organizeTargets(r)
		c.signalLocked()
		c.mu.Unlock()
		if err := a.validateQueued(ctx, r); err != nil {
			r.State = "failed"
			r.Error = err.Error()
		} else {
			r = a.work(ctx, r)
		}
		if ctx.Err() != nil && r.State != "completed" && r.State != "undone" {
			r.State = "cancelled"
			r.Error = "操作已取消，可重试"
		}
		if c.IsClosing() && r.State == "cancelled" {
			r.State = "interrupted"
			r.Error = "服务中断，可重试或撤销"
		}
		cancel()
		c.mu.Lock()
		if err := a.store.FinishQueued(r); err != nil {
			lane.wait = "无法保存任务结果：" + err.Error()
			c.signalLocked()
			c.mu.Unlock()
			return
		}
		lane.active = nil
		lane.cancel = nil
		lane.claims = nil
		lane.targets = nil
		c.signalLocked()
		c.mu.Unlock()
	}
}
func (a *Service) validateQueued(ctx context.Context, r Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s, err := a.settings.Load()
	if err != nil {
		return err
	}
	if s.Version != r.Version {
		return errors.New("设置已变化，请重新预览")
	}
	if r.Purpose == "organize" {
		pending := []Work{}
		for _, op := range r.Ops {
			if !withinMedia(s.TargetRoot, op.Target) || (op.Source != "" && !withinMedia(s.SourceRoot, op.Source)) {
				return errors.New("整理路径已超出当前媒体目录")
			}
			if !r.Undo && op.Status == "pending" {
				if !a.sources.SourceAllowed(op.Source, s) {
					return errors.New("来源已变化，请重新添加")
				}
				pending = append(pending, op)
			}
		}
		if !r.Undo {
			if err := a.validateOps(pending); err != nil {
				return err
			}
			return a.validateSkipped(r.Mappings, r.Ops)
		}
		return a.validateUndoSpace(r.Ops)
	}
	return a.validateScrapeRun(r, s)
}
func (a *Service) Queue() QueueStatus {
	c := a.coordinator
	c.mu.Lock()
	defer c.mu.Unlock()
	result := QueueStatus{Active: c.BusyLocked(), Revision: c.revision, Recognizing: c.RecognitionActiveLocked()}
	for purpose, lane := range c.lanes {
		view := LaneStatus{Queued: len(lane.pending), WaitReason: lane.wait}
		if lane.active != nil {
			r := summary(*lane.active)
			view.Current = &r
		}
		batches := map[string]bool{}
		for _, r := range lane.pending {
			if r.BatchID != "" {
				batches[r.BatchID] = true
			}
		}
		view.QueuedBatches = len(batches)
		if purpose == "organize" {
			result.Organize = view
		} else {
			result.Scrape = view
		}
	}
	return result
}
func (a *Service) QueuePage(purpose string, page int) (RunPage, error) {
	return a.store.QueuePage(purpose, page)
}
func (a *Service) Batches(page int) (BatchPage, error) { return a.store.Batches(page) }
