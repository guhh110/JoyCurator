package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"path/filepath"
	"time"
)

type extractionTrace struct {
	ID          string               `json:"id"`
	Path        string               `json:"path"`
	Root        string               `json:"root"`
	WorkspaceID string               `json:"workspaceId,omitempty"`
	Status      string               `json:"status"`
	StartedAt   string               `json:"startedAt"`
	FinishedAt  string               `json:"finishedAt,omitempty"`
	Metadata    recognition.Metadata `json:"metadata"`
	Error       string               `json:"error,omitempty"`
	StepCount   int                  `json:"stepCount"`
}

func (a *App) initExtraction() error {
	_, err := a.db.Exec(`CREATE TABLE IF NOT EXISTS extraction_rules(id INTEGER PRIMARY KEY CHECK(id=1),value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS extraction_traces(id TEXT PRIMARY KEY,path TEXT NOT NULL,root TEXT NOT NULL,workspace_id TEXT NOT NULL,status TEXT NOT NULL,started_at TEXT NOT NULL,finished_at TEXT NOT NULL DEFAULT '',metadata TEXT NOT NULL DEFAULT '{}',error TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS extraction_steps(trace_id TEXT NOT NULL REFERENCES extraction_traces(id) ON DELETE CASCADE,position INTEGER NOT NULL,value TEXT NOT NULL,PRIMARY KEY(trace_id,position));
 CREATE INDEX IF NOT EXISTS extraction_trace_workspace ON extraction_traces(workspace_id);
 UPDATE extraction_traces SET status='interrupted',error='服务中断，保留已完成步骤' WHERE status='running';`)
	if err != nil {
		return err
	}
	if _, err = a.db.Exec("INSERT OR IGNORE INTO extraction_rules(id,value) VALUES(1,?)", jsonText(recognition.DefaultRules())); err != nil {
		return err
	}
	var raw string
	if err = a.db.QueryRow("SELECT value FROM extraction_rules WHERE id=1").Scan(&raw); err != nil {
		return err
	}
	var rules []recognition.Rule
	if err = json.Unmarshal([]byte(raw), &rules); err != nil {
		return err
	}
	migrated, changed, err := recognition.MigrateLegacyRules(rules)
	if err != nil {
		return err
	}
	engine, err := recognition.Compile(migrated)
	if err != nil {
		return err
	}
	if changed {
		if _, err = a.db.Exec("UPDATE extraction_rules SET value=? WHERE id=1", jsonText(engine.Rules())); err != nil {
			return err
		}
	}
	a.extractor = engine
	return nil
}
func (a *App) currentExtractor() *recognition.Engine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.extractor
}
func (a *App) saveExtractionRules(rules []recognition.Rule) error {
	engine, err := recognition.Compile(rules)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("服务正在关闭")
	}
	if _, err = a.db.Exec("UPDATE extraction_rules SET value=? WHERE id=1", jsonText(engine.Rules())); err != nil {
		return err
	}
	a.extractor = engine
	return nil
}
func (a *App) extractFile(ctx context.Context, path, root, workspace string) (recognition.Result, string, error) {
	engine := a.currentExtractor()
	id := key()
	started := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.db.Exec("INSERT INTO extraction_traces(id,path,root,workspace_id,status,started_at) VALUES(?,?,?,?,?,?)", id, path, root, workspace, "running", started); err != nil {
		return recognition.Result{}, "", err
	}
	position := 0
	result, err := engine.Extract(ctx, recognition.Input{Path: path, Root: root}, func(_ context.Context, step recognition.Step) error {
		position++
		_, e := a.db.Exec("INSERT INTO extraction_steps(trace_id,position,value) VALUES(?,?,?)", id, position, jsonText(step))
		return e
	})
	status, message := "completed", ""
	if err != nil {
		status, message = "failed", err.Error()
		if ctx.Err() != nil {
			status = "cancelled"
			if a.isClosing() {
				status = "interrupted"
			}
		}
	}
	_, saveErr := a.db.Exec("UPDATE extraction_traces SET status=?,finished_at=?,metadata=?,error=? WHERE id=?", status, time.Now().UTC().Format(time.RFC3339Nano), jsonText(result.Metadata), message, id)
	if err == nil {
		err = saveErr
	}
	return result, id, err
}

// parsedMetadata is the only compatibility mapping to the older consumer DTO.
func parsedMetadata(metadata recognition.Metadata) model.Parsed {
	p := model.Parsed{Season: -1, Episodes: append([]int{}, metadata.Episodes...)}
	if metadata.Title != nil {
		p.Title = *metadata.Title
	}
	if metadata.MediaType != nil {
		p.Type = *metadata.MediaType
	}
	if metadata.TMDBID != nil {
		p.TMDBID = *metadata.TMDBID
	}
	if metadata.Year != nil {
		p.Year = *metadata.Year
	}
	if metadata.Season != nil {
		p.Season = *metadata.Season
	}
	if metadata.ReleaseTag != nil {
		p.ReleaseTag = *metadata.ReleaseTag
	}
	return p
}
func (a *App) parseForConsumer(ctx context.Context, path, root string) (model.Parsed, error) {
	result, err := a.currentExtractor().Extract(ctx, recognition.Input{Path: path, Root: root}, nil)
	return parsedMetadata(result.Metadata), err
}
func (a *App) extractionHistory(page int, path, workspace string) ([]extractionTrace, int, error) {
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	where := " WHERE (?='' OR t.path=?) AND (?='' OR t.workspace_id=?)"
	var total int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM extraction_traces t"+where, path, path, workspace, workspace).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := a.db.Query("SELECT t.id,t.path,t.root,t.workspace_id,t.status,t.started_at,t.finished_at,t.metadata,t.error,(SELECT COUNT(*) FROM extraction_steps s WHERE s.trace_id=t.id) FROM extraction_traces t"+where+" ORDER BY t.rowid DESC LIMIT 20 OFFSET ?", path, path, workspace, workspace, (page-1)*20)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []extractionTrace{}
	for rows.Next() {
		var value extractionTrace
		var raw string
		if err = rows.Scan(&value.ID, &value.Path, &value.Root, &value.WorkspaceID, &value.Status, &value.StartedAt, &value.FinishedAt, &raw, &value.Error, &value.StepCount); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &value.Metadata); err != nil {
			return nil, 0, err
		}
		if value.Metadata.Episodes == nil {
			value.Metadata.Episodes = []int{}
		}
		items = append(items, value)
	}
	return items, total, rows.Err()
}
func (a *App) extractionDetail(id string) (extractionTrace, []recognition.Step, error) {
	var item extractionTrace
	var raw string
	err := a.db.QueryRow("SELECT id,path,root,workspace_id,status,started_at,finished_at,metadata,error FROM extraction_traces WHERE id=?", id).Scan(&item.ID, &item.Path, &item.Root, &item.WorkspaceID, &item.Status, &item.StartedAt, &item.FinishedAt, &raw, &item.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return item, nil, errors.New("轨迹不存在或已清理")
	}
	if err != nil {
		return item, nil, err
	}
	if err = json.Unmarshal([]byte(raw), &item.Metadata); err != nil {
		return item, nil, err
	}
	if item.Metadata.Episodes == nil {
		item.Metadata.Episodes = []int{}
	}
	rows, err := a.db.Query("SELECT value FROM extraction_steps WHERE trace_id=? ORDER BY position", id)
	if err != nil {
		return item, nil, err
	}
	defer rows.Close()
	steps := []recognition.Step{}
	for rows.Next() {
		var text string
		var step recognition.Step
		if err = rows.Scan(&text); err != nil {
			return item, nil, err
		}
		if err = json.Unmarshal([]byte(text), &step); err != nil {
			return item, nil, err
		}
		steps = append(steps, step)
	}
	item.StepCount = len(steps)
	return item, steps, rows.Err()
}
func extractionRoot(path, root, source string) string {
	if root != "" {
		return root
	}
	if source != "" {
		rel, err := filepath.Rel(source, filepath.Clean(path))
		if err == nil && filepath.IsLocal(rel) {
			return source
		}
	}
	return "/"
}
