package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func (a *Store) ScrapeWorks(search, cursor string) (organizer.ScrapeWorkPage, error) {
	page := organizer.ScrapeWorkPage{Items: []organizer.ScrapeWork{}}
	if len(search) > 200 {
		return page, errors.New("搜索内容过长")
	}
	search = strings.TrimSpace(search)
	var cursorKind string
	var cursorID int
	if cursor != "" {
		parts := strings.Split(cursor, ":")
		if len(parts) != 2 || (parts[0] != "movie" && parts[0] != "tv") {
			return page, errors.New("作品游标无效")
		}
		id, err := strconv.Atoi(parts[1])
		if err != nil || id <= 0 || strconv.Itoa(id) != parts[1] {
			return page, errors.New("作品游标无效")
		}
		cursorKind, cursorID = parts[0], id
	}
	rows, err := a.db.Query(`SELECT json_extract(value,'$.type') kind,
	 CAST(json_extract(value,'$.tmdbId') AS INTEGER) id,
	 MIN(path) representative, COUNT(*) total,
	 SUM(CASE WHEN json_extract(value,'$.state')='pending' THEN 1 ELSE 0 END) pending,
	 SUM(CASE WHEN json_extract(value,'$.state')='failed' THEN 1 ELSE 0 END) failed,
	 SUM(CASE WHEN json_extract(value,'$.state')='completed' THEN 1 ELSE 0 END) completed
	 FROM media_mappings
	 WHERE (json_extract(value,'$.type')>? OR (json_extract(value,'$.type')=? AND CAST(json_extract(value,'$.tmdbId') AS INTEGER)>?))
	 AND (?='' OR instr(lower(CAST(json_extract(value,'$.tmdbId') AS TEXT)),lower(?))>0 OR instr(lower(path),lower(?))>0)
	 GROUP BY kind,id ORDER BY kind,id LIMIT 51`, cursorKind, cursorKind, cursorID, search, search, search)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item organizer.ScrapeWork
		if err := rows.Scan(&item.Type, &item.TMDBID, &item.Representative, &item.Total, &item.Pending, &item.Failed, &item.Completed); err != nil {
			return page, err
		}
		if len(page.Items) == 50 {
			page.NextCursor = fmt.Sprintf("%s:%d", page.Items[49].Type, page.Items[49].TMDBID)
			break
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func (a *Store) ScrapeWorkMappings(kind string, id int) ([]model.MediaMapping, error) {
	if (kind != "movie" && kind != "tv") || id <= 0 {
		return nil, errors.New("作品无效")
	}
	rows, err := a.db.Query("SELECT value FROM media_mappings WHERE json_extract(value,'$.type')=? AND json_extract(value,'$.tmdbId')=? ORDER BY path", kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.MediaMapping{}
	for rows.Next() {
		var raw string
		var item model.MediaMapping
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := decode(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

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
