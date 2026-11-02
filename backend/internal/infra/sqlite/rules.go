package sqlite

import (
	"joycurator/internal/recognition"
)

func (a *Store) SaveRules(v []recognition.Rule) error {
	return transaction(a.db, func(tx queryer) error { return saveRules(tx, v) })
}
func saveRules(q queryer, v []recognition.Rule) error {
	if _, err := q.Exec("DELETE FROM rules WHERE owner='current'"); err != nil {
		return err
	}
	for i, r := range v {
		if err := writeRules(q, childKey("current", i), "current", "current", i, r); err != nil {
			return err
		}
	}
	return nil
}
func (a *Store) LoadRules() ([]recognition.Rule, error) { return snapshotRead(a.db, loadRules) }
func loadRules(q queryer) ([]recognition.Rule, error) {
	var id string
	if err := q.QueryRow("SELECT id FROM rule_chain WHERE id='current'").Scan(&id); err != nil {
		return nil, err
	}
	rows, err := readRules(q, "current")
	if err != nil {
		return nil, err
	}
	out := []recognition.Rule{}
	for _, r := range rows {
		out = append(out, r.Value)
	}
	return out, nil
}
