package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/recognition"
)

type CandidateRequest struct {
	Revision int      `json:"revision"`
	Paths    []string `json:"paths"`
}
type CandidateView struct {
	Path       string                 `json:"path"`
	TraceID    string                 `json:"traceId"`
	Document   recognition.Document   `json:"document"`
	Suggestion recognition.FileResult `json:"suggestion"`
}

func (a *WorkspaceService) evidence(task matching.Workspace, paths []string) ([]CandidateView, error) {
	if len(paths) == 0 || len(paths) > 2000 {
		return nil, errors.New("请选择 1–2000 个文件")
	}
	items := map[string]matching.Item{}
	for _, it := range task.Items {
		items[it.Path] = it
	}
	cache := map[string]recognition.BatchResult{}
	out := []CandidateView{}
	seen := map[string]bool{}
	for _, path := range paths {
		it, ok := items[path]
		if !ok || it.Removed || seen[path] {
			return nil, errors.New("文件不在工作区或重复")
		}
		seen[path] = true
		v := CandidateView{Path: path, TraceID: it.TraceID, Document: recognition.Document{Path: path, Sources: []recognition.TextSource{}, Tokens: []recognition.Token{}, Candidates: []recognition.Candidate{}}, Suggestion: recognition.FileResult{Path: path, Metadata: it.Metadata}}
		if it.TraceID != "" {
			batch, ok := cache[it.TraceID]
			if !ok {
				_, steps, err := a.extract.Detail(it.TraceID)
				if err != nil {
					return nil, err
				}
				batch, err = recognition.RestoreBatch(steps)
				if err != nil {
					return nil, err
				}
				cache[it.TraceID] = batch
			}
			found := false
			for _, d := range batch.Documents {
				if d.Path == path {
					v.Document = d
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("文件缺少候选快照")
			}
			for _, f := range batch.Files {
				if f.Path == path {
					v.Suggestion = f
					break
				}
			}
		} else {
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			v.Document.Sources = append(v.Document.Sources, recognition.TextSource{ID: "name", Kind: recognition.Filename, Text: name})
			s, err := a.settings.Load()
			if err != nil {
				return nil, err
			}
			boundary := s.SourceRoot
			if task.Purpose == "scrape" {
				boundary = s.TargetRoot
			}
			for dir, depth := filepath.Dir(path), 1; fsops.Within(boundary, dir) && dir != boundary; dir, depth = filepath.Dir(dir), depth+1 {
				v.Document.Sources = append(v.Document.Sources, recognition.TextSource{ID: fmt.Sprintf("dir-%d", depth-1), Kind: recognition.Directories, Text: filepath.Base(dir), Depth: depth})
				if dir == boundary || filepath.Dir(dir) == dir {
					break
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}
func (a *WorkspaceService) Candidates(id string, in CandidateRequest) ([]CandidateView, error) {
	task, err := a.store.Workspace(id)
	if err != nil {
		return nil, err
	}
	if task.State == "running" || task.Revision != in.Revision {
		return nil, errors.New("工作区已更新，请重新加载")
	}
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	for _, path := range in.Paths {
		if !a.allowed(task.Purpose, path, s) {
			return nil, errors.New("文件超出当前工作区范围")
		}
	}
	return a.evidence(task, in.Paths)
}

var digitsRE = regexp.MustCompile(`[0-9]+`)

func digitStructure(text string) string {
	data, _ := json.Marshal(digitsRE.Split(text, -1))
	return string(data)
}
func sameDigitStructure(text, pattern string) bool {
	var parts []string
	if json.Unmarshal([]byte(pattern), &parts) != nil {
		return false
	}
	return slices.Equal(parts, digitsRE.Split(text, -1))
}
func checkedBasis(v CandidateView, b matching.EpisodeBasis, want []int) error {
	var numbers []int
	switch b.Kind {
	case "manual":
		if b.TraceID != "" || b.CandidateID != "" || b.SourceID != "" || b.Offset != 0 || b.Index != 0 || b.Pattern != "" {
			return errors.New("手动填写依据无效")
		}
		return nil
	case "suggestion":
		if b.TraceID == "" || b.TraceID != v.TraceID {
			return errors.New("建议快照已变化")
		}
		numbers = v.Suggestion.Metadata.Episodes
	case "candidate":
		if b.TraceID == "" || b.TraceID != v.TraceID {
			return errors.New("候选快照已变化")
		}
		for _, c := range v.Document.Candidates {
			if c.ID == b.CandidateID && c.Role == recognition.Episodes && c.Status == "valid" {
				data, _ := json.Marshal(c.Value)
				if err := json.Unmarshal(data, &numbers); err != nil {
					return errors.New("集数候选无效")
				}
				break
			}
		}
	case "digits":
		if b.TraceID != v.TraceID {
			return errors.New("原文快照已变化")
		}
		var text string
		found := false
		for _, s := range v.Document.Sources {
			if s.ID == b.SourceID {
				text = s.Text
				found = true
				break
			}
		}
		if !found || !sameDigitStructure(text, b.Pattern) {
			return errors.New("原文数字结构不一致")
		}
		matches := digitsRE.FindAllString(text, -1)
		if b.Index < 0 || b.Index >= len(matches) {
			return errors.New("原文缺少所选数字")
		}
		n, err := strconv.Atoi(matches[b.Index])
		if err != nil || n > 1<<53-1 {
			return errors.New("原文数字超出范围")
		}
		numbers = []int{n}
	default:
		return errors.New("请选择集数取值依据")
	}
	if len(numbers) == 0 || b.Offset < -(1<<53-1) || b.Offset > 1<<53-1 {
		return errors.New("集数取值依据无效")
	}
	shifted := make([]int, len(numbers))
	for i, n := range numbers {
		shifted[i] = n + b.Offset
		if shifted[i] < 0 || shifted[i] > 1<<53-1 {
			return errors.New("偏移后集数无效")
		}
	}
	if !slices.Equal(shifted, want) {
		return errors.New("集数与取值依据不一致")
	}
	return nil
}
func (a *WorkspaceService) Confirm(ctx context.Context, id string, in matching.ConfirmRequest) (matching.Workspace, error) {
	task, err := a.store.Workspace(id)
	if err != nil {
		return task, err
	}
	if task.State == "running" || task.Revision != in.Revision {
		return task, errors.New("工作区已更新，请重新加载")
	}
	if len(in.Items) == 0 || len(in.Items) > 2000 {
		return task, errors.New("请选择 1–2000 个文件")
	}
	s, err := a.settings.Load()
	if err != nil {
		return task, err
	}
	client, err := a.factory(s, nil)
	if err != nil {
		return task, err
	}
	memo := matching.NewResolver(client)
	paths := []string{}
	for _, e := range in.Items {
		paths = append(paths, e.Path)
	}
	views, err := a.evidence(task, paths)
	if err != nil {
		return task, err
	}
	evidence := map[string]CandidateView{}
	for _, v := range views {
		evidence[v.Path] = v
	}
	states, err := a.store.FileExecutions(id)
	if err != nil {
		return task, err
	}
	positions := map[string]int{}
	for i, it := range task.Items {
		positions[it.Path] = i
	}
	first := in.Items[0]
	for _, e := range in.Items {
		if e.Type != first.Type || e.TMDBID != first.TMDBID {
			return task, errors.New("一次只能确认一个作品")
		}
		i := positions[e.Path]
		it := &task.Items[i]
		if state := states[e.Path]; state.Locked || state.Completed {
			return task, errors.New("文件已提交或完成")
		}
		if !a.allowed(task.Purpose, e.Path, s) {
			return task, errors.New("文件超出工作区范围")
		}
		info, statErr := a.files.Access.Stat(e.Path)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != it.Size || info.ModTime().UnixNano() != it.MTime || it.Identity != "" && fsops.Identity(info) != it.Identity {
			return task, errors.New("文件已变化，请重新识别：" + e.Path)
		}
		if e.TMDBID <= 0 || (e.Type != "movie" && e.Type != "tv") {
			return task, errors.New("请选择作品")
		}
		entry, detailErr := memo.Detail(ctx, e.Type, e.TMDBID)
		if detailErr != nil {
			return task, detailErr
		}
		if e.Type == "tv" {
			if err = checkedBasis(evidence[e.Path], e.Basis, e.Episodes); err != nil {
				return task, fmt.Errorf("%s：%w", e.Path, err)
			}
		} else if e.Season != -1 || len(e.Episodes) != 0 || e.Basis.Kind != "manual" {
			return task, errors.New("电影不接受季集")
		}
		if it.Entry == nil || it.Entry.Type != e.Type || it.Entry.TMDBID != e.TMDBID {
			it.Layout.MoviePrefix = false
			it.Layout.Manual = false
			it.Layout.WorkDir = ""
			it.Layout.SeasonDir = ""
		}
		it.Identity = fsops.Identity(info)
		it.Entry = entry
		it.Season = e.Season
		it.Episodes = append([]int{}, e.Episodes...)
		memo.Validate(ctx, it)
		if it.State != "ready" {
			return task, fmt.Errorf("%s：%s", it.Path, it.Reason)
		}
		it.Confirm("user", e.Basis)
		it.Edited = true
		it.Selected = true
		it.Group = matching.EntryGroup(*entry)
		it.Reason = "已确认作品与季集"
	}
	a.coordinator.Lock()
	defer a.coordinator.Unlock()
	current, err := a.store.Workspace(id)
	if err != nil {
		return task, err
	}
	if current.Revision != in.Revision {
		return current, errors.New("工作区已更新，请重新加载")
	}
	now, err := a.settings.Load()
	if err != nil {
		return current, err
	}
	if now.Version != s.Version {
		return current, errors.New("设置已变化，请重新确认")
	}
	states, err = a.store.FileExecutions(id)
	if err != nil {
		return current, err
	}
	for _, e := range in.Items {
		it := task.Items[positions[e.Path]]
		info, statErr := a.files.Access.Stat(e.Path)
		state := states[e.Path]
		if state.Locked || state.Completed || statErr != nil || info.Size() != it.Size || info.ModTime().UnixNano() != it.MTime || fsops.Identity(info) != it.Identity {
			return current, errors.New("文件已变化或已提交，请重新加载")
		}
	}
	if task.Purpose == "scrape" {
		matching.InferLayouts(task.Items, s.TargetRoot)
	}
	if err = a.store.SaveWorkspace(&task); err != nil {
		return current, err
	}
	return a.project(task)
}
