package sqlite

import (
	"joycurator/internal/recognition"
	_ "modernc.org/sqlite"
)

func (a *Store) SaveRules(rules []recognition.Rule) error {
	_, err := a.db.Exec("UPDATE extraction_rules SET value=? WHERE id=1", jsonText(rules))
	return err
}
func (a *Store) LoadRules() ([]recognition.Rule, error) {
	var raw string
	err := a.db.QueryRow("SELECT value FROM extraction_rules WHERE id=1").Scan(&raw)
	if err != nil {
		return nil, err
	}
	var rules []recognition.Rule
	err = decode(raw, &rules)
	return rules, err
}
