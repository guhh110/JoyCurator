package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Batch struct {
	ID     string `json:"id"`
	Runs   []Run  `json:"runs"`
	Active bool   `json:"active"`
}
type BatchPreviewRequest struct {
	RecognitionID string `json:"recognitionId"`
	Revision      int    `json:"revision"`
	Mode          string `json:"mode"`
}

func (a *App) batchPreview(ctx context.Context, in BatchPreviewRequest) (*Plan, error) {
	task, err := a.recognition(in.RecognitionID)
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
		key := entryGroup(*item.Entry)
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
	plan := &Plan{ID: key(), Title: fmt.Sprintf("自动整理 · %d 部作品", len(requests)), Mode: in.Mode, Rows: []Row{}, Issues: []string{}, Groups: []*Plan{}, Expires: time.Now().Add(15 * time.Minute), RecognitionID: task.ID, Revision: task.Revision}
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
		if err := a.validateOps(plan.Ops); err != nil {
			plan.Issues = append(plan.Issues, err.Error())
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.storePlan(plan)
	return plan, nil
}
func (a *App) executeBatch(id string) (Batch, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return Batch{}, errors.New("服务正在关闭")
	}
	if a.running != "" || a.batch != "" {
		return Batch{}, errBusy
	}
	p := a.plans[id]
	if p == nil || time.Now().After(p.Expires) {
		return Batch{}, errors.New("预览已过期，请重新预览")
	}
	if len(p.Groups) == 0 || len(p.Issues) > 0 {
		return Batch{}, errors.New("请先处理预览中的冲突")
	}
	task, err := a.recognition(p.RecognitionID)
	if err != nil {
		return Batch{}, err
	}
	if task.Revision != p.Revision {
		return Batch{}, errors.New("识别结果已修改，请重新预览")
	}
	s, err := a.settings()
	if err != nil {
		return Batch{}, err
	}
	if s.Version != p.Version {
		return Batch{}, errors.New("设置已更新，请重新预览")
	}
	if err = a.validateOps(p.Ops); err != nil {
		return Batch{}, err
	}
	batch := Batch{ID: key(), Runs: []Run{}, Active: true}
	tx, err := a.db.Begin()
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback()
	for _, group := range p.Groups {
		run := Run{ID: key(), Title: group.Title, Mode: p.Mode, State: "queued", Created: time.Now().Format(time.RFC3339), Ops: append([]Work(nil), group.Ops...), Total: len(group.Ops), BatchID: batch.ID, Version: p.Version, RecognitionID: p.RecognitionID}
		if _, err = tx.Exec("INSERT INTO runs(id,value) VALUES(?,?)", run.ID, jsonText(run)); err != nil {
			return Batch{}, err
		}
		batch.Runs = append(batch.Runs, run)
	}
	if err = tx.Commit(); err != nil {
		return Batch{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.batch, a.batchCancel = batch.ID, cancel
	a.wg.Add(1)
	go a.runBatch(ctx, cancel, batch)
	delete(a.plans, id)
	batch.Runs = append([]Run(nil), batch.Runs...)
	for i := range batch.Runs {
		batch.Runs[i].Ops = nil
	}
	return batch, nil
}
func (a *App) runBatch(ctx context.Context, cancel context.CancelFunc, batch Batch) {
	defer a.wg.Done()
	defer cancel()
	defer func() { a.mu.Lock(); a.batch = ""; a.batchCancel = nil; a.mu.Unlock() }()
	for _, run := range batch.Runs {
		if ctx.Err() != nil {
			run.State, run.Error = "cancelled", "整批操作已取消，可独立重试"
			if a.isClosing() {
				run.State, run.Error = "interrupted", "服务中断，可独立重试或撤销"
			}
			_ = a.save(run)
			continue
		}
		s, err := a.settings()
		if err == nil && s.Version != run.Version {
			err = errors.New("设置已变化，请重新预览")
		}
		if err == nil {
			for _, op := range run.Ops {
				if op.Source != "" && !a.sourceAllowed(op.Source, s) {
					err = errors.New("来源已超出当前源目录")
					break
				}
			}
		}
		if err == nil {
			err = a.validateOps(run.Ops)
		}
		if err != nil {
			run.State, run.Error = "failed", err.Error()
			_ = a.save(run)
			continue
		}
		child, stop := context.WithCancel(ctx)
		a.mu.Lock()
		run.State = "running"
		if err = a.save(run); err == nil {
			a.running, a.cancel = run.ID, stop
			a.wg.Add(1)
		}
		a.mu.Unlock()
		if err != nil {
			stop()
			return
		}
		// work persists progress and releases the current-run slot. batch remains
		// reserved until the complete queue finishes, including gaps between runs.
		a.work(child, stop, run)
	}
}
func (a *App) readBatch(id string) (Batch, error) {
	rows, err := a.db.Query("SELECT value FROM runs WHERE json_extract(value,'$.batchId')=? ORDER BY rowid", id)
	if err != nil {
		return Batch{}, err
	}
	defer rows.Close()
	batch := Batch{ID: id, Runs: []Run{}}
	for rows.Next() {
		var raw string
		var run Run
		if err = rows.Scan(&raw); err != nil {
			return batch, err
		}
		if err = json.Unmarshal([]byte(raw), &run); err != nil {
			return batch, err
		}
		run.Ops = nil
		batch.Runs = append(batch.Runs, run)
	}
	if err = rows.Err(); err != nil {
		return batch, err
	}
	if len(batch.Runs) == 0 {
		return batch, errors.New("批次不存在")
	}
	a.mu.Lock()
	batch.Active = a.batch == id
	a.mu.Unlock()
	return batch, nil
}
func (a *App) batchRoutes(add func(string, endpoint)) {
	add("POST /api/batches/preview", func(w http.ResponseWriter, r *http.Request) error {
		var in BatchPreviewRequest
		if err := read(r, &in); err != nil {
			return err
		}
		plan, err := a.batchPreview(r.Context(), in)
		if err == nil {
			reply(w, plan)
		}
		return err
	})
	add("POST /api/batches/execute", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			ID string `json:"id"`
		}
		if err := read(r, &in); err != nil {
			return err
		}
		batch, err := a.executeBatch(in.ID)
		if err == nil {
			reply(w, batch)
		}
		return err
	})
	add("GET /api/batches/{id}", func(w http.ResponseWriter, r *http.Request) error {
		batch, err := a.readBatch(r.PathValue("id"))
		if err == nil {
			reply(w, batch)
		}
		return err
	})
	add("GET /api/batches", func(w http.ResponseWriter, r *http.Request) error {
		var id string
		err := a.db.QueryRow("SELECT json_extract(value,'$.batchId') FROM runs WHERE json_extract(value,'$.batchId') IS NOT NULL ORDER BY rowid DESC LIMIT 1").Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			reply(w, nil)
			return nil
		}
		if err != nil {
			return err
		}
		batch, err := a.readBatch(id)
		if err == nil {
			reply(w, batch)
		}
		return err
	})
	add("POST /api/batches/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.batch != r.PathValue("id") || a.batchCancel == nil {
			return errors.New("批次未在运行")
		}
		a.batchCancel()
		reply(w, map[string]bool{"ok": true})
		return nil
	})
}
