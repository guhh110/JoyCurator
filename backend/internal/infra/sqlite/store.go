package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
	_ "modernc.org/sqlite"
)

const formatVersion = 5

type Store struct{ db *sql.DB }

func (a *Store) Close() error                   { return a.db.Close() }
func (a *Store) Ping(ctx context.Context) error { return a.db.PingContext(ctx) }

func Open(dir string, initial model.Settings) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "joycurator.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, fmt.Errorf("数据库 %s：%w", path, err) }
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON"); err != nil {
		return fail(err)
	}
	var tables, version int
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return fail(err)
	}
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	a := &Store{db: db}
	if tables == 0 && version == 0 {
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		e = func() error {
			defer tx.Rollback()
			if _, e := tx.Exec(schema); e != nil {
				return e
			}
			if _, e := tx.Exec("PRAGMA user_version=5; INSERT INTO rule_chain(id) VALUES('current')"); e != nil {
				return e
			}
			if e := writeConfig(tx, "1", "1", "", 0, initial); e != nil {
				return e
			}
			if e := saveRules(tx, recognition.DefaultPipeline()); e != nil {
				return e
			}
			if _, e := loadSettings(tx); e != nil {
				return e
			}
			return tx.Commit()
		}()
		if e != nil {
			return fail(e)
		}
	} else {
		if version != formatVersion {
			return fail(errors.New("存储格式不符；不支持迁移，未自动重建数据"))
		}
		if err = validateSchema(db); err != nil {
			return fail(err)
		}
	}
	if _, err = a.LoadRules(); err != nil {
		return fail(err)
	}
	if err = validateReferences(db); err != nil {
		return fail(err)
	}
	if _, err = a.LoadSettings(); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fail(err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		return fail(err)
	}
	if err = a.recoverQueue(); err != nil {
		return fail(err)
	}
	return a, nil
}

func validateSchema(db *sql.DB) error {
	rows, err := db.Query("SELECT name,sql FROM sqlite_schema WHERE sql IS NOT NULL")
	if err != nil {
		return err
	}
	actual := map[string]string{}
	for rows.Next() {
		var n, s string
		if err = rows.Scan(&n, &s); err != nil {
			rows.Close()
			return err
		}
		actual[n] = strings.Join(strings.Fields(s), " ")
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, stmt := range strings.Split(schema, ";") {
		words := strings.Fields(stmt)
		if len(words) == 0 {
			continue
		}
		nameIndex := 2
		if words[1] == "UNIQUE" {
			nameIndex = 3
		}
		name := strings.Split(words[nameIndex], "(")[0]
		if actual[name] != strings.Join(words, " ") {
			return fmt.Errorf("存储结构不符：%s；未自动修复", name)
		}
	}
	return nil
}

func transaction(db *sql.DB, fn func(queryer) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	session := &writeSession{Tx: tx, statements: map[string]*sql.Stmt{}}
	defer session.close()
	if err = fn(session); err != nil {
		return err
	}
	return tx.Commit()
}
func childKey(owner string, position int) string {
	return strconv.Itoa(len(owner)) + ":" + owner + ":" + strconv.Itoa(position)
}

type prefixScanner struct {
	s      scanner
	prefix []any
}

func (p prefixScanner) Scan(args ...any) error { return p.s.Scan(append(p.prefix, args...)...) }

// JSON is reserved for values whose shape belongs to user scripts, not records.
func dynamicJSON(v any) (any, error) {
	if raw, ok := v.(json.RawMessage); ok && len(raw) == 0 {
		return nil, nil
	}
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}
func decode(raw string, value any) error {
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("JSON 后存在额外值")
		}
		return err
	}
	return nil
}

func splitColumns(s string) []string { return strings.Split(s, ",") }

// Reuse repeated fixed statements only for the lifetime of one transaction.
// This particularly matters when storing thousands of ordered child rows.
type writeSession struct {
	*sql.Tx
	statements map[string]*sql.Stmt
}

func (s *writeSession) Exec(query string, args ...any) (sql.Result, error) {
	if strings.Contains(query, ";") {
		return s.Tx.Exec(query, args...)
	}
	stmt, ok := s.statements[query]
	if !ok {
		var err error
		stmt, err = s.Prepare(query)
		if err != nil {
			return nil, err
		}
		s.statements[query] = stmt
	}
	return stmt.Exec(args...)
}
func (s *writeSession) close() {
	for _, stmt := range s.statements {
		stmt.Close()
	}
}

func validateReferences(db *sql.DB) error {
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("存储外键引用不一致；未自动修复")
	}
	return rows.Err()
}

func snapshotRead[T any](db *sql.DB, fn func(queryer) (T, error)) (T, error) {
	var zero T
	tx, err := db.Begin()
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	value, err := fn(tx)
	if err != nil {
		return zero, err
	}
	if err = tx.Commit(); err != nil {
		return zero, err
	}
	return value, nil
}
