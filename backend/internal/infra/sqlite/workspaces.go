package sqlite

import (
	"database/sql"
	"errors"

	"joycurator/internal/matching"
	_ "modernc.org/sqlite"
)

func (a *Store) SaveWorkspace(task *matching.Workspace) error {
	task.Revision++
	_, err := a.db.Exec("INSERT INTO recognitions(id,value) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", task.ID, jsonText(task))
	return err
}
func (a *Store) Workspace(id string) (matching.Workspace, error) {
	var raw string
	var task matching.Workspace
	err := a.db.QueryRow("SELECT value FROM recognitions WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return task, errors.New("识别任务不存在")
	}
	if err != nil {
		return task, err
	}
	err = decode(raw, &task)
	return task, err
}
func (a *Store) LatestWorkspace() (*matching.Workspace, error) {
	var raw string
	err := a.db.QueryRow("SELECT value FROM recognitions ORDER BY rowid DESC LIMIT 1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var w matching.Workspace
	err = decode(raw, &w)
	return &w, err
}
