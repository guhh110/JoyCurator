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
func (a *Store) FinishTrace(t recognition.Trace, snapshot recognition.Snapshot) error {
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
		if _, err = tx.Exec("INSERT INTO extraction_snapshots(id,digest) VALUES(?,?)", t.ID, snapshot.Digest); err != nil {
			return err
		}
		for i, rule := range snapshot.Pipeline.Rules() {
			if err = writeSnapshotRules(tx, childKey(t.ID, i), t.ID, t.ID, i, rule); err != nil {
				return err
			}
		}
		batch := newTraceBatch(tx)
		if err = writeSnapshotContext(batch, childKey(t.ID, 0), t.ID, t.ID, 0, snapshot.Context); err != nil {
			return err
		}
		for i, v := range snapshot.Tree {
			if err = writeSnapshotTree(batch, childKey(t.ID, i), t.ID, t.ID, i, v); err != nil {
				return err
			}
		}
		for i, v := range snapshot.Documents {
			if err = writeSnapshotDocuments(batch, childKey(t.ID, i), t.ID, t.ID, i, v); err != nil {
				return err
			}
		}
		for i, v := range snapshot.Files {
			if err = writeSnapshotFiles(batch, childKey(t.ID, i), t.ID, t.ID, i, v); err != nil {
				return err
			}
		}
		for i, v := range snapshot.Diagnostics {
			if err = writeSnapshotDiagnostics(batch, childKey(t.ID, i), t.ID, t.ID, i, v); err != nil {
				return err
			}
		}
		return batch.flush()
	})
}
func (a *Store) DeleteTrace(id string) error {
	r, err := a.db.Exec("DELETE FROM extraction_traces WHERE id=? AND f_status<>'running' AND NOT EXISTS(SELECT 1 FROM workspace_items WHERE f_trace_id=extraction_traces.id) AND NOT EXISTS(SELECT 1 FROM workspace_confirmations WHERE f_trace_id=extraction_traces.id)", id)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("轨迹不存在、仍在运行或被工作区引用，不能清理")
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
		where += " AND (t.f_path=? OR EXISTS(SELECT 1 FROM snapshot_documents i WHERE i.root=t.id AND i.path=?))"
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
	rows, err := tx.Query("SELECT id,"+columnsExtractionTraces+" FROM extraction_traces t"+where+" ORDER BY sequence DESC LIMIT 20 OFFSET ?", args...)
	if err != nil {
		return nil, 0, err
	}
	out := []recognition.Trace{}
	for rows.Next() {
		var v recognition.Trace
		var id string
		v, err = scanExtractionTraces(prefixScanner{rows, []any{&id}})
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		v.ID = id
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

func (a *Store) TraceDetail(id string) (recognition.Trace, recognition.Snapshot, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return recognition.Trace{}, recognition.Snapshot{}, err
	}
	defer tx.Rollback()
	v, err := scanExtractionTraces(tx.QueryRow("SELECT "+columnsExtractionTraces+" FROM extraction_traces WHERE id=?", id))
	v.ID = id
	out := recognition.Snapshot{Tree: []recognition.TreeNode{}, Documents: []recognition.Document{}, Files: []recognition.FileResult{}, Diagnostics: []recognition.Diagnostic{}}
	if errors.Is(err, sql.ErrNoRows) {
		return v, out, errors.New("轨迹不存在或已清理")
	}
	if err != nil {
		return v, out, err
	}
	m, err := readTraceMetadata(tx, id)
	if err != nil {
		return v, out, err
	}
	if len(m) != 1 {
		return v, out, errors.New("轨迹元数据不存在")
	}
	v.Metadata = m[0].Value
	if v.Status == "running" || v.Status == "interrupted" {
		var count int
		if err = tx.QueryRow("SELECT COUNT(*) FROM extraction_snapshots WHERE id=?", id).Scan(&count); err != nil {
			return v, out, err
		}
		if count == 0 {
			return v, out, tx.Commit()
		}
	}
	if err = tx.QueryRow("SELECT digest FROM extraction_snapshots WHERE id=?", id).Scan(&out.Digest); err != nil {
		return v, out, err
	}
	contexts, err := readSnapshotContext(tx, id)
	if err != nil {
		return v, out, err
	}
	if len(contexts) != 1 {
		return v, out, errors.New("缺少目录快照")
	}
	out.Context = contexts[0].Value
	rules, err := readSnapshotRules(tx, id)
	if err != nil {
		return v, out, err
	}
	out.Pipeline.Annotators = []recognition.Rule{}
	for _, r := range rules {
		switch r.Value.Stage {
		case "tokenize":
			out.Pipeline.Tokenizer = r.Value
		case "annotate":
			out.Pipeline.Annotators = append(out.Pipeline.Annotators, r.Value)
		case "resolve":
			out.Pipeline.Resolver = r.Value
		}
	}
	tree, err := readSnapshotTree(tx, id)
	if err != nil {
		return v, out, err
	}
	for _, r := range tree {
		out.Tree = append(out.Tree, r.Value)
	}
	docs, err := readSnapshotDocuments(tx, id)
	if err != nil {
		return v, out, err
	}
	for _, r := range docs {
		out.Documents = append(out.Documents, r.Value)
	}
	files, err := readSnapshotFiles(tx, id)
	if err != nil {
		return v, out, err
	}
	for _, r := range files {
		out.Files = append(out.Files, r.Value)
	}
	diagnostics, err := readSnapshotDiagnostics(tx, id)
	if err != nil {
		return v, out, err
	}
	for _, r := range diagnostics {
		out.Diagnostics = append(out.Diagnostics, r.Value)
	}
	return v, out, tx.Commit()
}
