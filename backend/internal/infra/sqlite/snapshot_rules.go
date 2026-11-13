package sqlite

import (
	"database/sql"
	"fmt"
	"joycurator/internal/recognition"
)

func scanSnapshotRules(s scanner) (recognition.Rule, error) {
	var v recognition.Rule
	var err error
	var raw11 sql.NullString
	var raw14 sql.NullString
	err = s.Scan(&v.ID, &v.Name, &v.Description, &v.Target, &v.Source, &v.Order, &v.Enabled, &v.Kind, &v.Stage, &v.Config.Code, &raw11, &v.Config.Pattern, &v.Config.Capture, &raw14)
	if err != nil {
		return v, err
	}
	if raw11.Valid {
		if err = decode(raw11.String, &v.Config.Params); err != nil {
			return v, err
		}
	}
	if raw14.Valid {
		if err = decode(raw14.String, &v.Config.Value); err != nil {
			return v, err
		}
	}
	return v, nil
}

func putSnapshotRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	j11, err := dynamicJSON(v.Config.Params)
	if err != nil {
		return err
	}
	j14, err := dynamicJSON(v.Config.Value)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO snapshot_rules(k,root,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_stage,f_config_code,f_config_params,f_config_pattern,f_config_capture,f_config_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_id=excluded.f_id,f_name=excluded.f_name,f_description=excluded.f_description,f_target=excluded.f_target,f_source=excluded.f_source,f_order=excluded.f_order,f_enabled=excluded.f_enabled,f_kind=excluded.f_kind,f_stage=excluded.f_stage,f_config_code=excluded.f_config_code,f_config_params=excluded.f_config_params,f_config_pattern=excluded.f_config_pattern,f_config_capture=excluded.f_config_capture,f_config_value=excluded.f_config_value", key, root, owner, position, v.ID, v.Name, v.Description, v.Target, v.Source, v.Order, v.Enabled, v.Kind, v.Stage, v.Config.Code, j11, v.Config.Pattern, v.Config.Capture, j14)
	return err
}

func writeSnapshotRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	if err := putSnapshotRules(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM snapshot_rule_examples WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Examples {
		if err := writeSnapshotRuleExamples(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotRules struct {
	Key, Owner string
	Position   int
	Value      recognition.Rule
}

func readSnapshotRules(q queryer, root string) ([]rowSnapshotRules, error) {
	cSnapshotRuleExamples, err := readSnapshotRuleExamples(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_stage,f_config_code,f_config_params,f_config_pattern,f_config_capture,f_config_value FROM snapshot_rules WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowSnapshotRules{}
	for rows.Next() {
		var r rowSnapshotRules
		var v recognition.Rule
		v, err = scanSnapshotRules(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Examples = []recognition.Example{}
		}
		for _, c := range cSnapshotRuleExamples {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan snapshot_rule_examples")
			}
			out[i].Value.Examples = append(out[i].Value.Examples, c.Value)
		}
	}
	return out, nil
}

func scanSnapshotRuleExamples(s scanner) (recognition.Example, error) {
	var v recognition.Example
	var raw sql.NullString
	err := s.Scan(&v.Root, &raw)
	if err == nil && raw.Valid {
		err = decode(raw.String, &v.Expected)
	}
	return v, err
}
func writeSnapshotRuleExamples(q queryer, key, root, owner string, position int, v recognition.Example) error {
	raw, err := dynamicJSON(v.Expected)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO snapshot_rule_examples(k,root,owner,position,f_root,f_expected) VALUES(?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET f_root=excluded.f_root,f_expected=excluded.f_expected", key, root, owner, position, v.Root, raw)
	if err != nil {
		return err
	}
	for _, kind := range []string{"paths", "targets"} {
		if _, err = q.Exec("DELETE FROM snapshot_rule_examples_"+kind+" WHERE owner=?", key); err != nil {
			return err
		}
		values := v.Paths
		if kind == "targets" {
			values = v.Targets
		}
		for i, x := range values {
			if _, err = q.Exec("INSERT INTO snapshot_rule_examples_"+kind+"(owner,position,value) VALUES(?,?,?)", key, i, x); err != nil {
				return err
			}
		}
	}
	return nil
}

type rowSnapshotRuleExamples struct {
	Key, Owner string
	Position   int
	Value      recognition.Example
}

func readSnapshotRuleExamples(q queryer, root string) ([]rowSnapshotRuleExamples, error) {
	rows, err := q.Query("SELECT k,owner,position,f_root,f_expected FROM snapshot_rule_examples WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotRuleExamples{}
	for rows.Next() {
		var r rowSnapshotRuleExamples
		r.Value, err = scanSnapshotRuleExamples(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.Paths = []string{}
		r.Value.Targets = []string{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, r := range out {
		index[r.Key] = i
	}
	for _, kind := range []string{"paths", "targets"} {
		rows, err = q.Query("SELECT c.owner,c.value FROM snapshot_rule_examples_"+kind+" c JOIN snapshot_rule_examples p ON p.k=c.owner WHERE p.root=? ORDER BY c.owner,c.position", root)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var owner, value string
			if err = rows.Scan(&owner, &value); err != nil {
				rows.Close()
				return nil, err
			}
			i, ok := index[owner]
			if !ok {
				rows.Close()
				return nil, fmt.Errorf("orphan example")
			}
			if kind == "paths" {
				out[i].Value.Paths = append(out[i].Value.Paths, value)
			} else {
				out[i].Value.Targets = append(out[i].Value.Targets, value)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
