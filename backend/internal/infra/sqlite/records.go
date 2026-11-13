// Fixed-column persistence for current domain records.
// Collections use owned rows; only script-defined values use JSON.
package sqlite

import (
	"database/sql"
	"fmt"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}
type scanner interface{ Scan(...any) error }

const columnsConfig = "f_version,f_source_root,f_target_root,f_token,f_proxy_url,f_language,f_rules_movie_folder,f_rules_movie_file,f_rules_tv_folder,f_rules_tv_file"

func scanConfig(s scanner) (model.Settings, error) {
	var v model.Settings
	var err error
	err = s.Scan(&v.Version, &v.SourceRoot, &v.TargetRoot, &v.Token, &v.ProxyURL, &v.Language, &v.Rules.MovieFolder, &v.Rules.MovieFile, &v.Rules.TVFolder, &v.Rules.TVFile)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putConfig(q queryer, key, root, owner string, position int, v model.Settings) error {
	_, err := q.Exec("INSERT INTO config(id,f_version,f_source_root,f_target_root,f_token,f_proxy_url,f_language,f_rules_movie_folder,f_rules_movie_file,f_rules_tv_folder,f_rules_tv_file) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_version=excluded.f_version,f_source_root=excluded.f_source_root,f_target_root=excluded.f_target_root,f_token=excluded.f_token,f_proxy_url=excluded.f_proxy_url,f_language=excluded.f_language,f_rules_movie_folder=excluded.f_rules_movie_folder,f_rules_movie_file=excluded.f_rules_movie_file,f_rules_tv_folder=excluded.f_rules_tv_folder,f_rules_tv_file=excluded.f_rules_tv_file", key, v.Version, v.SourceRoot, v.TargetRoot, v.Token, v.ProxyURL, v.Language, v.Rules.MovieFolder, v.Rules.MovieFile, v.Rules.TVFolder, v.Rules.TVFile)
	return err
}

func writeConfig(q queryer, key, root, owner string, position int, v model.Settings) error {
	if err := putConfig(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM config_categories WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Rules.Categories {
		if err := writeConfigCategories(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

func scanConfigCategories(s scanner) (model.Category, error) {
	var v model.Category
	var err error
	err = s.Scan(&v.Name, &v.Type, &v.YearMin, &v.YearMax)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putConfigCategories(q queryer, key, root, owner string, position int, v model.Category) error {
	_, err := q.Exec("INSERT INTO config_categories(k,root,owner,position,f_name,f_type,f_year_min,f_year_max) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_name=excluded.f_name,f_type=excluded.f_type,f_year_min=excluded.f_year_min,f_year_max=excluded.f_year_max", key, root, owner, position, v.Name, v.Type, v.YearMin, v.YearMax)
	return err
}

func writeConfigCategories(q queryer, key, root, owner string, position int, v model.Category) error {
	if err := putConfigCategories(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM category_countries WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Countries {
		if err := writeCategoryCountries(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM category_genres WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Genres {
		if err := writeCategoryGenres(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowConfigCategories struct {
	Key, Owner string
	Position   int
	Value      model.Category
}

func readConfigCategories(q queryer, root string) ([]rowConfigCategories, error) {
	cCategoryCountries, err := readCategoryCountries(q, root)
	if err != nil {
		return nil, err
	}
	cCategoryGenres, err := readCategoryGenres(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_name,f_type,f_year_min,f_year_max FROM config_categories WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowConfigCategories{}
	for rows.Next() {
		var r rowConfigCategories
		var v model.Category
		v, err = scanConfigCategories(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Countries = []string{}
		}
		for _, c := range cCategoryCountries {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan category_countries")
			}
			out[i].Value.Countries = append(out[i].Value.Countries, c.Value)
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Genres = []string{}
		}
		for _, c := range cCategoryGenres {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan category_genres")
			}
			out[i].Value.Genres = append(out[i].Value.Genres, c.Value)
		}
	}
	return out, nil
}

func scanCategoryCountries(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeCategoryCountries(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO category_countries(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowCategoryCountries struct {
	Key, Owner string
	Position   int
	Value      string
}

func readCategoryCountries(q queryer, root string) ([]rowCategoryCountries, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM category_countries WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowCategoryCountries{}
	for rows.Next() {
		var r rowCategoryCountries
		var v string
		v, err = scanCategoryCountries(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanCategoryGenres(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeCategoryGenres(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO category_genres(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowCategoryGenres struct {
	Key, Owner string
	Position   int
	Value      string
}

func readCategoryGenres(q queryer, root string) ([]rowCategoryGenres, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM category_genres WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowCategoryGenres{}
	for rows.Next() {
		var r rowCategoryGenres
		var v string
		v, err = scanCategoryGenres(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanRules(s scanner) (recognition.Rule, error) {
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

func putRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	j11, err := dynamicJSON(v.Config.Params)
	if err != nil {
		return err
	}
	j14, err := dynamicJSON(v.Config.Value)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO rules(k,root,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_stage,f_config_code,f_config_params,f_config_pattern,f_config_capture,f_config_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_id=excluded.f_id,f_name=excluded.f_name,f_description=excluded.f_description,f_target=excluded.f_target,f_source=excluded.f_source,f_order=excluded.f_order,f_enabled=excluded.f_enabled,f_kind=excluded.f_kind,f_stage=excluded.f_stage,f_config_code=excluded.f_config_code,f_config_params=excluded.f_config_params,f_config_pattern=excluded.f_config_pattern,f_config_capture=excluded.f_config_capture,f_config_value=excluded.f_config_value", key, root, owner, position, v.ID, v.Name, v.Description, v.Target, v.Source, v.Order, v.Enabled, v.Kind, v.Stage, v.Config.Code, j11, v.Config.Pattern, v.Config.Capture, j14)
	return err
}

func writeRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	if err := putRules(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM rule_examples WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Examples {
		if err := writeRuleExamples(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowRules struct {
	Key, Owner string
	Position   int
	Value      recognition.Rule
}

func readRules(q queryer, root string) ([]rowRules, error) {
	cRuleExamples, err := readRuleExamples(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_stage,f_config_code,f_config_params,f_config_pattern,f_config_capture,f_config_value FROM rules WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowRules{}
	for rows.Next() {
		var r rowRules
		var v recognition.Rule
		v, err = scanRules(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		for _, c := range cRuleExamples {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan rule_examples")
			}
			out[i].Value.Examples = append(out[i].Value.Examples, c.Value)
		}
	}
	return out, nil
}

func scanRuleExamples(s scanner) (recognition.Example, error) {
	var v recognition.Example
	var raw sql.NullString
	err := s.Scan(&v.Root, &raw)
	if err == nil && raw.Valid {
		err = decode(raw.String, &v.Expected)
	}
	return v, err
}
func writeRuleExamples(q queryer, key, root, owner string, position int, v recognition.Example) error {
	raw, err := dynamicJSON(v.Expected)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO rule_examples(k,root,owner,position,f_root,f_expected) VALUES(?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET f_root=excluded.f_root,f_expected=excluded.f_expected", key, root, owner, position, v.Root, raw)
	if err != nil {
		return err
	}
	for _, kind := range []string{"paths", "targets"} {
		if _, err = q.Exec("DELETE FROM rule_examples_"+kind+" WHERE owner=?", key); err != nil {
			return err
		}
		values := v.Paths
		if kind == "targets" {
			values = v.Targets
		}
		for i, x := range values {
			if _, err = q.Exec("INSERT INTO rule_examples_"+kind+"(owner,position,value) VALUES(?,?,?)", key, i, x); err != nil {
				return err
			}
		}
	}
	return nil
}

type rowRuleExamples struct {
	Key, Owner string
	Position   int
	Value      recognition.Example
}

func readRuleExamples(q queryer, root string) ([]rowRuleExamples, error) {
	rows, err := q.Query("SELECT k,owner,position,f_root,f_expected FROM rule_examples WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowRuleExamples{}
	for rows.Next() {
		var r rowRuleExamples
		r.Value, err = scanRuleExamples(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		rows, err = q.Query("SELECT c.owner,c.value FROM rule_examples_"+kind+" c JOIN rule_examples p ON p.k=c.owner WHERE p.root=? ORDER BY c.owner,c.position", root)
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

const columnsRecognitions = "f_purpose,f_revision,f_state,f_completed,f_total,f_error,f_created"

func scanRecognitions(s scanner) (matching.Workspace, error) {
	var v matching.Workspace
	var err error
	err = s.Scan(&v.Purpose, &v.Revision, &v.State, &v.Completed, &v.Total, &v.Error, &v.Created)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putRecognitions(q queryer, key, root, owner string, position int, v matching.Workspace) error {
	_, err := q.Exec("INSERT INTO recognitions(id,f_revision,f_state,f_completed,f_total,f_error,f_created,f_purpose) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_revision=excluded.f_revision,f_state=excluded.f_state,f_completed=excluded.f_completed,f_total=excluded.f_total,f_error=excluded.f_error,f_created=excluded.f_created,f_purpose=excluded.f_purpose", key, v.Revision, v.State, v.Completed, v.Total, v.Error, v.Created, v.Purpose)
	return err
}

func writeRecognitions(q queryer, key, root, owner string, position int, v matching.Workspace) error {
	if err := putRecognitions(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM workspace_paths WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Paths {
		if err := writeWorkspacePaths(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM workspace_items WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Items {
		if err := writeWorkspaceItems(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

func scanWorkspacePaths(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeWorkspacePaths(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO workspace_paths(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowWorkspacePaths struct {
	Key, Owner string
	Position   int
	Value      string
}

func readWorkspacePaths(q queryer, root string) ([]rowWorkspacePaths, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM workspace_paths WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowWorkspacePaths{}
	for rows.Next() {
		var r rowWorkspacePaths
		var v string
		v, err = scanWorkspacePaths(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanWorkspaceItems(s scanner) (matching.Item, error) {
	var v matching.Item
	var err error
	err = s.Scan(&v.Identity, &v.Layout.WorkDir, &v.Layout.SeasonDir, &v.Layout.MoviePrefix, &v.Layout.Manual, &v.Path, &v.Size, &v.MTime, &v.Season, &v.Group, &v.State, &v.Reason, &v.Selected, &v.Removed, &v.Edited, &v.TraceID)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putWorkspaceItems(q queryer, key, root, owner string, position int, v matching.Item) error {
	_, err := q.Exec("INSERT INTO workspace_items(k,root,owner,position,f_path,f_size,f_m_time,f_season,f_group,f_state,f_reason,f_selected,f_removed,f_edited,f_trace_id,f_identity,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_path=excluded.f_path,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_season=excluded.f_season,f_group=excluded.f_group,f_state=excluded.f_state,f_reason=excluded.f_reason,f_selected=excluded.f_selected,f_removed=excluded.f_removed,f_edited=excluded.f_edited,f_trace_id=excluded.f_trace_id,f_identity=excluded.f_identity,f_work_dir=excluded.f_work_dir,f_season_dir=excluded.f_season_dir,f_movie_prefix=excluded.f_movie_prefix,f_layout_manual=excluded.f_layout_manual", key, root, owner, position, v.Path, v.Size, v.MTime, v.Season, v.Group, v.State, v.Reason, v.Selected, v.Removed, v.Edited, v.TraceID, v.Identity, v.Layout.WorkDir, v.Layout.SeasonDir, v.Layout.MoviePrefix, v.Layout.Manual)
	return err
}

func writeWorkspaceItems(q queryer, key, root, owner string, position int, v matching.Item) error {
	for _, id := range []string{v.TraceID, confirmationTrace(v)} {
		if id == "" {
			continue
		}
		var saved bool
		if err := q.QueryRow("SELECT EXISTS(SELECT 1 FROM extraction_snapshots s JOIN extraction_traces t ON t.id=s.id WHERE s.id=? AND t.f_status='completed')", id).Scan(&saved); err != nil {
			return err
		}
		if !saved {
			return fmt.Errorf("识别快照尚未保存成功：%s", id)
		}
	}

	if err := putWorkspaceItems(q, key, root, owner, position, v); err != nil {
		return err
	}
	if err := writeConfirmation(q, key, root, v.Confirmation); err != nil {
		return err
	}
	if err := writeMappingSnapshot(q, "workspace", key, root, v.Original); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM item_episodes WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Episodes {
		if err := writeItemEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM item_metadata WHERE owner=?", key); err != nil {
		return err
	}
	if err := writeItemMetadata(q, childKey(key, 0), root, key, 0, v.Metadata); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM item_entries WHERE owner=?", key); err != nil {
		return err
	}
	if v.Entry != nil {
		if err := writeItemEntries(q, childKey(key, 0), root, key, 0, *v.Entry); err != nil {
			return err
		}
	}
	return nil
}

type rowWorkspaceItems struct {
	Key, Owner string
	Position   int
	Value      matching.Item
}

func readWorkspaceItems(q queryer, root string) ([]rowWorkspaceItems, error) {
	cItemEpisodes, err := readItemEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	cItemMetadata, err := readItemMetadata(q, root)
	if err != nil {
		return nil, err
	}
	cItemEntries, err := readItemEntries(q, root)
	if err != nil {
		return nil, err
	}
	confirmations, err := readConfirmations(q, root)
	if err != nil {
		return nil, err
	}
	originals, err := readMappingSnapshots(q, "workspace", root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_identity,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual,f_path,f_size,f_m_time,f_season,f_group,f_state,f_reason,f_selected,f_removed,f_edited,f_trace_id FROM workspace_items WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowWorkspaceItems{}
	for rows.Next() {
		var r rowWorkspaceItems
		var v matching.Item
		v, err = scanWorkspaceItems(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		v.Original = originals[r.Key]
		v.Confirmation = confirmations[r.Key]
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
			out[i].Value.Episodes = []int{}
		}
		for _, c := range cItemEpisodes {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_episodes")
			}
			out[i].Value.Episodes = append(out[i].Value.Episodes, c.Value)
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		if len(cItemMetadata) != len(out) {
			return nil, fmt.Errorf("missing required ItemMetadata record")
		}
		for _, c := range cItemMetadata {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_metadata")
			}
			out[i].Value.Metadata = c.Value
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for _, c := range cItemEntries {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_entries")
			}
			x := c.Value
			out[i].Value.Entry = &x
		}
	}
	return out, nil
}

func scanItemEpisodes(s scanner) (int, error) {
	var v int
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemEpisodes(q queryer, key, root, owner string, position int, v int) error {
	_, err := q.Exec("INSERT INTO item_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowItemEpisodes struct {
	Key, Owner string
	Position   int
	Value      int
}

func readItemEpisodes(q queryer, root string) ([]rowItemEpisodes, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM item_episodes WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemEpisodes{}
	for rows.Next() {
		var r rowItemEpisodes
		var v int
		v, err = scanItemEpisodes(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanMetadataEpisodes(s scanner) (int, error) {
	var v int
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeMetadataEpisodes(q queryer, key, root, owner string, position int, v int) error {
	_, err := q.Exec("INSERT INTO metadata_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowMetadataEpisodes struct {
	Key, Owner string
	Position   int
	Value      int
}

func readMetadataEpisodes(q queryer, root string) ([]rowMetadataEpisodes, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM metadata_episodes WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowMetadataEpisodes{}
	for rows.Next() {
		var r rowMetadataEpisodes
		var v int
		v, err = scanMetadataEpisodes(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanTraceEpisodes(s scanner) (int, error) {
	var v int
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeTraceEpisodes(q queryer, key, root, owner string, position int, v int) error {
	_, err := q.Exec("INSERT INTO trace_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowTraceEpisodes struct {
	Key, Owner string
	Position   int
	Value      int
}

func readTraceEpisodes(q queryer, root string) ([]rowTraceEpisodes, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM trace_episodes WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowTraceEpisodes{}
	for rows.Next() {
		var r rowTraceEpisodes
		var v int
		v, err = scanTraceEpisodes(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeMappingEpisodes(q queryer, key, root, owner string, position int, v int) error {
	_, err := q.Exec("INSERT INTO mapping_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

func scanRunMappingEpisodes(s scanner) (int, error) {
	var v int
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeRunMappingEpisodes(q queryer, key, root, owner string, position int, v int) error {
	_, err := q.Exec("INSERT INTO run_mapping_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowRunMappingEpisodes struct {
	Key, Owner string
	Position   int
	Value      int
}

func readRunMappingEpisodes(q queryer, root string) ([]rowRunMappingEpisodes, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM run_mapping_episodes WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowRunMappingEpisodes{}
	for rows.Next() {
		var r rowRunMappingEpisodes
		var v int
		v, err = scanRunMappingEpisodes(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanItemMetadata(s scanner) (recognition.Metadata, error) {
	var v recognition.Metadata
	var err error
	err = s.Scan(&v.TMDBID, &v.Title, &v.MediaType, &v.Year, &v.Season)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putItemMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	_, err := q.Exec("INSERT INTO item_metadata(k,root,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_title=excluded.f_title,f_media_type=excluded.f_media_type,f_year=excluded.f_year,f_season=excluded.f_season", key, root, owner, position, v.TMDBID, v.Title, v.MediaType, v.Year, v.Season)
	return err
}

func writeItemMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	if err := putItemMetadata(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM metadata_episodes WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Episodes {
		if err := writeMetadataEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowItemMetadata struct {
	Key, Owner string
	Position   int
	Value      recognition.Metadata
}

func readItemMetadata(q queryer, root string) ([]rowItemMetadata, error) {
	cMetadataEpisodes, err := readMetadataEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season FROM item_metadata WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemMetadata{}
	for rows.Next() {
		var r rowItemMetadata
		var v recognition.Metadata
		v, err = scanItemMetadata(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Episodes = []int{}
		}
		for _, c := range cMetadataEpisodes {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan metadata_episodes")
			}
			out[i].Value.Episodes = append(out[i].Value.Episodes, c.Value)
		}
	}
	return out, nil
}

const columnsTraceMetadata = "f_tmdbid,f_title,f_media_type,f_year,f_season"

func scanTraceMetadata(s scanner) (recognition.Metadata, error) {
	var v recognition.Metadata
	var err error
	err = s.Scan(&v.TMDBID, &v.Title, &v.MediaType, &v.Year, &v.Season)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putTraceMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	_, err := q.Exec("INSERT INTO trace_metadata(k,root,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_title=excluded.f_title,f_media_type=excluded.f_media_type,f_year=excluded.f_year,f_season=excluded.f_season", key, root, owner, position, v.TMDBID, v.Title, v.MediaType, v.Year, v.Season)
	return err
}

func writeTraceMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	if err := putTraceMetadata(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM trace_episodes WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Episodes {
		if err := writeTraceEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowTraceMetadata struct {
	Key, Owner string
	Position   int
	Value      recognition.Metadata
}

func readTraceMetadata(q queryer, root string) ([]rowTraceMetadata, error) {
	cTraceEpisodes, err := readTraceEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season FROM trace_metadata WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowTraceMetadata{}
	for rows.Next() {
		var r rowTraceMetadata
		var v recognition.Metadata
		v, err = scanTraceMetadata(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Episodes = []int{}
		}
		for _, c := range cTraceEpisodes {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan trace_episodes")
			}
			out[i].Value.Episodes = append(out[i].Value.Episodes, c.Value)
		}
	}
	return out, nil
}

func scanItemEntries(s scanner) (model.Entry, error) {
	var v model.Entry
	var err error
	err = s.Scan(&v.TMDBID, &v.Type, &v.Title, &v.OriginalTitle, &v.Year, &v.Overview, &v.Date, &v.PosterPath, &v.BackdropPath)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putItemEntries(q queryer, key, root, owner string, position int, v model.Entry) error {
	_, err := q.Exec("INSERT INTO item_entries(k,root,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_type=excluded.f_type,f_title=excluded.f_title,f_original_title=excluded.f_original_title,f_year=excluded.f_year,f_overview=excluded.f_overview,f_date=excluded.f_date,f_poster_path=excluded.f_poster_path,f_backdrop_path=excluded.f_backdrop_path", key, root, owner, position, v.TMDBID, v.Type, v.Title, v.OriginalTitle, v.Year, v.Overview, v.Date, v.PosterPath, v.BackdropPath)
	return err
}

func writeItemEntries(q queryer, key, root, owner string, position int, v model.Entry) error {
	if err := putItemEntries(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM item_entries_genres WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Genres {
		if err := writeItemEntriesGenres(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM item_entries_countries WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Countries {
		if err := writeItemEntriesCountries(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM item_entries_cast WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Cast {
		if err := writeItemEntriesCast(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowItemEntries struct {
	Key, Owner string
	Position   int
	Value      model.Entry
}

func readItemEntries(q queryer, root string) ([]rowItemEntries, error) {
	cItemEntriesGenres, err := readItemEntriesGenres(q, root)
	if err != nil {
		return nil, err
	}
	cItemEntriesCountries, err := readItemEntriesCountries(q, root)
	if err != nil {
		return nil, err
	}
	cItemEntriesCast, err := readItemEntriesCast(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path FROM item_entries WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemEntries{}
	for rows.Next() {
		var r rowItemEntries
		var v model.Entry
		v, err = scanItemEntries(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Genres = []string{}
		}
		for _, c := range cItemEntriesGenres {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_entries_genres")
			}
			out[i].Value.Genres = append(out[i].Value.Genres, c.Value)
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Countries = []string{}
		}
		for _, c := range cItemEntriesCountries {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_entries_countries")
			}
			out[i].Value.Countries = append(out[i].Value.Countries, c.Value)
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Cast = []model.Actor{}
		}
		for _, c := range cItemEntriesCast {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_entries_cast")
			}
			out[i].Value.Cast = append(out[i].Value.Cast, c.Value)
		}
	}
	return out, nil
}

func scanItemEntriesGenres(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemEntriesGenres(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO item_entries_genres(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowItemEntriesGenres struct {
	Key, Owner string
	Position   int
	Value      string
}

func readItemEntriesGenres(q queryer, root string) ([]rowItemEntriesGenres, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM item_entries_genres WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemEntriesGenres{}
	for rows.Next() {
		var r rowItemEntriesGenres
		var v string
		v, err = scanItemEntriesGenres(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanItemEntriesCountries(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemEntriesCountries(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO item_entries_countries(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowItemEntriesCountries struct {
	Key, Owner string
	Position   int
	Value      string
}

func readItemEntriesCountries(q queryer, root string) ([]rowItemEntriesCountries, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM item_entries_countries WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemEntriesCountries{}
	for rows.Next() {
		var r rowItemEntriesCountries
		var v string
		v, err = scanItemEntriesCountries(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanItemEntriesCast(s scanner) (model.Actor, error) {
	var v model.Actor
	var err error
	err = s.Scan(&v.Name, &v.Role)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemEntriesCast(q queryer, key, root, owner string, position int, v model.Actor) error {
	_, err := q.Exec("INSERT INTO item_entries_cast(k,root,owner,position,f_name,f_role) VALUES(?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_name=excluded.f_name,f_role=excluded.f_role", key, root, owner, position, v.Name, v.Role)
	return err
}

type rowItemEntriesCast struct {
	Key, Owner string
	Position   int
	Value      model.Actor
}

func readItemEntriesCast(q queryer, root string) ([]rowItemEntriesCast, error) {
	rows, err := q.Query("SELECT k,owner,position,f_name,f_role FROM item_entries_cast WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemEntriesCast{}
	for rows.Next() {
		var r rowItemEntriesCast
		var v model.Actor
		v, err = scanItemEntriesCast(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

const columnsRuns = "f_title,f_mode,f_state,f_created,f_error,f_completed,f_total,f_undo,f_batch_id,f_version,f_recognition_id,f_purpose"

func scanRuns(s scanner) (organizer.Run, error) {
	var v organizer.Run
	var err error
	err = s.Scan(&v.Title, &v.Mode, &v.State, &v.Created, &v.Error, &v.Completed, &v.Total, &v.Undo, &v.BatchID, &v.Version, &v.RecognitionID, &v.Purpose)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putRuns(q queryer, key, root, owner string, position int, v organizer.Run) error {
	_, err := q.Exec("INSERT INTO runs(id,f_title,f_mode,f_state,f_created,f_error,f_completed,f_total,f_undo,f_batch_id,f_version,f_recognition_id,f_purpose) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_title=excluded.f_title,f_mode=excluded.f_mode,f_state=excluded.f_state,f_created=excluded.f_created,f_error=excluded.f_error,f_completed=excluded.f_completed,f_total=excluded.f_total,f_undo=excluded.f_undo,f_batch_id=excluded.f_batch_id,f_version=excluded.f_version,f_recognition_id=excluded.f_recognition_id,f_purpose=excluded.f_purpose", key, v.Title, v.Mode, v.State, v.Created, v.Error, v.Completed, v.Total, v.Undo, v.BatchID, v.Version, v.RecognitionID, v.Purpose)
	return err
}

func scanRunMappings(s scanner) (model.MediaMapping, error) {
	var v model.MediaMapping
	var err error
	err = s.Scan(&v.Layout.WorkDir, &v.Layout.SeasonDir, &v.Layout.MoviePrefix, &v.Layout.Manual, &v.Path, &v.RunID, &v.Source, &v.Type, &v.TMDBID, &v.Season, &v.State, &v.Identity, &v.Size, &v.MTime)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putRunMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	_, err := q.Exec("INSERT INTO run_mappings(k,root,owner,position,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_path=excluded.f_path,f_run_id=excluded.f_run_id,f_source=excluded.f_source,f_type=excluded.f_type,f_tmdbid=excluded.f_tmdbid,f_season=excluded.f_season,f_state=excluded.f_state,f_identity=excluded.f_identity,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_work_dir=excluded.f_work_dir,f_season_dir=excluded.f_season_dir,f_movie_prefix=excluded.f_movie_prefix,f_layout_manual=excluded.f_layout_manual", key, root, owner, position, v.Path, v.RunID, v.Source, v.Type, v.TMDBID, v.Season, v.State, v.Identity, v.Size, v.MTime, v.Layout.WorkDir, v.Layout.SeasonDir, v.Layout.MoviePrefix, v.Layout.Manual)
	return err
}

func writeRunMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	if err := putRunMappings(q, key, root, owner, position, v); err != nil {
		return err
	}
	if err := writeWork(q, "run_mapping", key, v.Entry); err != nil {
		return err
	}
	if err := writeMappingSnapshot(q, "run", key, root, v.Original); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM run_mapping_episodes WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Episodes {
		if err := writeRunMappingEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowRunMappings struct {
	Key, Owner string
	Position   int
	Value      model.MediaMapping
}

func readRunMappings(q queryer, root string) ([]rowRunMappings, error) {
	works, err := readWorks(q, "run_mapping", "run_mappings", "k", "WHERE p.root=?", root)
	if err != nil {
		return nil, err
	}
	cRunMappingEpisodes, err := readRunMappingEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	originals, err := readMappingSnapshots(q, "run", root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time FROM run_mappings WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowRunMappings{}
	for rows.Next() {
		var r rowRunMappings
		var v model.MediaMapping
		v, err = scanRunMappings(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		v.Original = originals[r.Key]
		v.Entry = works[r.Key]
		if v.Entry == nil {
			return nil, fmt.Errorf("任务映射缺少已确认作品快照")
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
			out[i].Value.Episodes = []int{}
		}
		for _, c := range cRunMappingEpisodes {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan run_mapping_episodes")
			}
			out[i].Value.Episodes = append(out[i].Value.Episodes, c.Value)
		}
	}
	return out, nil
}

const columnsMediaMappings = "f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time"

func scanMediaMappings(s scanner) (model.MediaMapping, error) {
	var v model.MediaMapping
	var err error
	err = s.Scan(&v.Layout.WorkDir, &v.Layout.SeasonDir, &v.Layout.MoviePrefix, &v.Layout.Manual, &v.Path, &v.RunID, &v.Source, &v.Type, &v.TMDBID, &v.Season, &v.State, &v.Identity, &v.Size, &v.MTime)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putMediaMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	_, err := q.Exec("INSERT INTO media_mappings(id,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time,f_work_dir,f_season_dir,f_movie_prefix,f_layout_manual) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_path=excluded.f_path,f_run_id=excluded.f_run_id,f_source=excluded.f_source,f_type=excluded.f_type,f_tmdbid=excluded.f_tmdbid,f_season=excluded.f_season,f_state=excluded.f_state,f_identity=excluded.f_identity,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_work_dir=excluded.f_work_dir,f_season_dir=excluded.f_season_dir,f_movie_prefix=excluded.f_movie_prefix,f_layout_manual=excluded.f_layout_manual", key, v.Path, v.RunID, v.Source, v.Type, v.TMDBID, v.Season, v.State, v.Identity, v.Size, v.MTime, v.Layout.WorkDir, v.Layout.SeasonDir, v.Layout.MoviePrefix, v.Layout.Manual)
	return err
}

func writeMediaMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	if err := putMediaMappings(q, key, root, owner, position, v); err != nil {
		return err
	}
	if err := writeWork(q, "mapping", key, v.Entry); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM mapping_episodes WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Episodes {
		if err := writeMappingEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

const columnsOperations = "f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused,f_duration_ms,f_speed_bps"

func scanOperations(s scanner) (organizer.Work, error) {
	var v organizer.Work
	var err error
	err = s.Scan(&v.ID, &v.Kind, &v.Source, &v.Target, &v.Mode, &v.Size, &v.MTime, &v.Content, &v.URL, &v.ConvertJPEG, &v.Status, &v.Error, &v.Hash, &v.Temp, &v.ReplaceHold, &v.RemoveHold, &v.SourceRemoveHold, &v.Identity, &v.SourceIdentity, &v.Replace, &v.ReplaceIdentity, &v.ReplaceSize, &v.ReplaceMTime, &v.Owner, &v.Reused, &v.DurationMs, &v.SpeedBps)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putOperations(q queryer, key, root, owner string, position int, v organizer.Work, undoParent any) error {
	_, err := q.Exec("INSERT INTO operations(k,root,owner,position,undo_parent,f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused,f_duration_ms,f_speed_bps) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,undo_parent=excluded.undo_parent,f_id=excluded.f_id,f_kind=excluded.f_kind,f_source=excluded.f_source,f_target=excluded.f_target,f_mode=excluded.f_mode,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_content=excluded.f_content,f_url=excluded.f_url,f_convert_jpeg=excluded.f_convert_jpeg,f_status=excluded.f_status,f_error=excluded.f_error,f_hash=excluded.f_hash,f_temp=excluded.f_temp,f_replace_hold=excluded.f_replace_hold,f_remove_hold=excluded.f_remove_hold,f_source_remove_hold=excluded.f_source_remove_hold,f_identity=excluded.f_identity,f_source_identity=excluded.f_source_identity,f_replace=excluded.f_replace,f_replace_identity=excluded.f_replace_identity,f_replace_size=excluded.f_replace_size,f_replace_m_time=excluded.f_replace_m_time,f_owner=excluded.f_owner,f_reused=excluded.f_reused,f_duration_ms=excluded.f_duration_ms,f_speed_bps=excluded.f_speed_bps", key, root, owner, position, undoParent, v.ID, v.Kind, v.Source, v.Target, v.Mode, v.Size, v.MTime, v.Content, v.URL, v.ConvertJPEG, v.Status, v.Error, v.Hash, v.Temp, v.ReplaceHold, v.RemoveHold, v.SourceRemoveHold, v.Identity, v.SourceIdentity, v.Replace, v.ReplaceIdentity, v.ReplaceSize, v.ReplaceMTime, v.Owner, v.Reused, v.DurationMs, v.SpeedBps)
	return err
}

type rowOperations struct {
	Key, Owner string
	Position   int
	Value      organizer.Work
}

func readOperations(q queryer, root string) ([]rowOperations, error) {
	cOperationOwners, err := readOperationOwners(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused,f_duration_ms,f_speed_bps FROM operations WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowOperations{}
	for rows.Next() {
		var r rowOperations
		var v organizer.Work
		v, err = scanOperations(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Owners = []string{}
		}
		for _, c := range cOperationOwners {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan operation_owners")
			}
			out[i].Value.Owners = append(out[i].Value.Owners, c.Value)
		}
	}
	return out, nil
}

func scanOperationOwners(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeOperationOwners(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO operation_owners(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowOperationOwners struct {
	Key, Owner string
	Position   int
	Value      string
}

func readOperationOwners(q queryer, root string) ([]rowOperationOwners, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM operation_owners WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowOperationOwners{}
	for rows.Next() {
		var r rowOperationOwners
		var v string
		v, err = scanOperationOwners(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
		if err != nil {
			return nil, err
		}
		r.Value = v
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

const columnsExtractionTraces = "f_path,f_root,f_workspace_id,f_status,f_started_at,f_finished_at,f_error"

func scanExtractionTraces(s scanner) (recognition.Trace, error) {
	var v recognition.Trace
	var err error
	err = s.Scan(&v.Path, &v.Root, &v.WorkspaceID, &v.Status, &v.StartedAt, &v.FinishedAt, &v.Error)
	if err != nil {
		return v, err
	}
	return v, nil
}

func confirmationTrace(v matching.Item) string {
	if v.Confirmation != nil {
		return v.Confirmation.Basis.TraceID
	}
	return ""
}
