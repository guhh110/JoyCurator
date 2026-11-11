package sqlite

import (
	"database/sql"
	"errors"
	"joycurator/internal/matching"
)

func (a *Store) SaveWorkspace(v *matching.Workspace) error {
	next := *v
	next.Revision++
	err := transaction(a.db, func(tx queryer) error {
		var revision int
		err := tx.QueryRow("SELECT f_revision FROM recognitions WHERE id=?", next.ID).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && revision != v.Revision {
			return errors.New("工作区已更新，请刷新后重试")
		}
		return writeRecognitions(tx, next.ID, next.ID, "", 0, next)
	})
	if err == nil {
		v.Revision = next.Revision
	}
	return err
}

// CheckpointWorkspace updates only changed file rows and the recognition summary.
func (a *Store) CheckpointWorkspace(v *matching.Workspace, positions []int) error {
	next := *v
	next.Revision++
	err := transaction(a.db, func(tx queryer) error {
		var revision int
		if err := tx.QueryRow("SELECT f_revision FROM recognitions WHERE id=?", v.ID).Scan(&revision); err != nil {
			return err
		}
		if revision != v.Revision {
			return errors.New("工作区已更新，请刷新后重试")
		}
		if err := putRecognitions(tx, next.ID, next.ID, "", 0, next); err != nil {
			return err
		}
		for _, i := range positions {
			if i < 0 || i >= len(v.Items) {
				return errors.New("工作区条目位置无效")
			}
			if err := writeWorkspaceItems(tx, childKey(v.ID, i), v.ID, v.ID, i, v.Items[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		v.Revision = next.Revision
	}
	return err
}
func (a *Store) Workspace(id string) (matching.Workspace, error) {
	return snapshotRead(a.db, func(q queryer) (matching.Workspace, error) { return readWorkspace(q, id) })
}
func readWorkspace(q queryer, id string) (matching.Workspace, error) {
	v, err := scanRecognitions(q.QueryRow("SELECT "+columnsRecognitions+" FROM recognitions WHERE id=?", id))
	v.ID = id
	if errors.Is(err, sql.ErrNoRows) {
		return v, errors.New("识别任务不存在")
	}
	if err != nil {
		return v, err
	}
	paths, err := readWorkspacePaths(q, id)
	if err != nil {
		return v, err
	}
	v.Paths = []string{}
	for _, p := range paths {
		v.Paths = append(v.Paths, p.Value)
	}
	items, err := readWorkspaceItems(q, id)
	if err != nil {
		return v, err
	}
	v.Items = []matching.Item{}
	for _, r := range items {
		v.Items = append(v.Items, r.Value)
	}
	return v, nil
}
func (a *Store) LatestWorkspace(purpose string) (*matching.Workspace, error) {
	var id string
	err := a.db.QueryRow("SELECT id FROM recognitions WHERE f_purpose=? ORDER BY sequence DESC LIMIT 1", purpose).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := a.Workspace(id)
	return &v, err
}
