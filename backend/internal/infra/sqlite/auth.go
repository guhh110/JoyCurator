package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"joycurator/internal/auth"
	_ "modernc.org/sqlite"
)

func (a *Store) AdministratorExists(ctx context.Context) (bool, error) {
	var n int
	err := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM administrators").Scan(&n)
	return n > 0, err
}
func (a *Store) CreateAdministrator(ctx context.Context, name string, hash []byte) error {
	r, err := a.db.ExecContext(ctx, "INSERT OR IGNORE INTO administrators(id,username,password_hash) VALUES(1,?,?)", name, hash)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n == 0 {
		return auth.ErrInitialized
	}
	return err
}
func (a *Store) Administrator(ctx context.Context) (auth.User, []byte, error) {
	var u auth.User
	var hash []byte
	err := a.db.QueryRowContext(ctx, "SELECT username,password_hash FROM administrators WHERE id=1").Scan(&u.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		err = auth.ErrCredentials
	}
	return u, hash, err
}
func (a *Store) CreateSession(ctx context.Context, hash string, now, expires int64) error {
	if _, err := a.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx, "INSERT INTO sessions(token_hash,administrator_id,expires_at) VALUES(?,1,?)", hash, expires)
	return err
}
func (a *Store) SessionUser(ctx context.Context, hash string, now int64) (auth.User, error) {
	var u auth.User
	err := a.db.QueryRowContext(ctx, `SELECT a.username FROM sessions s JOIN administrators a ON a.id=s.administrator_id WHERE s.token_hash=? AND s.expires_at>?`, hash, now).Scan(&u.Username)
	if errors.Is(err, sql.ErrNoRows) {
		err = auth.ErrCredentials
	}
	return u, err
}
func (a *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := a.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", hash)
	return err
}
