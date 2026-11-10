package sqlite

import (
	"database/sql"
	"errors"
	"joycurator/internal/recognition"
)

func (a *Store) StartTrace(t recognition.Trace) error {
	return transaction(a.db, func(tx queryer) error {
		// INSERT, rather than upsert: a trace identity cannot be reused.
		if _, err := tx.Exec("INSERT INTO extraction_traces(id,f_path,f_root,f_workspace_id,f_status,f_started_at,f_finished_at,f_error) VALUES(?,?,?,?,?,?,?,?)", t.ID, t.Path, t.Root, t.WorkspaceID, t.Status, t.StartedAt, t.FinishedAt, t.Error); err != nil {
			return err
		}
		return writeTraceMetadata(tx, childKey(t.ID, 0), t.ID, t.ID, 0, t.Metadata)
	})
}
func (a *Store) FinishTrace(t recognition.Trace, steps []recognition.Step) error {
	return transaction(a.db, func(tx queryer) error {
		result, err := tx.Exec("UPDATE extraction_traces SET f_status=?,f_finished_at=?,f_error=? WHERE id=? AND f_status='running'", t.Status, t.FinishedAt, t.Error, t.ID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("轨迹不存在或已结束")
		}
		if err = writeTraceMetadata(tx, childKey(t.ID, 0), t.ID, t.ID, 0, t.Metadata); err != nil {
			return err
		}
		batch := newTraceBatch(tx)
		for i, s := range steps {
			if err = writeExtractionSteps(batch, childKey(t.ID, i), t.ID, t.ID, i, s); err != nil {
				return err
			}
		}
		return batch.flush()
	})
}
func (a *Store) DeleteTrace(id string) error {
	r, err := a.db.Exec("DELETE FROM extraction_traces WHERE id=? AND f_status<>'running'", id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("轨迹不存在或尚在运行，不能清理")
	}
	return err
}
func (a *Store) TraceHistory(page int, path, workspace string) ([]recognition.Trace, int, error) {
	tx, txErr := a.db.Begin()
	if txErr != nil {
		return nil, 0, txErr
	}
	defer tx.Rollback()

	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	where := " WHERE 1=1"
	args := []any{}
	if path != "" {
		where += " AND (t.f_path=? OR EXISTS(SELECT 1 FROM step_inputs i WHERE i.root=t.id AND i.value=?))"
		args = append(args, path, path)
	}
	if workspace != "" {
		where += " AND f_workspace_id=?"
		args = append(args, workspace)
	}
	var total int
	if err := tx.QueryRow("SELECT COUNT(*) FROM extraction_traces t"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, (page-1)*20)
	rows, err := tx.Query("SELECT id,"+columnsExtractionTraces+",(SELECT COUNT(*) FROM extraction_steps s WHERE s.root=t.id) FROM extraction_traces t"+where+" ORDER BY sequence DESC LIMIT 20 OFFSET ?", args...)
	if err != nil {
		return nil, 0, err
	}
	out := []recognition.Trace{}
	for rows.Next() {
		var v recognition.Trace
		var id string
		var count int
		v, err = scanExtractionTraces(suffixScanner{rows, []any{&id}, []any{&count}})
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		v.ID = id
		v.StepCount = count
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}

	// Read all metadata and episode rows for this page together. No step content is read.
	selected := "SELECT id FROM extraction_traces t" + where + " ORDER BY sequence DESC LIMIT 20 OFFSET ?"
	rows, err = tx.Query("SELECT owner,"+columnsTraceMetadata+" FROM trace_metadata WHERE root IN ("+selected+")", args...)
	if err != nil {
		return nil, 0, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].ID] = i
	}
	found := map[string]bool{}
	for rows.Next() {
		var owner string
		m, e := scanTraceMetadata(prefixScanner{rows, []any{&owner}})
		if e != nil {
			rows.Close()
			return nil, 0, e
		}
		i, ok := index[owner]
		if !ok {
			rows.Close()
			return nil, 0, errors.New("轨迹元数据归属无效")
		}
		m.Episodes = []int{}
		out[i].Metadata = m
		found[owner] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	if len(found) != len(out) {
		return nil, 0, errors.New("轨迹元数据不存在")
	}
	rows, err = tx.Query("SELECT m.root,e.value FROM trace_episodes e JOIN trace_metadata m ON m.k=e.owner WHERE m.root IN ("+selected+") ORDER BY e.owner,e.position", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var episode int
		if err = rows.Scan(&owner, &episode); err != nil {
			return nil, 0, err
		}
		i := index[owner]
		out[i].Metadata.Episodes = append(out[i].Metadata.Episodes, episode)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type suffixScanner struct {
	s              scanner
	prefix, suffix []any
}

func (s suffixScanner) Scan(args ...any) error {
	all := append(s.prefix, args...)
	return s.s.Scan(append(all, s.suffix...)...)
}
func (a *Store) TraceDetail(id string) (recognition.Trace, []recognition.Step, error) {
	tx, txErr := a.db.Begin()
	if txErr != nil {
		return recognition.Trace{}, nil, txErr
	}
	defer tx.Rollback()

	v, err := scanExtractionTraces(tx.QueryRow("SELECT "+columnsExtractionTraces+" FROM extraction_traces WHERE id=?", id))
	v.ID = id
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil, errors.New("轨迹不存在或已清理")
	}
	if err != nil {
		return v, nil, err
	}
	m, err := readTraceMetadata(tx, id)
	if err != nil {
		return v, nil, err
	}
	if len(m) != 1 {
		return v, nil, errors.New("轨迹元数据不存在")
	}
	v.Metadata = m[0].Value
	rows, err := readExtractionSteps(tx, id)
	if err != nil {
		return v, nil, err
	}
	steps := []recognition.Step{}
	for _, r := range rows {
		steps = append(steps, r.Value)
	}
	v.StepCount = len(steps)
	if err = tx.Commit(); err != nil {
		return v, nil, err
	}
	return v, steps, nil
}
