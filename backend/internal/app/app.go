// Package app owns the small HTTP application. Domain helpers stay stateless.
package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"joycurator/internal/auth"
	"joycurator/internal/fsops"
	"joycurator/internal/model"
	"joycurator/internal/tmdb"
	_ "modernc.org/sqlite"
)

type App struct {
	db      *sql.DB
	access  *fsops.FS
	auth    *auth.Service
	mu      sync.Mutex
	plans   map[string]*Plan
	running string
	cancel  context.CancelFunc
	closed  bool
	wg      sync.WaitGroup
	secure  bool
}
type Row struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Season    int      `json:"season"`
	Episodes  []int    `json:"episodes"`
	Artifacts []string `json:"artifacts"`
	Issues    []string `json:"issues"`
}
type Work struct {
	model.Operation
	Owner  string `json:"owner"`
	Reused bool   `json:"reused,omitempty"`
}
type Plan struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Mode     string    `json:"mode"`
	Rows     []Row     `json:"rows"`
	Required int64     `json:"required"`
	Issues   []string  `json:"issues"`
	Version  int       `json:"-"`
	Expires  time.Time `json:"-"`
	Ops      []Work    `json:"-"`
}
type Run struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Mode      string `json:"mode"`
	State     string `json:"state"`
	Created   string `json:"created"`
	Error     string `json:"error"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Undo      bool   `json:"undo"`
	Ops       []Work `json:"ops,omitempty"`
}

func Open(dir string, roots []string, secure bool) (*App, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	access, err := fsops.New(roots)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "joycurator.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var legacy, version int
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('settings','records','schema_migrations')").Scan(&legacy); err != nil {
		db.Close()
		return nil, err
	}
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if legacy != 0 || version > 2 {
		db.Close()
		return nil, errors.New("此数据目录属于其他版本，请使用独立的数据目录")
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;
 CREATE TABLE IF NOT EXISTS administrators(id INTEGER PRIMARY KEY CHECK(id=1),username TEXT NOT NULL,password_hash BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY,administrator_id INTEGER NOT NULL REFERENCES administrators(id),expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS config(id INTEGER PRIMARY KEY CHECK(id=1),value TEXT NOT NULL,token TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS tmdb_cache(key TEXT PRIMARY KEY,value BLOB NOT NULL,expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,value TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(filepath.Join(dir, "joycurator.db"), 0600); err != nil {
		db.Close()
		return nil, err
	}
	setting := model.DefaultSettings()
	if len(access.Roots) == 0 {
		db.Close()
		return nil, errors.New("请配置源目录和目标目录")
	}
	if len(access.Roots) >= 2 {
		setting.SourceRoot, setting.TargetRoot = access.Roots[0], access.Roots[1]
	} else {
		setting.SourceRoot, setting.TargetRoot = filepath.Join(access.Roots[0], "source"), filepath.Join(access.Roots[0], "target")
	}
	if fsops.Within(setting.TargetRoot, setting.SourceRoot) {
		db.Close()
		return nil, errors.New("源目录不能位于目标目录内或与其相同")
	}
	_, err = db.Exec("INSERT OR IGNORE INTO config VALUES(1,?, '')", jsonText(setting))
	if err == nil {
		err = migrateDirectories(db, setting)
	}
	if err != nil {
		db.Close()
		return nil, err
	}

	a := &App{db: db, access: access, auth: &auth.Service{DB: db}, plans: map[string]*Plan{}, secure: secure}
	// Persisted operation states contain staging paths and hashes for recovery.
	rows, err := db.Query("SELECT value FROM runs")
	if err != nil {
		db.Close()
		return nil, err
	}
	interrupted := []Run{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			break
		}
		var r Run
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			break
		}
		if r.State == "running" {
			r.State = "interrupted"
			r.Error = "服务中断，可继续或撤销"
			interrupted = append(interrupted, r)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		db.Close()
		return nil, err
	}
	for _, r := range interrupted {
		if err = a.save(r); err != nil {
			db.Close()
			return nil, err
		}
	}
	return a, nil
}
func (a *App) Close() error {
	a.mu.Lock()
	a.closed = true
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	return a.db.Close()
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func key() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (a *App) settings() (model.Settings, error) {
	var s model.Settings
	var raw string
	err := a.db.QueryRow("SELECT value,token FROM config WHERE id=1").Scan(&raw, &s.Token)
	if err != nil {
		return s, err
	}
	token := s.Token
	err = json.Unmarshal([]byte(raw), &s)
	s.Token = token
	return s, err
}
func (a *App) client(s model.Settings) (*tmdb.Client, error) {
	return tmdb.New(a.db, s.Token, s.ProxyURL, s.Language)
}
func (a *App) save(r Run) error {
	_, err := a.db.Exec("INSERT INTO runs(id,value) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", r.ID, jsonText(r))
	return err
}
func (a *App) record(id string) (Run, error) {
	var r Run
	var raw string
	err := a.db.QueryRow("SELECT value FROM runs WHERE id=?", id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &r)
	}
	return r, err
}
func (a *App) records(page int) (map[string]any, error) {
	var total int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM runs").Scan(&total); err != nil {
		return nil, err
	}
	rows, err := a.db.Query("SELECT value FROM runs ORDER BY rowid DESC LIMIT 20 OFFSET ?", (page-1)*20)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Run{}
	for rows.Next() {
		var raw string
		var r Run
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		r.Ops = nil
		items = append(items, r)
	}
	return map[string]any{"items": items, "total": total, "page": page}, rows.Err()
}

var errBusy = errors.New("已有任务正在运行，请等待完成")
