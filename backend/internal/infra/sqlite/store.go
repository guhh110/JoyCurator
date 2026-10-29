package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	_ "modernc.org/sqlite"
)

func (a *Store) Close() error                   { return a.db.Close() }
func (a *Store) Ping(ctx context.Context) error { return a.db.PingContext(ctx) }

type Store struct{ db *sql.DB }

func Open(dir string, initial model.Settings) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "joycurator.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;
 CREATE TABLE IF NOT EXISTS administrators(id INTEGER PRIMARY KEY CHECK(id=1),username TEXT NOT NULL,password_hash BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY,administrator_id INTEGER NOT NULL REFERENCES administrators(id),expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS config(id INTEGER PRIMARY KEY CHECK(id=1),value TEXT NOT NULL,token TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS tmdb_cache(key TEXT PRIMARY KEY,value BLOB NOT NULL,expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS recognitions(id TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS extraction_rules(id INTEGER PRIMARY KEY CHECK(id=1),value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS extraction_traces(id TEXT PRIMARY KEY,path TEXT NOT NULL,root TEXT NOT NULL,workspace_id TEXT NOT NULL,status TEXT NOT NULL,started_at TEXT NOT NULL,finished_at TEXT NOT NULL DEFAULT '',metadata TEXT NOT NULL DEFAULT '{}',error TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS extraction_steps(trace_id TEXT NOT NULL REFERENCES extraction_traces(id) ON DELETE CASCADE,position INTEGER NOT NULL,value TEXT NOT NULL,PRIMARY KEY(trace_id,position));
 CREATE INDEX IF NOT EXISTS extraction_trace_workspace ON extraction_traces(workspace_id);
 UPDATE extraction_traces SET status='interrupted',error='服务中断，保留已完成步骤' WHERE status='running';`); err != nil {
		return fail(err)
	}
	if err = os.Chmod(filepath.Join(dir, "joycurator.db"), 0600); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO config VALUES(1,?, '')", jsonText(initial)); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO extraction_rules(id,value) VALUES(1,?)", jsonText(recognition.DefaultRules())); err != nil {
		return fail(err)
	}
	a := &Store{db: db}
	if _, err = a.LoadSettings(); err != nil {
		return fail(err)
	}
	runs, err := a.AllRuns()
	if err != nil {
		return fail(err)
	}
	for _, r := range runs {
		if r.State == "running" || r.State == "queued" {
			r.State = "interrupted"
			r.Error = "服务中断，可继续或撤销"
			if err = a.SaveRun(r); err != nil {
				return fail(err)
			}
		}
	}
	if _, err = db.Exec(`UPDATE recognitions SET value=json_set(value,'$.state','interrupted','$.error','服务中断，可重试未完成项') WHERE json_extract(value,'$.state')='running'`); err != nil {
		return fail(err)
	}
	return a, nil
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }

func decode(raw string, value any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
