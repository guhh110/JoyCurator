package sqlite

import "joycurator/internal/model"

const expiredTraces = " FROM extraction_traces WHERE f_status IN ('completed','failed','cancelled','interrupted') AND julianday(f_finished_at)<julianday(?) AND NOT EXISTS(SELECT 1 FROM workspace_items WHERE f_trace_id=extraction_traces.id) AND NOT EXISTS(SELECT 1 FROM workspace_confirmations WHERE f_trace_id=extraction_traces.id)"
const expiredCache = " FROM tmdb_cache WHERE expires_at<=?"

func (a *Store) PreviewCleanup(r model.CleanupRequest) (model.CleanupPreview, error) {
	v := model.CleanupPreview{Request: r}
	err := transaction(a.db, func(q queryer) error {
		if r.Traces {
			if err := q.QueryRow("SELECT COUNT(*)"+expiredTraces, r.Before).Scan(&v.Traces); err != nil {
				return err
			}
		}
		if r.Cache {
			return q.QueryRow("SELECT COUNT(*)"+expiredCache, r.CacheBefore).Scan(&v.Cache)
		}
		return nil
	})
	return v, err
}

// Both selections commit together. Foreign keys cascade through every trace
// child table; workspace results and their clearable history references stay intact.
func (a *Store) Cleanup(r model.CleanupRequest) (model.CleanupCounts, error) {
	v := model.CleanupCounts{}
	err := transaction(a.db, func(q queryer) error {
		if r.Traces {
			result, err := q.Exec("DELETE"+expiredTraces, r.Before)
			if err != nil {
				return err
			}
			if v.Traces, err = result.RowsAffected(); err != nil {
				return err
			}
		}
		if r.Cache {
			result, err := q.Exec("DELETE"+expiredCache, r.CacheBefore)
			if err != nil {
				return err
			}
			if v.Cache, err = result.RowsAffected(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.CleanupCounts{}, err
	}
	return v, nil
}
