package sqlite

import (
	"errors"
	"joycurator/internal/model"
)

// Snapshots use the same scalar schema as mappings and relational episode rows.
// The scope is internal, never supplied by HTTP input.
func writeMappingSnapshot(q queryer, scope, owner, root string, v *model.MediaMapping) error {
	if _, err := q.Exec("DELETE FROM "+scope+"_originals WHERE owner=?", owner); err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	_, err := q.Exec("INSERT INTO "+scope+"_originals(owner,root,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", owner, root, v.Path, v.RunID, v.Source, v.Type, v.TMDBID, v.Season, v.State, v.Identity, v.Size, v.MTime, v.Layout.WorkDir, v.Layout.SeasonDir, v.Layout.MoviePrefix, v.Layout.Manual)
	if err != nil {
		return err
	}
	for i, ep := range v.Episodes {
		if _, err = q.Exec("INSERT INTO "+scope+"_original_episodes(owner,position,value) VALUES(?,?,?)", owner, i, ep); err != nil {
			return err
		}
	}
	return writeWork(q, scope+"_original", owner, v.Entry)
}
func readMappingSnapshots(q queryer, scope, root string) (map[string]*model.MediaMapping, error) {
	rows, err := q.Query("SELECT owner,"+columnsMediaMappings+" FROM "+scope+"_originals WHERE root=?", root)
	if err != nil {
		return nil, err
	}
	out := map[string]*model.MediaMapping{}
	for rows.Next() {
		var key string
		v, e := scanMediaMappings(prefixScanner{rows, []any{&key}})
		if e != nil {
			rows.Close()
			return nil, e
		}
		v.Episodes = []int{}
		out[key] = &v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query("SELECT e.owner,e.value FROM "+scope+"_original_episodes e JOIN "+scope+"_originals o ON o.owner=e.owner WHERE o.root=? ORDER BY e.owner,e.position", root)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var ep int
		if err = rows.Scan(&key, &ep); err != nil {
			return nil, err
		}
		out[key].Episodes = append(out[key].Episodes, ep)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	works, err := readWorks(q, scope+"_original", scope+"_originals", "owner", "WHERE p.root=?", root)
	if err != nil {
		return nil, err
	}
	for key, v := range out {
		v.Entry = works[key]
		if v.Entry == nil {
			return nil, errors.New("原登记缺少已确认作品快照")
		}
	}
	return out, nil
}
