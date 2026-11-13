package recognition

import (
	"context"
	"crypto/rand"
	"errors"
	"time"
)

func (a *Service) ExtractFile(ctx context.Context, path, root, workspace string) (Result, string, error) {
	snapshot, id, err := a.ExtractTree(ctx, BatchInput{ContextRoot: pathDir(path), SourceBoundary: root, Paths: []string{path}, Targets: []string{path}}, workspace)
	result := Result{Metadata: Metadata{Episodes: []int{}}, Steps: []Step{}}
	if len(snapshot.Files) > 0 {
		result.Metadata = snapshot.Files[0].Metadata
	}
	return result, id, err
}

// ExtractTree commits immutable local evidence before its identity can be used by a workspace.
func (a *Service) ExtractTree(ctx context.Context, in BatchInput, workspace string, progress ...Progress) (Snapshot, string, error) {
	engine := a.Current()
	id := rand.Text()
	trace := Trace{ID: id, Path: in.ContextRoot, Root: in.SourceBoundary, WorkspaceID: workspace, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Metadata: Metadata{Episodes: []int{}}}
	if err := a.store.StartTrace(trace); err != nil {
		return Snapshot{}, "", err
	}
	result, err := engine.Snapshot(ctx, in, progress...)
	if err == nil {
		err = ctx.Err()
	}
	trace.Status = "completed"
	if err != nil {
		trace.Status, trace.Error = "failed", err.Error()
		if ctx.Err() != nil {
			trace.Status = "cancelled"
			if a.closing() {
				trace.Status = "interrupted"
			}
		}
	}
	if len(result.Files) == 1 {
		trace.Metadata = result.Files[0].Metadata
	}
	trace.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if saveErr := a.store.FinishTrace(trace, result); saveErr != nil {
		return result, "", errors.Join(err, saveErr)
	}
	return result, id, err
}
