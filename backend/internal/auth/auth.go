package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInitialized = errors.New("administrator already initialized")
	ErrCredentials = errors.New("invalid credentials")
	ErrValidation  = errors.New("username must be 1-64 characters and password 12-72 bytes")
)

const SessionLifetime = 7 * 24 * time.Hour

type Service struct{ DB *sql.DB }
type User struct {
	Username string `json:"username"`
}

func (s *Service) Initialized(ctx context.Context) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM administrators").Scan(&n)
	return n > 0, err
}

func (s *Service) Setup(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if len(username) < 1 || len(username) > 64 || len(password) < 12 || len(password) > 72 {
		return ErrValidation
	}
	initialized, err := s.Initialized(ctx)
	if err != nil {
		return err
	}
	if initialized {
		return ErrInitialized
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	r, err := s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO administrators(id,username,password_hash) VALUES(1,?,?)", username, hash)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInitialized
	}
	return nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, User, error) {
	var user User
	var hash []byte
	err := s.DB.QueryRowContext(ctx, "SELECT username,password_hash FROM administrators WHERE id=1").Scan(&user.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", user, ErrCredentials
	}
	if err != nil {
		return "", user, err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil || user.Username != strings.TrimSpace(username) {
		return "", User{}, ErrCredentials
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", User{}, err
	}
	token := hex.EncodeToString(buf)
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now().Unix()); err != nil {
		return "", User{}, err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO sessions(token_hash,administrator_id,expires_at) VALUES(?,1,?)", tokenHash(token), time.Now().Add(SessionLifetime).Unix())
	return token, user, err
}

func (s *Service) Session(ctx context.Context, token string) (User, error) {
	var user User
	if len(token) != 64 {
		return user, ErrCredentials
	}
	err := s.DB.QueryRowContext(ctx, `SELECT a.username FROM sessions s JOIN administrators a ON a.id=s.administrator_id WHERE s.token_hash=? AND s.expires_at>?`, tokenHash(token), time.Now().Unix()).Scan(&user.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return user, ErrCredentials
	}
	return user, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", tokenHash(token))
	return err
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
