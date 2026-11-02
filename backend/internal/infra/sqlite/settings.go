package sqlite

import (
	"errors"
	"joycurator/internal/model"
	"path/filepath"
)

func (a *Store) SaveSettings(v model.Settings) error {
	return transaction(a.db, func(tx queryer) error { return writeConfig(tx, "1", "1", "", 0, v) })
}
func (a *Store) LoadSettings() (model.Settings, error) { return snapshotRead(a.db, loadSettings) }
func loadSettings(q queryer) (model.Settings, error) {
	v, err := scanConfig(q.QueryRow("SELECT " + columnsConfig + " FROM config WHERE id='1'"))
	if err != nil {
		return v, err
	}
	cats, err := readConfigCategories(q, "1")
	if err != nil {
		return v, err
	}
	v.Rules.Categories = []model.Category{}
	for _, c := range cats {
		v.Rules.Categories = append(v.Rules.Categories, c.Value)
	}
	if !filepath.IsAbs(v.SourceRoot) || !filepath.IsAbs(v.TargetRoot) || v.Version < 1 {
		return v, errors.New("源目录、目标目录或设置版本无效")
	}
	return v, nil
}
