package sqlite

import (
	"database/sql"
	"errors"

	"joycurator/internal/model"
)

func (a *Store) SaveMapping(item model.MediaMapping) error {
	result, err := a.db.Exec(`INSERT INTO media_mappings(path,value) VALUES(?,?)
	 ON CONFLICT(path) DO UPDATE SET value=excluded.value
	 WHERE json_extract(media_mappings.value,'$.runId')=json_extract(excluded.value,'$.runId')
	 OR EXISTS (SELECT 1 FROM runs incoming, runs previous
	    WHERE incoming.id=json_extract(excluded.value,'$.runId')
	    AND previous.id=json_extract(media_mappings.value,'$.runId')
	    AND incoming.rowid > previous.rowid)`, item.Path, jsonText(item))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return errors.New("目标视频已有更新的整理映射：" + item.Path)
	}
	return nil
}

func (a *Store) Mappings() ([]model.MediaMapping, error) {
	rows, err := a.db.Query("SELECT value FROM media_mappings ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.MediaMapping{}
	for rows.Next() {
		var raw string
		var item model.MediaMapping
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = decode(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *Store) DeleteMapping(item model.MediaMapping) error {
	_, err := a.db.Exec("DELETE FROM media_mappings WHERE path=? AND json_extract(value,'$.runId')=? AND json_extract(value,'$.identity')=?", item.Path, item.RunID, item.Identity)
	return err
}

func (a *Store) UpdateMappingState(item model.MediaMapping, state, runID string) error {
	_, err := a.db.Exec(`UPDATE media_mappings SET value=json_set(value,'$.state',?)
	 WHERE path=? AND json_extract(value,'$.runId')=? AND json_extract(value,'$.identity')=?
	 AND NOT EXISTS (
	   SELECT 1 FROM runs newer, json_each(newer.value,'$.mappings') mapped
	   WHERE json_extract(newer.value,'$.purpose')='scrape'
	   AND json_extract(newer.value,'$.state')<>'undone'
	   AND json_extract(mapped.value,'$.path')=?
	   AND newer.rowid > (SELECT rowid FROM runs WHERE id=?))`, state, item.Path, item.RunID, item.Identity, item.Path, runID)
	return err
}

func (a *Store) Mapping(path string) (*model.MediaMapping, error) {
	var raw string
	var item model.MediaMapping
	err := a.db.QueryRow("SELECT value FROM media_mappings WHERE path=?", path).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err == nil {
		err = decode(raw, &item)
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
