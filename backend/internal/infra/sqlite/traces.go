package sqlite

import (
	"database/sql"
	"errors"

	"joycurator/internal/recognition"
	_ "modernc.org/sqlite"
)

func (a *Store) StartTrace(t recognition.Trace) error {
	_, err := a.db.Exec("INSERT INTO extraction_traces(id,path,root,workspace_id,status,started_at) VALUES(?,?,?,?,?,?)", t.ID, t.Path, t.Root, t.WorkspaceID, t.Status, t.StartedAt)
	return err
}
func (a *Store) FinishTrace(t recognition.Trace, steps []recognition.Step) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare("INSERT INTO extraction_steps(trace_id,position,value) VALUES(?,?,?)")
	if err != nil {
		return err
	}
	defer statement.Close()
	for i, step := range steps {
		if _, err = statement.Exec(t.ID, i+1, jsonText(step)); err != nil {
			return err
		}
	}
	result, err := tx.Exec("UPDATE extraction_traces SET status=?,finished_at=?,metadata=?,error=? WHERE id=? AND status='running'", t.Status, t.FinishedAt, jsonText(t.Metadata), t.Error, t.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("轨迹不存在或已结束")
	}
	return tx.Commit()
}
func (a *Store) DeleteTrace(id string) error {
	r, err := a.db.Exec("DELETE FROM extraction_traces WHERE id=? AND status<>'running'", id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("轨迹不存在或尚在运行，不能清理")
	}
	return nil
}
func (a *Store) TraceHistory(page int, path, workspace string) ([]recognition.Trace, int, error) {
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	where := ""
	args := []any{}
	if path != "" {
		where = " WHERE t.path=?"
		args = append(args, path)
	}
	if workspace != "" {
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += "t.workspace_id=?"
		args = append(args, workspace)
	}
	var total int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM extraction_traces t"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, (page-1)*20)
	rows, err := a.db.Query("SELECT t.id,t.path,t.root,t.workspace_id,t.status,t.started_at,t.finished_at,t.metadata,t.error,(SELECT COUNT(*) FROM extraction_steps s WHERE s.trace_id=t.id) FROM extraction_traces t"+where+" ORDER BY t.rowid DESC LIMIT 20 OFFSET ?", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []recognition.Trace{}
	for rows.Next() {
		var value recognition.Trace
		var raw string
		if err = rows.Scan(&value.ID, &value.Path, &value.Root, &value.WorkspaceID, &value.Status, &value.StartedAt, &value.FinishedAt, &raw, &value.Error, &value.StepCount); err != nil {
			return nil, 0, err
		}
		if err = decode(raw, &value.Metadata); err != nil {
			return nil, 0, err
		}
		if value.Metadata.Episodes == nil {
			value.Metadata.Episodes = []int{}
		}
		items = append(items, value)
	}
	return items, total, rows.Err()
}
func (a *Store) TraceDetail(id string) (recognition.Trace, []recognition.Step, error) {
	var item recognition.Trace
	var raw string
	err := a.db.QueryRow("SELECT id,path,root,workspace_id,status,started_at,finished_at,metadata,error FROM extraction_traces WHERE id=?", id).Scan(&item.ID, &item.Path, &item.Root, &item.WorkspaceID, &item.Status, &item.StartedAt, &item.FinishedAt, &raw, &item.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return item, nil, errors.New("轨迹不存在或已清理")
	}
	if err != nil {
		return item, nil, err
	}
	if err = decode(raw, &item.Metadata); err != nil {
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
		if err = decode(text, &step); err != nil {
			return item, nil, err
		}
		steps = append(steps, step)
	}
	item.StepCount = len(steps)
	return item, steps, rows.Err()
}
