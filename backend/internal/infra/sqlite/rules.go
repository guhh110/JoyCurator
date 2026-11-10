package sqlite

import (
	"errors"
	"joycurator/internal/recognition"
)

func (a *Store) SaveRules(v recognition.Pipeline) error {
	return transaction(a.db, func(tx queryer) error { return saveRules(tx, v) })
}
func saveRules(q queryer, v recognition.Pipeline) error {
	if _, err := recognition.Compile(v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM rules WHERE owner='current'"); err != nil {
		return err
	}
	for i, r := range v.Rules() {
		if err := writeRules(q, childKey("current", i), "current", "current", i, r); err != nil {
			return err
		}
	}
	return nil
}
func (a *Store) LoadRules() (recognition.Pipeline, error) { return snapshotRead(a.db, loadRules) }
func loadRules(q queryer) (recognition.Pipeline, error) {
	var id string
	p := recognition.Pipeline{Annotators: []recognition.Rule{}}
	if err := q.QueryRow("SELECT id FROM rule_chain WHERE id='current'").Scan(&id); err != nil {
		return p, err
	}
	rows, err := readRules(q, "current")
	if err != nil {
		return p, err
	}
	for _, r := range rows {
		switch r.Value.Stage {
		case "tokenize":
			if p.Tokenizer.ID != "" {
				return p, errors.New("重复分词规则")
			}
			p.Tokenizer = r.Value
		case "annotate":
			p.Annotators = append(p.Annotators, r.Value)
		case "resolve":
			if p.Resolver.ID != "" {
				return p, errors.New("重复裁决规则")
			}
			p.Resolver = r.Value
		default:
			return p, errors.New("未知规则阶段")
		}
	}
	_, err = recognition.Compile(p)
	return p, err
}
