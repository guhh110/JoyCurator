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
