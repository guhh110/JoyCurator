package sqlite

import (
	"context"
	"database/sql"
	"errors"

	_ "modernc.org/sqlite"
)

func (a *Store) SaveCache(ctx context.Context, key string, data []byte, expires int64) error {
	_, err := a.db.ExecContext(ctx, "INSERT INTO tmdb_cache(key,value,expires_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,expires_at=excluded.expires_at", key, data, expires)
	return err
}
func (a *Store) LoadCache(ctx context.Context, key string, now int64) ([]byte, error) {
	var data []byte
	err := a.db.QueryRowContext(ctx, "SELECT value FROM tmdb_cache WHERE key=? AND expires_at>?", key, now).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return data, err
}
