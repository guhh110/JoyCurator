package recognition

import (
	"context"
	"errors"
	"time"

	"crypto/rand"
)

func (a *Service) ExtractFile(ctx context.Context, path, root, workspace string) (Result, string, error) {
	engine := a.Current()
	id := rand.Text()
	started := time.Now().UTC().Format(time.RFC3339Nano)
	trace := Trace{ID: id, Path: path, Root: root, WorkspaceID: workspace, Status: "running", StartedAt: started}
	if err := a.store.StartTrace(trace); err != nil {
		return Result{}, "", err
	}
	result, err := engine.Extract(ctx, Input{Path: path, Root: root}, nil)
	status, message := "completed", ""
	if err != nil {
		status, message = "failed", err.Error()
		if ctx.Err() != nil {
			status = "cancelled"
			if a.closing() {
				status = "interrupted"
			}
		}
	}
	trace.Status, trace.Error, trace.Metadata, trace.FinishedAt = status, message, result.Metadata, time.Now().UTC().Format(time.RFC3339Nano)
	saveErr := a.store.FinishTrace(trace, result.Steps)
	err = errors.Join(err, saveErr)
	return result, id, err
}

// ExtractTree snapshots one pipeline for every file in the context and shares a trace.
func (a *Service) ExtractTree(ctx context.Context, in BatchInput, workspace string) (BatchResult, string, error) {
	engine := a.Current()
	id := rand.Text()
	trace := Trace{ID: id, Path: in.ContextRoot, Root: in.SourceBoundary, WorkspaceID: workspace, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Metadata: Metadata{Episodes: []int{}}}
	if err := a.store.StartTrace(trace); err != nil {
		return BatchResult{}, "", err
	}
	result, err := engine.ExtractBatch(ctx, in, nil)
	trace.Status = "completed"
	if err != nil {
		trace.Status = "failed"
		trace.Error = err.Error()
		if ctx.Err() != nil {
			trace.Status = "cancelled"
			if a.closing() {
				trace.Status = "interrupted"
			}
		}
	}
	trace.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result, id, errors.Join(err, a.store.FinishTrace(trace, result.Steps))
}

// RestoreBatch rebuilds one shared context and each file's stage outputs.
func RestoreBatch(steps []Step) (BatchResult, error) {
	out := BatchResult{Documents: []Document{}, Files: []FileResult{}, Steps: steps, Tree: []TreeNode{}}
	index := map[string]int{}
	for _, step := range steps {
		switch step.Stage {
		case "tokenize":
			var snapshot struct {
				Document Document    `json:"document"`
				Context  *BatchInput `json:"context,omitempty"`
				Tree     []TreeNode  `json:"tree,omitempty"`
			}
			if err := strict(step.Before, &snapshot); err != nil {
				return out, err
			}
			if snapshot.Context != nil {
				out.Context = *snapshot.Context
				out.Tree = snapshot.Tree
			}
			doc := snapshot.Document
			if _, exists := index[doc.FileID]; exists {
				return out, errors.New("重复文件分词快照")
			}
			if len(out.Documents) == 0 && snapshot.Context == nil {
				return out, errors.New("缺少共享目录上下文")
			}
			if doc.FileID != step.FileID {
				return out, errors.New("分词快照文件不符")
			}
			if step.Status == "matched" {
				if err := strict(step.ExtractedValue, &doc.Tokens); err != nil {
					return out, err
				}
				if err := validateTokens(doc, doc.Tokens); err != nil {
					return out, err
				}
			}
			index[doc.FileID] = len(out.Documents)
			out.Documents = append(out.Documents, doc)
		case "annotate":
			i, ok := index[step.FileID]
			if !ok {
				return out, errors.New("标注缺少分词快照")
			}
			if step.Status == "matched" {
				var candidates []Candidate
				if err := strict(step.ExtractedValue, &candidates); err != nil {
					return out, err
				}
				out.Documents[i].Candidates = append(out.Documents[i].Candidates, candidates...)
			}
		case "resolve":
			if step.Status == "matched" {
				if err := strict(step.ExtractedValue, &out.Files); err != nil {
					return out, err
				}
			}
		default:
			return out, errors.New("未知轨迹阶段")
		}
	}
	return out, nil
}
