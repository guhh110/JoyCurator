package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"joycurator/internal/matching"
	"joycurator/internal/organizer"
)

func (a *Store) Enqueue(runs []organizer.Run) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	session := &writeSession{Tx: tx, statements: map[string]*sql.Stmt{}}
	defer session.close()
	for _, r := range runs {
		if r.State != "queued" || (r.Purpose != "organize" && r.Purpose != "scrape") {
			return errors.New("入队任务格式无效")
		}
		if err = saveRun(session, r); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO execution_queue(run_id,purpose) VALUES(?,?)", r.ID, r.Purpose); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (a *Store) QueuedRuns() ([]organizer.Run, error) {
	return a.fullRuns("SELECT " + runColumns("r") + " FROM execution_queue q JOIN runs r ON r.id=q.run_id ORDER BY q.position")
}
func (a *Store) FinishQueued(r organizer.Run) error {
	if r.State == "running" || r.State == "queued" {
		return errors.New("任务尚未结束")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = putRuns(tx, r.ID, r.ID, "", 0, r); err != nil {
		return err
	}
	result, err := tx.Exec("DELETE FROM execution_queue WHERE run_id=?", r.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("任务队列记录不存在")
	}
	return tx.Commit()
}
func (a *Store) CancelQueued(ids []string) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		result, err := tx.Exec("UPDATE runs SET f_state='cancelled',f_error='排队任务已取消，可重试' WHERE id=? AND f_state='queued'", id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("任务已不在等待队列")
		}
		result, err = tx.Exec("DELETE FROM execution_queue WHERE run_id=?", id)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return errors.New("任务队列记录不存在")
		}
	}
	return tx.Commit()
}
func (a *Store) recoverQueue() error {
	return transaction(a.db, func(tx queryer) error {
		var invalid int
		if err := tx.QueryRow(`SELECT
 (SELECT COUNT(*) FROM runs r LEFT JOIN execution_queue q ON q.run_id=r.id WHERE r.f_state IN ('queued','running') AND q.run_id IS NULL)
 +(SELECT COUNT(*) FROM execution_queue q LEFT JOIN runs r ON r.id=q.run_id WHERE r.id IS NULL OR r.f_state NOT IN ('queued','running') OR r.f_purpose<>q.purpose)`).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return errors.New("队列与任务记录不一致，请检查数据；未自动修复")
		}
		_, err := tx.Exec(`DELETE FROM execution_queue WHERE run_id IN (SELECT id FROM runs WHERE f_state='running');
 UPDATE runs SET f_state='interrupted',f_error='服务中断，可重试或撤销' WHERE f_state='running';
 UPDATE recognitions SET f_state='interrupted',f_error='服务中断，可重试未完成项' WHERE f_state='running';
 UPDATE extraction_traces SET f_status='interrupted',f_error='服务中断，保留已完成步骤' WHERE f_status='running';`)
		return err
	})
}
func (a *Store) QueuePage(purpose string, page int) (organizer.RunPage, error) {
	result := organizer.RunPage{Items: []organizer.Run{}, Page: page}
	if purpose != "organize" && purpose != "scrape" {
		return result, errors.New("队列类型无效")
	}
	if err := a.db.QueryRow("SELECT COUNT(*) FROM execution_queue q JOIN runs r ON r.id=q.run_id WHERE q.purpose=? AND r.f_state='queued'", purpose).Scan(&result.Total); err != nil {
		return result, err
	}
	items, err := a.readRuns("SELECT "+runColumns("r")+" FROM execution_queue q JOIN runs r ON r.id=q.run_id WHERE q.purpose=? AND r.f_state='queued' ORDER BY q.position LIMIT 20 OFFSET ?", purpose, (page-1)*20)
	result.Items = items
	return result, err
}
func (a *Store) BatchPosition(id string) (int, error) {
	var position int
	err := a.db.QueryRow(`SELECT COUNT(DISTINCT r.f_batch_id) FROM execution_queue q JOIN runs r ON r.id=q.run_id WHERE q.purpose='organize' AND q.position<=(SELECT MIN(q2.position) FROM execution_queue q2 JOIN runs r2 ON r2.id=q2.run_id WHERE r2.f_batch_id=?)`, id).Scan(&position)
	return position, err
}
func (a *Store) Batches(page int) (organizer.BatchPage, error) {
	return snapshotRead(a.db, func(q queryer) (organizer.BatchPage, error) {
		result := organizer.BatchPage{Items: []organizer.Batch{}, Page: page}
		if err := q.QueryRow("SELECT COUNT(DISTINCT f_batch_id) FROM runs WHERE f_batch_id<>''").Scan(&result.Total); err != nil {
			return result, err
		}
		selected := "SELECT f_batch_id FROM runs WHERE f_batch_id<>'' GROUP BY f_batch_id ORDER BY MIN(sequence) DESC LIMIT 20 OFFSET ?"
		rows, err := q.Query(selected, (page-1)*20)
		if err != nil {
			return result, err
		}
		index := map[string]int{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return result, err
			}
			index[id] = len(result.Items)
			result.Items = append(result.Items, organizer.Batch{ID: id, State: "completed", Runs: []organizer.Run{}})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		summaries, err := queryRuns(q, "SELECT id,"+columnsRuns+" FROM runs WHERE f_batch_id IN ("+selected+") ORDER BY sequence", (page-1)*20)
		if err != nil {
			return result, err
		}
		for _, r := range summaries {
			i := index[r.BatchID]
			result.Items[i].Runs = append(result.Items[i].Runs, r)
		}
		rows, err = q.Query("SELECT r.f_batch_id FROM execution_queue q JOIN runs r ON r.id=q.run_id WHERE q.purpose='organize' GROUP BY r.f_batch_id ORDER BY MIN(q.position)")
		if err != nil {
			return result, err
		}
		position := 0
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return result, err
			}
			position++
			if i, ok := index[id]; ok {
				result.Items[i].Position = position
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		for i := range result.Items {
			batch := &result.Items[i]
			for _, r := range batch.Runs {
				if r.State == "running" {
					batch.State = "running"
					batch.Active = true
					break
				}
				if r.State == "queued" {
					batch.State = "queued"
					batch.Active = true
				}
				if !batch.Active && r.State != "completed" && r.State != "undone" {
					batch.State = r.State
				}
			}
		}
		return result, nil
	})
}

// Execution progress is projected onto workspaces without changing edit revisions.
func (a *Store) FileExecutions(workspace string) (map[string]matching.FileExecution, error) {
	rows, err := a.db.Query(`WITH relevant AS MATERIALIZED (
 SELECT r.id,r.f_state state,r.f_recognition_id workspace,r.sequence FROM runs r WHERE r.f_purpose='organize'
 AND (r.f_recognition_id=? OR r.f_state IN ('queued','running') OR (r.f_state IN ('failed','cancelled','interrupted') AND EXISTS(SELECT 1 FROM operations o WHERE o.root=r.id AND o.undo_parent IS NULL AND o.f_status='manual_review'))))
 SELECT r.id,r.state,r.workspace,m.f_source,m.f_run_id,m.f_identity,COALESCE(o.f_status,'')
 FROM relevant r JOIN run_mappings m ON m.root=r.id
 LEFT JOIN operations o ON o.root=r.id AND o.undo_parent IS NULL AND o.f_kind='video' AND o.f_target=m.f_path ORDER BY r.sequence`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]matching.FileExecution{}
	for rows.Next() {
		var id, state, source, identity, status string
		var owner, registered sql.NullString
		if err = rows.Scan(&id, &state, &owner, &source, &registered, &identity, &status); err != nil {
			return nil, err
		}
		locked := state == "queued" || state == "running" || status == "manual_review"
		completed := (status == "completed" || status == "" && state != "undone") && registered.String == id && identity != "" && owner.String == workspace
		if old, ok := result[source]; ok && old.Locked && !locked {
			continue
		}
		if owner.String == workspace || locked {
			result[source] = matching.FileExecution{RunID: id, State: state, Locked: locked, Completed: completed, ManualReview: status == "manual_review"}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("读取文件执行状态失败：%w", err)
	}
	return result, nil
}
