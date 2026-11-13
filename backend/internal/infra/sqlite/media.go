package sqlite

import (
	"errors"
	"fmt"
	"slices"
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
	rows, err := a.db.Query(`SELECT f_type kind,f_tmdbid id,MIN(f_path) representative,COUNT(*) total,
 SUM(CASE WHEN f_state='pending' THEN 1 ELSE 0 END) pending,
 SUM(CASE WHEN f_state='failed' THEN 1 ELSE 0 END) failed,
 SUM(CASE WHEN f_state='completed' THEN 1 ELSE 0 END) completed
 FROM media_mappings WHERE (f_type>? OR (f_type=? AND f_tmdbid>?))
 AND (?='' OR instr(lower(CAST(f_tmdbid AS TEXT)),lower(?))>0 OR instr(lower(f_path),lower(?))>0)
 GROUP BY f_type,f_tmdbid ORDER BY f_type,f_tmdbid LIMIT 51`, cursorKind, cursorKind, cursorID, search, search, search)
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
	return snapshotRead(a.db, func(q queryer) ([]model.MediaMapping, error) {
		return readMappings(q, "WHERE f_type=? AND f_tmdbid=? ORDER BY f_path", kind, id)
	})
}
func (a *Store) SaveMapping(v model.MediaMapping) error {
	return transaction(a.db, func(tx queryer) error { return saveMapping(tx, v) })
}
func saveMapping(q queryer, v model.MediaMapping) error {
	previous, err := mapping(q, v.Path)
	if err != nil {
		return err
	}
	if previous != nil {
		if previous.RunID != v.RunID {
			var newer bool
			if err = q.QueryRow("SELECT incoming.sequence>previous.sequence FROM runs incoming,runs previous WHERE incoming.id=? AND previous.id=?", v.RunID, previous.RunID).Scan(&newer); err != nil {
				return err
			}
			if !newer {
				return errors.New("目标视频已有更新的整理映射：" + v.Path)
			}
		}
		if previous.Identity == v.Identity && previous.Type == v.Type && previous.TMDBID == v.TMDBID && previous.Season == v.Season && slices.Equal(previous.Episodes, v.Episodes) {
			v.State = previous.State
		}
	}
	return writeMediaMappings(q, v.Path, v.Path, "", 0, v)
}
func readMappings(q queryer, where string, args ...any) ([]model.MediaMapping, error) {
	rows, err := q.Query("SELECT "+columnsMediaMappings+" FROM media_mappings "+where, args...)
	if err != nil {
		return nil, err
	}
	out := []model.MediaMapping{}
	for rows.Next() {
		v, e := scanMediaMappings(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Fetch the selected episode sets in one query, irrespective of page size.
	rows, err = q.Query("SELECT e.owner,e.value FROM mapping_episodes e JOIN media_mappings m ON m.id=e.owner WHERE m.id IN (SELECT id FROM media_mappings "+where+") ORDER BY e.owner,e.position", args...)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Path] = i
		out[i].Episodes = []int{}
	}
	for rows.Next() {
		var owner string
		var n int
		if err = rows.Scan(&owner, &n); err != nil {
			return nil, err
		}
		out[index[owner]].Episodes = append(out[index[owner]].Episodes, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	works, err := readWorks(q, "mapping", "media_mappings", "id", where, args...)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Entry = works[out[i].Path]
		if out[i].Entry == nil {
			return nil, errors.New("已登记视频缺少已确认作品快照")
		}
	}
	return out, nil
}
func mapping(q queryer, path string) (*model.MediaMapping, error) {
	items, err := readMappings(q, "WHERE id=?", path)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}
func (a *Store) Mappings() ([]model.MediaMapping, error) {
	return snapshotRead(a.db, func(q queryer) ([]model.MediaMapping, error) { return readMappings(q, "ORDER BY f_path") })
}
func (a *Store) Mapping(path string) (*model.MediaMapping, error) {
	return snapshotRead(a.db, func(q queryer) (*model.MediaMapping, error) { return mapping(q, path) })
}
func (a *Store) DeleteMapping(v model.MediaMapping) error {
	_, err := a.db.Exec("DELETE FROM media_mappings WHERE id=? AND f_run_id=? AND f_identity=?", v.Path, v.RunID, v.Identity)
	return err
}
func (a *Store) UpdateMappingState(v model.MediaMapping, state, runID string) error {
	_, err := a.db.Exec(`UPDATE media_mappings SET f_state=? WHERE id=? AND f_run_id=? AND f_identity=? AND NOT EXISTS(SELECT 1 FROM runs newer JOIN run_mappings m ON m.root=newer.id WHERE newer.f_purpose='scrape' AND newer.f_state<>'undone' AND m.f_path=? AND newer.sequence>(SELECT sequence FROM runs WHERE id=?))`, state, v.Path, v.RunID, v.Identity, v.Path, runID)
	return err
}
