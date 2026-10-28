package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"slices"
	"sort"
	"time"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/scanner"
	"joycurator/internal/tmdb"
)

type RecognitionItem struct {
	File
	Parsed     model.Parsed  `json:"parsed"`
	Group      string        `json:"group"`
	Entry      *model.Entry  `json:"entry"`
	Candidates []model.Entry `json:"candidates"`
	State      string        `json:"state"`
	Reason     string        `json:"reason"`
	Selected   bool          `json:"selected"`
	Removed    bool          `json:"removed"`
	Edited     bool          `json:"edited,omitempty"`
	TraceID    string        `json:"traceId,omitempty"`
}
type Recognition struct {
	ID        string            `json:"id"`
	Revision  int               `json:"revision"`
	State     string            `json:"state"`
	Paths     []string          `json:"paths"`
	Items     []RecognitionItem `json:"items"`
	Completed int               `json:"completed"`
	Total     int               `json:"total"`
	Error     string            `json:"error"`
	Created   string            `json:"created"`
}
type RecognitionEdit struct {
	Path     string `json:"path"`
	Group    string `json:"group"`
	Type     string `json:"type"`
	TMDBID   int    `json:"tmdbId"`
	Season   int    `json:"season"`
	Episodes []int  `json:"episodes"`
	Selected bool   `json:"selected"`
	Removed  bool   `json:"removed"`
}
type RecognitionUpdate struct {
	Revision int               `json:"revision"`
	Items    []RecognitionEdit `json:"items"`
}

func (a *App) recognition(id string) (Recognition, error) {
	var raw string
	var task Recognition
	err := a.db.QueryRow("SELECT value FROM recognitions WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return task, errors.New("识别任务不存在")
	}
	if err != nil {
		return task, err
	}
	err = json.Unmarshal([]byte(raw), &task)
	return task, err
}
func (a *App) saveRecognition(task *Recognition) error {
	task.Revision++
	_, err := a.db.Exec("INSERT INTO recognitions(id,value) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", task.ID, jsonText(task))
	return err
}
func (a *App) startRecognition(paths []string, retryID string) (Recognition, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return Recognition{}, errors.New("服务正在关闭")
	}
	if a.batch != "" || a.running != "" {
		return Recognition{}, errBusy
	}
	if len(a.recognitionCancels) > 0 {
		return Recognition{}, errors.New("已有识别任务运行中，请等待完成或取消")
	}
	s, err := a.settings()
	if err != nil {
		return Recognition{}, err
	}
	client, err := a.client(s)
	if err != nil {
		return Recognition{}, err
	}
	if s.Token == "" {
		return Recognition{}, errors.New("请先配置 TMDB API Read Access Token")
	}
	task := Recognition{ID: key(), State: "running", Paths: paths, Items: []RecognitionItem{}, Created: time.Now().Format(time.RFC3339)}
	if retryID != "" {
		task, err = a.recognition(retryID)
		if err != nil {
			return task, err
		}
		if task.State == "running" {
			return task, errors.New("识别任务仍在运行")
		}
		task.State, task.Error = "running", ""
		if len(paths) > 0 {
			seen := map[string]bool{}
			for _, path := range task.Paths {
				seen[path] = true
			}
			for _, path := range paths {
				path = a.access.Canonical(path)
				if !seen[path] {
					task.Paths = append(task.Paths, path)
					seen[path] = true
				}
			}
		}
		for i := range task.Items {
			if task.Items[i].State == "failed" {
				task.Items[i].State = "pending"
			}
		}
	}
	remainingPaths := []string{}
	for _, path := range task.Paths {
		_, statErr := a.access.Stat(path)
		completed := false
		for _, item := range task.Items {
			if item.Path == a.access.Canonical(path) && item.Removed {
				completed = true
				break
			}
		}
		if completed && errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		remainingPaths = append(remainingPaths, path)
	}
	task.Paths = remainingPaths
	if len(task.Paths) == 0 || len(task.Paths) > 200 {
		return task, errors.New("请选择 1–200 个文件或目录")
	}
	for _, path := range task.Paths {
		if !a.sourceAllowed(path, s) {
			return task, errors.New("只能添加源目录内的文件或文件夹，目标目录不能作为来源")
		}
	}
	if err = a.saveRecognition(&task); err != nil {
		return task, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.recognitionCancels[task.ID] = cancel
	a.wg.Add(1)
	var workerTask Recognition
	_ = json.Unmarshal([]byte(jsonText(task)), &workerTask)
	go a.recognize(ctx, cancel, workerTask, s, client)
	return task, nil
}
func (a *App) recognize(ctx context.Context, cancel context.CancelFunc, task Recognition, s model.Settings, client *tmdb.Client) {
	defer a.wg.Done()
	defer cancel()
	defer func() { a.mu.Lock(); delete(a.recognitionCancels, task.ID); a.mu.Unlock() }()
	persist := func() error { a.mu.Lock(); defer a.mu.Unlock(); return a.saveRecognition(&task) }
	defer func() {
		if task.State == "running" {
			task.State, task.Error = "failed", "无法保存识别进度，请重试"
			_ = persist()
		}
	}()
	// A cancelled scan can be resumed without losing already resolved files.
	previous := map[string]RecognitionItem{}
	for _, item := range task.Items {
		previous[item.Path] = item
	}
	scanned := []RecognitionItem{}
	scanErr := scanner.Walk(ctx, a.access, task.Paths, []string{s.TargetRoot}, func(path string, info fs.FileInfo) error {
		if len(scanned) >= 2000 {
			return errors.New("单次最多导入 2000 个视频，请分批添加")
		}
		if !a.sourceAllowed(path, s) {
			return errors.New("扫描文件超出源目录范围")
		}
		if item, ok := previous[path]; ok && item.Size == info.Size() && item.MTime == info.ModTime().UnixNano() && (item.State != "pending" || item.Edited) {
			scanned = append(scanned, item)
			return nil
		}
		extracted, traceID, extractErr := a.extractFile(ctx, path, s.SourceRoot, task.ID)
		if extractErr != nil {
			return extractErr
		}
		p := parsedMetadata(extracted.Metadata)
		group := fmt.Sprintf("query:%s:%s:%d:%d", p.Type, recognition.NormalizeTitle(p.Title), p.Year, p.TMDBID)
		if p.Title == "" && p.TMDBID == 0 {
			group = "path:" + path
		}
		scanned = append(scanned, RecognitionItem{File: File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano(), Season: p.Season, Episodes: p.Episodes}, Parsed: p, Group: group, Candidates: []model.Entry{}, State: "pending", TraceID: traceID})
		return nil
	})
	if scanErr != nil {
		task.State, task.Error = "failed", scanErr.Error()
		if ctx.Err() != nil {
			task.State, task.Error = "cancelled", "识别已取消，可重试"
			if a.isClosing() {
				task.State, task.Error = "interrupted", "服务中断，可重试未完成项"
			}
		}
		_ = persist()
		return
	}
	sort.Slice(scanned, func(i, j int) bool { return scanned[i].Path < scanned[j].Path })
	task.Items, task.Total, task.Completed = scanned, len(scanned), 0
	if err := persist(); err != nil {
		return
	}
	memo := newRecognitionMemo(client)
	resolved := map[string]RecognitionItem{}
	for i := range task.Items {
		if ctx.Err() != nil {
			break
		}
		item := &task.Items[i]
		if item.State == "pending" {
			if item.Entry == nil {
				group := item.Group
				match, ok := resolved[group]
				if !ok {
					match = *item
					memo.match(ctx, &match)
					resolved[group] = match
				}
				item.Entry, item.Candidates, item.State, item.Reason = match.Entry, match.Candidates, match.State, match.Reason
				if item.Entry != nil {
					item.Group = entryGroup(*item.Entry)
				}
			}
			if item.Entry != nil {
				memo.validate(ctx, item)
			}
			if !item.Edited {
				item.Selected = item.State == "ready" && !item.Removed
			} else if item.State != "ready" || item.Removed {
				item.Selected = false
			}
		}
		task.Completed++
		// Bound write amplification for large groups while still exposing progress.
		if i%10 == 0 || i+1 == len(task.Items) {
			if err := persist(); err != nil {
				return
			}
		}
	}
	task.State = "completed"
	if ctx.Err() != nil {
		task.State, task.Error = "cancelled", "识别已取消，可重试未完成项"
		if a.isClosing() {
			task.State, task.Error = "interrupted", "服务中断，可重试未完成项"
		}
	}
	_ = persist()
}
func entryGroup(entry model.Entry) string { return fmt.Sprintf("tmdb:%s:%d", entry.Type, entry.TMDBID) }

func (a *App) updateRecognition(ctx context.Context, id string, in RecognitionUpdate) (Recognition, error) {
	// Network validation happens outside the application mutex. Revision is
	// checked again when committing so two browser tabs cannot lose edits.
	task, err := a.recognition(id)
	if err != nil {
		return task, err
	}
	if task.State == "running" {
		return task, errors.New("请等待识别完成后修正")
	}
	if task.Revision != in.Revision {
		return task, errors.New("识别结果已更新，请重新加载")
	}
	if len(in.Items) != len(task.Items) {
		return task, errors.New("文件列表已变化，请重新加载")
	}
	s, err := a.settings()
	if err != nil {
		return task, err
	}
	client, err := a.client(s)
	if err != nil {
		return task, err
	}
	edits := map[string]RecognitionEdit{}
	for _, edit := range in.Items {
		if _, ok := edits[edit.Path]; ok {
			return task, errors.New("文件重复")
		}
		if len(edit.Group) > 1024 {
			return task, errors.New("分组名称过长")
		}
		edits[edit.Path] = edit
	}
	memo := newRecognitionMemo(client)
	for i := range task.Items {
		item := &task.Items[i]
		edit, ok := edits[item.Path]
		if !ok {
			return task, errors.New("文件列表已变化")
		}
		if !edit.Removed && edit.Selected && !a.sourceAllowed(item.Path, s) {
			return task, errors.New("文件已不在当前源目录内，请重新导入")
		}
		if edit.TMDBID < 0 || (edit.TMDBID > 0 && edit.Type != "movie" && edit.Type != "tv") {
			return task, errors.New("作品类型或 ID 无效")
		}
		oldID := 0
		oldType := ""
		if item.Entry != nil {
			oldID, oldType = item.Entry.TMDBID, item.Entry.Type
		}
		if edit.TMDBID != oldID || edit.Type != oldType || edit.Group != item.Group || edit.Season != item.Season || !slices.Equal(edit.Episodes, item.Episodes) || edit.Selected != item.Selected || edit.Removed != item.Removed {
			item.Edited = true
		}
		if edit.TMDBID > 0 {
			changed := item.Entry == nil || item.Entry.TMDBID != edit.TMDBID || item.Entry.Type != edit.Type
			if changed {
				entry, e := memo.detail(ctx, edit.Type, edit.TMDBID)
				if e != nil {
					return task, e
				}
				item.Entry = entry
				item.Reason = "已手动选择作品"
			}
			item.Season, item.Episodes = edit.Season, edit.Episodes
			memo.validate(ctx, item)
		} else if item.Entry != nil || edit.Group != item.Group {
			item.Entry = nil
			item.State, item.Reason = "review", "请手动选择作品"
		}
		if edit.Group != "" {
			item.Group = edit.Group
		} else if item.Entry != nil {
			item.Group = entryGroup(*item.Entry)
		}
		item.Removed, item.Selected = edit.Removed, edit.Selected && item.State == "ready" && !edit.Removed
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.batch != "" || a.running != "" {
		return task, errBusy
	}
	current, err := a.recognition(id)
	if err != nil {
		return task, err
	}
	if current.Revision != in.Revision {
		return current, errors.New("识别结果已更新，请重新加载")
	}
	err = a.saveRecognition(&task)
	return task, err
}
func (a *App) recognitionRoutes(add func(string, endpoint)) {
	add("GET /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		var raw string
		err := a.db.QueryRow("SELECT value FROM recognitions ORDER BY rowid DESC LIMIT 1").Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			reply(w, nil)
			return nil
		}
		if err != nil {
			return err
		}
		var task Recognition
		if err = json.Unmarshal([]byte(raw), &task); err != nil {
			return err
		}
		reply(w, task)
		return nil
	})
	add("POST /api/recognitions", func(w http.ResponseWriter, r *http.Request) error {
		var in struct {
			Paths []string `json:"paths"`
			ID    string   `json:"id"`
		}
		if err := read(r, &in); err != nil {
			return err
		}
		if len(in.Paths) == 0 {
			return errors.New("请选择文件或目录")
		}
		task, err := a.startRecognition(in.Paths, in.ID)
		if err == nil {
			reply(w, task)
		}
		return err
	})
	add("GET /api/recognitions/{id}", func(w http.ResponseWriter, r *http.Request) error {
		task, err := a.recognition(r.PathValue("id"))
		if err == nil {
			reply(w, task)
		}
		return err
	})
	add("PUT /api/recognitions/{id}", func(w http.ResponseWriter, r *http.Request) error {
		var in RecognitionUpdate
		if err := read(r, &in); err != nil {
			return err
		}
		task, err := a.updateRecognition(r.Context(), r.PathValue("id"), in)
		if err == nil {
			reply(w, task)
		}
		return err
	})
	add("POST /api/recognitions/{id}/retry", func(w http.ResponseWriter, r *http.Request) error {
		task, err := a.startRecognition(nil, r.PathValue("id"))
		if err == nil {
			reply(w, task)
		}
		return err
	})
	add("POST /api/recognitions/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		cancel := a.recognitionCancels[r.PathValue("id")]
		if cancel == nil {
			return errors.New("识别任务未在运行")
		}
		cancel()
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	a.batchRoutes(add)
}

// Completed videos leave the pending workspace even when a later sidecar fails.
// Their independent run remains available for retry and undo.
func (a *App) finishRecognitionFiles(run Run) {
	completed := map[string]bool{}
	for _, op := range run.Ops {
		if op.Kind == "video" && op.Status == "completed" {
			completed[op.Source] = true
		}
	}
	if len(completed) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	task, err := a.recognition(run.RecognitionID)
	if err != nil {
		return
	}
	changed := false
	for i := range task.Items {
		if completed[task.Items[i].Path] && !task.Items[i].Removed {
			task.Items[i].Removed, task.Items[i].Selected = true, false
			changed = true
		}
	}
	if changed {
		_ = a.saveRecognition(&task)
	}
}
