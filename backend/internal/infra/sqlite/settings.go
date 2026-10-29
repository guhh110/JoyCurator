package sqlite

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"joycurator/internal/model"
	_ "modernc.org/sqlite"
)

func (a *Store) SaveSettings(s model.Settings) error {
	_, err := a.db.Exec("UPDATE config SET value=?,token=? WHERE id=1", jsonText(s), s.Token)
	return err
}
func (a *Store) LoadSettings() (model.Settings, error) {
	var s model.Settings
	var raw string
	err := a.db.QueryRow("SELECT value,token FROM config WHERE id=1").Scan(&raw, &s.Token)
	if err != nil {
		return s, err
	}
	token := s.Token
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return s, err
	}
	if !filepath.IsAbs(s.SourceRoot) || !filepath.IsAbs(s.TargetRoot) || s.Version < 1 {
		return s, errors.New("源目录、目标目录或设置版本无效")
	}
	s.Token = token
	return s, nil
}
