package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
	_ "modernc.org/sqlite"
)

func (a *Store) SaveRun(r organizer.Run) error {
	_, err := a.db.Exec("INSERT INTO runs(id,value) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", r.ID, jsonText(r))
	return err
}

func (a *Store) SaveBatch(runs []organizer.Run) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range runs {
		if _, err = tx.Exec("INSERT INTO runs(id,value) VALUES(?,?)", r.ID, jsonText(r)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (a *Store) Run(id string) (organizer.Run, error) {
	var r organizer.Run
	var raw string
	err := a.db.QueryRow("SELECT value FROM runs WHERE id=?", id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r)
	}
	return r, err
}
func (a *Store) Runs(page int) (map[string]any, error) {
	var total int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM runs").Scan(&total); err != nil {
		return nil, err
	}
	rows, err := a.db.Query("SELECT value FROM runs ORDER BY rowid DESC LIMIT 20 OFFSET ?", (page-1)*20)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []organizer.Run{}
	for rows.Next() {
		var raw string
		var r organizer.Run
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		r.Ops = nil
		items = append(items, r)
	}
	return map[string]any{"items": items, "total": total, "page": page}, rows.Err()
}

func (a *Store) AllRuns() ([]organizer.Run, error) { return a.readRuns("SELECT value FROM runs") }
func (a *Store) BatchRuns(id string) ([]organizer.Run, error) {
	return a.readRuns("SELECT value FROM runs WHERE json_extract(value,'$.batchId')=? ORDER BY rowid", id)
}
func (a *Store) readRuns(query string, args ...any) ([]organizer.Run, error) {
	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []organizer.Run{}
	for rows.Next() {
		var raw string
		var r organizer.Run
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (a *Store) LatestBatchID() (string, error) {
	var id string
	err := a.db.QueryRow("SELECT json_extract(value,'$.batchId') FROM runs WHERE json_extract(value,'$.batchId') IS NOT NULL ORDER BY rowid DESC LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
func (a *Store) Owned(target string) (*model.Work, error) {
	var raw string
	err := a.db.QueryRow(`SELECT op.value FROM runs, json_each(runs.value,'$.ops') op
 WHERE json_extract(runs.value,'$.state')<>'undone' AND json_extract(op.value,'$.target')=?
 AND json_extract(op.value,'$.status') IN ('completed','published') LIMIT 1`, target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var work model.Work
	err = json.Unmarshal([]byte(raw), &work)
	return &work, err
}
func (a *Store) Shared(id, target string) (bool, error) {
	var n int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM runs, json_each(runs.value,'$.ops') op
 WHERE runs.id<>? AND json_extract(runs.value,'$.state')<>'undone' AND json_extract(op.value,'$.target')=?
 AND json_extract(op.value,'$.status') IN ('completed','published')`, id, target).Scan(&n)
	return n > 0, err
}
