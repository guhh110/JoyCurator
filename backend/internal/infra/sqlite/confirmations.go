package sqlite

import "joycurator/internal/matching"

func writeConfirmation(q queryer, owner, root string, c *matching.Confirmation) error {
	if _, err := q.Exec("DELETE FROM workspace_confirmations WHERE owner=?", owner); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	b := c.Basis
	if _, err := q.Exec("INSERT INTO workspace_confirmations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", owner, root, c.Source, c.At, c.Identity, c.Size, c.MTime, c.Type, c.TMDBID, c.Season, b.Kind, b.TraceID, b.CandidateID, b.SourceID, b.Index, b.Pattern, b.Offset); err != nil {
		return err
	}
	for i, e := range c.Episodes {
		if _, err := q.Exec("INSERT INTO confirmation_episodes VALUES(?,?,?)", owner, i, e); err != nil {
			return err
		}
	}
	return nil
}
func readConfirmations(q queryer, root string) (map[string]*matching.Confirmation, error) {
	out := map[string]*matching.Confirmation{}
	rows, err := q.Query("SELECT owner,f_source,f_at,f_identity,f_size,f_m_time,f_type,f_tmdbid,f_season,f_basis_kind,f_trace_id,f_candidate_id,f_source_id,f_digit_index,f_pattern,f_offset FROM workspace_confirmations WHERE root=?", root)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		c := &matching.Confirmation{Episodes: []int{}}
		b := &c.Basis
		if err = rows.Scan(&key, &c.Source, &c.At, &c.Identity, &c.Size, &c.MTime, &c.Type, &c.TMDBID, &c.Season, &b.Kind, &b.TraceID, &b.CandidateID, &b.SourceID, &b.Index, &b.Pattern, &b.Offset); err != nil {
			rows.Close()
			return nil, err
		}
		out[key] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query("SELECT e.owner,e.value FROM confirmation_episodes e JOIN workspace_confirmations c ON c.owner=e.owner WHERE c.root=? ORDER BY e.owner,e.position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var v int
		if err = rows.Scan(&key, &v); err != nil {
			return nil, err
		}
		out[key].Episodes = append(out[key].Episodes, v)
	}
	return out, rows.Err()
}
