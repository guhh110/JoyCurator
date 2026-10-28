package app

import (
	"database/sql"
	"encoding/json"
	"joycurator/internal/model"
)

// Replace the previous per-media roots with startup defaults without moving files.
// Tokens and operation records are kept; settings versions invalidate stale previews.
func migrateDirectories(db *sql.DB, defaults model.Settings) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err = tx.QueryRow("SELECT value FROM config WHERE id=1").Scan(&raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal([]byte(raw), &fields); err != nil {
		return err
	}
	if fields["sourceRoot"] == nil && (fields["movieRoot"] != nil || fields["tvRoot"] != nil) {
		var settings model.Settings
		if err = json.Unmarshal([]byte(raw), &settings); err != nil {
			return err
		}
		settings.SourceRoot, settings.TargetRoot = defaults.SourceRoot, defaults.TargetRoot
		settings.Version++
		categories := []model.Category{}
		for _, category := range settings.Rules.Categories {
			for _, kind := range []string{"movie", "tv"} {
				if category.Type != "" && category.Type != kind {
					continue
				}
				next := category
				next.Type = kind
				prefix := "电影/"
				if kind == "tv" {
					prefix = "剧集/"
				}
				next.Name = prefix + category.Name
				categories = append(categories, next)
			}
		}
		if len(categories) == 0 {
			categories = model.DefaultCategories()
		}
		settings.Rules.Categories = categories
		if _, err = tx.Exec("UPDATE config SET value=? WHERE id=1", jsonText(settings)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("PRAGMA user_version=2"); err != nil {
		return err
	}
	return tx.Commit()
}
