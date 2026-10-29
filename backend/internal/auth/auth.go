package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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

type Store interface {
	AdministratorExists(context.Context) (bool, error)
	CreateAdministrator(context.Context, string, []byte) error
	Administrator(context.Context) (User, []byte, error)
	CreateSession(context.Context, string, int64, int64) error
	SessionUser(context.Context, string, int64) (User, error)
	DeleteSession(context.Context, string) error
}
type Service struct{ Store Store }

func New(store Store) *Service { return &Service{Store: store} }

type User struct {
	Username string `json:"username"`
}

func (s *Service) Initialized(ctx context.Context) (bool, error) {
	return s.Store.AdministratorExists(ctx)
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
	return s.Store.CreateAdministrator(ctx, username, hash)
}

func (s *Service) Login(ctx context.Context, username, password string) (string, User, error) {
	user, hash, err := s.Store.Administrator(ctx)
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
	err = s.Store.CreateSession(ctx, tokenHash(token), time.Now().Unix(), time.Now().Add(SessionLifetime).Unix())
	return token, user, err
}

func (s *Service) Session(ctx context.Context, token string) (User, error) {
	var user User
	if len(token) != 64 {
		return user, ErrCredentials
	}
	return s.Store.SessionUser(ctx, tokenHash(token), time.Now().Unix())
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.DeleteSession(ctx, tokenHash(token))
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
