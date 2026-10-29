package recognition

import (
	"context"
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
	position := 0
	result, err := engine.Extract(ctx, Input{Path: path, Root: root}, func(_ context.Context, step Step) error {
		position++
		e := a.store.AppendStep(id, position, step)
		return e
	})
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
	saveErr := a.store.FinishTrace(trace)
	if err == nil {
		err = saveErr
	}
	return result, id, err
}
