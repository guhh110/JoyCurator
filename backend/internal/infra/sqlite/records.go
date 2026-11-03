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
	"sort"
	"time"
)

type capture struct{ Name, Value string }
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
	err = s.Scan(&v.ID, &v.Name, &v.Description, &v.Target, &v.Source, &v.Order, &v.Enabled, &v.Kind, &v.Overwrite, &v.Config.Code, &v.Config.SourceCode, &raw11, &v.Config.Pattern, &v.Config.Capture, &raw14)
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
	_, err = q.Exec("INSERT INTO rules(k,root,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_overwrite,f_config_code,f_config_source_code,f_config_params,f_config_pattern,f_config_capture,f_config_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_id=excluded.f_id,f_name=excluded.f_name,f_description=excluded.f_description,f_target=excluded.f_target,f_source=excluded.f_source,f_order=excluded.f_order,f_enabled=excluded.f_enabled,f_kind=excluded.f_kind,f_overwrite=excluded.f_overwrite,f_config_code=excluded.f_config_code,f_config_source_code=excluded.f_config_source_code,f_config_params=excluded.f_config_params,f_config_pattern=excluded.f_config_pattern,f_config_capture=excluded.f_config_capture,f_config_value=excluded.f_config_value", key, root, owner, position, v.ID, v.Name, v.Description, v.Target, v.Source, v.Order, v.Enabled, v.Kind, v.Overwrite, v.Config.Code, v.Config.SourceCode, j11, v.Config.Pattern, v.Config.Capture, j14)
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
	rows, err := q.Query("SELECT k,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_overwrite,f_config_code,f_config_source_code,f_config_params,f_config_pattern,f_config_capture,f_config_value FROM rules WHERE root=? ORDER BY owner,position", root)
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
	var err error
	var raw2 sql.NullString
	err = s.Scan(&v.Path, &v.Root, &raw2)
	if err != nil {
		return v, err
	}
	if raw2.Valid {
		if err = decode(raw2.String, &v.Expected); err != nil {
			return v, err
		}
	}
	return v, nil
}

func writeRuleExamples(q queryer, key, root, owner string, position int, v recognition.Example) error {
	j2, err := dynamicJSON(v.Expected)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO rule_examples(k,root,owner,position,f_path,f_root,f_expected) VALUES(?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_path=excluded.f_path,f_root=excluded.f_root,f_expected=excluded.f_expected", key, root, owner, position, v.Path, v.Root, j2)
	return err
}

type rowRuleExamples struct {
	Key, Owner string
	Position   int
	Value      recognition.Example
}

func readRuleExamples(q queryer, root string) ([]rowRuleExamples, error) {
	rows, err := q.Query("SELECT k,owner,position,f_path,f_root,f_expected FROM rule_examples WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowRuleExamples{}
	for rows.Next() {
		var r rowRuleExamples
		var v recognition.Example
		v, err = scanRuleExamples(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

const columnsRecognitions = "f_revision,f_state,f_completed,f_total,f_error,f_created"

func scanRecognitions(s scanner) (matching.Workspace, error) {
	var v matching.Workspace
	var err error
	err = s.Scan(&v.Revision, &v.State, &v.Completed, &v.Total, &v.Error, &v.Created)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putRecognitions(q queryer, key, root, owner string, position int, v matching.Workspace) error {
	_, err := q.Exec("INSERT INTO recognitions(id,f_revision,f_state,f_completed,f_total,f_error,f_created) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_revision=excluded.f_revision,f_state=excluded.f_state,f_completed=excluded.f_completed,f_total=excluded.f_total,f_error=excluded.f_error,f_created=excluded.f_created", key, v.Revision, v.State, v.Completed, v.Total, v.Error, v.Created)
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
	err = s.Scan(&v.Path, &v.Size, &v.MTime, &v.Season, &v.Group, &v.State, &v.Reason, &v.Selected, &v.Removed, &v.Edited, &v.TraceID)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putWorkspaceItems(q queryer, key, root, owner string, position int, v matching.Item) error {
	_, err := q.Exec("INSERT INTO workspace_items(k,root,owner,position,f_path,f_size,f_m_time,f_season,f_group,f_state,f_reason,f_selected,f_removed,f_edited,f_trace_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_path=excluded.f_path,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_season=excluded.f_season,f_group=excluded.f_group,f_state=excluded.f_state,f_reason=excluded.f_reason,f_selected=excluded.f_selected,f_removed=excluded.f_removed,f_edited=excluded.f_edited,f_trace_id=excluded.f_trace_id", key, root, owner, position, v.Path, v.Size, v.MTime, v.Season, v.Group, v.State, v.Reason, v.Selected, v.Removed, v.Edited, v.TraceID)
	return err
}

func writeWorkspaceItems(q queryer, key, root, owner string, position int, v matching.Item) error {
	if err := putWorkspaceItems(q, key, root, owner, position, v); err != nil {
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
	if _, err := q.Exec("DELETE FROM item_candidates WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Candidates {
		if err := writeItemCandidates(q, childKey(key, i), root, key, i, x); err != nil {
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
	cItemCandidates, err := readItemCandidates(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_path,f_size,f_m_time,f_season,f_group,f_state,f_reason,f_selected,f_removed,f_edited,f_trace_id FROM workspace_items WHERE root=? ORDER BY owner,position", root)
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
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Candidates = []model.Entry{}
		}
		for _, c := range cItemCandidates {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_candidates")
			}
			out[i].Value.Candidates = append(out[i].Value.Candidates, c.Value)
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
	err = s.Scan(&v.TMDBID, &v.Title, &v.MediaType, &v.Year, &v.Season, &v.ReleaseTag)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putItemMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	_, err := q.Exec("INSERT INTO item_metadata(k,root,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season,f_release_tag) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_title=excluded.f_title,f_media_type=excluded.f_media_type,f_year=excluded.f_year,f_season=excluded.f_season,f_release_tag=excluded.f_release_tag", key, root, owner, position, v.TMDBID, v.Title, v.MediaType, v.Year, v.Season, v.ReleaseTag)
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
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season,f_release_tag FROM item_metadata WHERE root=? ORDER BY owner,position", root)
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

const columnsTraceMetadata = "f_tmdbid,f_title,f_media_type,f_year,f_season,f_release_tag"

func scanTraceMetadata(s scanner) (recognition.Metadata, error) {
	var v recognition.Metadata
	var err error
	err = s.Scan(&v.TMDBID, &v.Title, &v.MediaType, &v.Year, &v.Season, &v.ReleaseTag)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putTraceMetadata(q queryer, key, root, owner string, position int, v recognition.Metadata) error {
	_, err := q.Exec("INSERT INTO trace_metadata(k,root,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season,f_release_tag) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_title=excluded.f_title,f_media_type=excluded.f_media_type,f_year=excluded.f_year,f_season=excluded.f_season,f_release_tag=excluded.f_release_tag", key, root, owner, position, v.TMDBID, v.Title, v.MediaType, v.Year, v.Season, v.ReleaseTag)
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
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_title,f_media_type,f_year,f_season,f_release_tag FROM trace_metadata WHERE root=? ORDER BY owner,position", root)
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

func scanItemCandidates(s scanner) (model.Entry, error) {
	var v model.Entry
	var err error
	err = s.Scan(&v.TMDBID, &v.Type, &v.Title, &v.OriginalTitle, &v.Year, &v.Overview, &v.Date, &v.PosterPath, &v.BackdropPath)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putItemCandidates(q queryer, key, root, owner string, position int, v model.Entry) error {
	_, err := q.Exec("INSERT INTO item_candidates(k,root,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_type=excluded.f_type,f_title=excluded.f_title,f_original_title=excluded.f_original_title,f_year=excluded.f_year,f_overview=excluded.f_overview,f_date=excluded.f_date,f_poster_path=excluded.f_poster_path,f_backdrop_path=excluded.f_backdrop_path", key, root, owner, position, v.TMDBID, v.Type, v.Title, v.OriginalTitle, v.Year, v.Overview, v.Date, v.PosterPath, v.BackdropPath)
	return err
}

func writeItemCandidates(q queryer, key, root, owner string, position int, v model.Entry) error {
	if err := putItemCandidates(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM item_candidates_genres WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Genres {
		if err := writeItemCandidatesGenres(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM item_candidates_countries WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Countries {
		if err := writeItemCandidatesCountries(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM item_candidates_cast WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Cast {
		if err := writeItemCandidatesCast(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowItemCandidates struct {
	Key, Owner string
	Position   int
	Value      model.Entry
}

func readItemCandidates(q queryer, root string) ([]rowItemCandidates, error) {
	cItemCandidatesGenres, err := readItemCandidatesGenres(q, root)
	if err != nil {
		return nil, err
	}
	cItemCandidatesCountries, err := readItemCandidatesCountries(q, root)
	if err != nil {
		return nil, err
	}
	cItemCandidatesCast, err := readItemCandidatesCast(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path FROM item_candidates WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemCandidates{}
	for rows.Next() {
		var r rowItemCandidates
		var v model.Entry
		v, err = scanItemCandidates(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		for _, c := range cItemCandidatesGenres {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_candidates_genres")
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
		for _, c := range cItemCandidatesCountries {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_candidates_countries")
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
		for _, c := range cItemCandidatesCast {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan item_candidates_cast")
			}
			out[i].Value.Cast = append(out[i].Value.Cast, c.Value)
		}
	}
	return out, nil
}

func scanItemCandidatesGenres(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemCandidatesGenres(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO item_candidates_genres(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowItemCandidatesGenres struct {
	Key, Owner string
	Position   int
	Value      string
}

func readItemCandidatesGenres(q queryer, root string) ([]rowItemCandidatesGenres, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM item_candidates_genres WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemCandidatesGenres{}
	for rows.Next() {
		var r rowItemCandidatesGenres
		var v string
		v, err = scanItemCandidatesGenres(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanItemCandidatesCountries(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemCandidatesCountries(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO item_candidates_countries(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowItemCandidatesCountries struct {
	Key, Owner string
	Position   int
	Value      string
}

func readItemCandidatesCountries(q queryer, root string) ([]rowItemCandidatesCountries, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM item_candidates_countries WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemCandidatesCountries{}
	for rows.Next() {
		var r rowItemCandidatesCountries
		var v string
		v, err = scanItemCandidatesCountries(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanItemCandidatesCast(s scanner) (model.Actor, error) {
	var v model.Actor
	var err error
	err = s.Scan(&v.Name, &v.Role)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeItemCandidatesCast(q queryer, key, root, owner string, position int, v model.Actor) error {
	_, err := q.Exec("INSERT INTO item_candidates_cast(k,root,owner,position,f_name,f_role) VALUES(?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_name=excluded.f_name,f_role=excluded.f_role", key, root, owner, position, v.Name, v.Role)
	return err
}

type rowItemCandidatesCast struct {
	Key, Owner string
	Position   int
	Value      model.Actor
}

func readItemCandidatesCast(q queryer, root string) ([]rowItemCandidatesCast, error) {
	rows, err := q.Query("SELECT k,owner,position,f_name,f_role FROM item_candidates_cast WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowItemCandidatesCast{}
	for rows.Next() {
		var r rowItemCandidatesCast
		var v model.Actor
		v, err = scanItemCandidatesCast(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
	err = s.Scan(&v.Path, &v.RunID, &v.Source, &v.Type, &v.TMDBID, &v.Season, &v.State, &v.Identity, &v.Size, &v.MTime)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putRunMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	_, err := q.Exec("INSERT INTO run_mappings(k,root,owner,position,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_path=excluded.f_path,f_run_id=excluded.f_run_id,f_source=excluded.f_source,f_type=excluded.f_type,f_tmdbid=excluded.f_tmdbid,f_season=excluded.f_season,f_state=excluded.f_state,f_identity=excluded.f_identity,f_size=excluded.f_size,f_m_time=excluded.f_m_time", key, root, owner, position, v.Path, v.RunID, v.Source, v.Type, v.TMDBID, v.Season, v.State, v.Identity, v.Size, v.MTime)
	return err
}

func writeRunMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	if err := putRunMappings(q, key, root, owner, position, v); err != nil {
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
	cRunMappingEpisodes, err := readRunMappingEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time FROM run_mappings WHERE root=? ORDER BY owner,position", root)
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

const columnsMediaMappings = "f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time"

func scanMediaMappings(s scanner) (model.MediaMapping, error) {
	var v model.MediaMapping
	var err error
	err = s.Scan(&v.Path, &v.RunID, &v.Source, &v.Type, &v.TMDBID, &v.Season, &v.State, &v.Identity, &v.Size, &v.MTime)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putMediaMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	_, err := q.Exec("INSERT INTO media_mappings(id,f_path,f_run_id,f_source,f_type,f_tmdbid,f_season,f_state,f_identity,f_size,f_m_time) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET f_path=excluded.f_path,f_run_id=excluded.f_run_id,f_source=excluded.f_source,f_type=excluded.f_type,f_tmdbid=excluded.f_tmdbid,f_season=excluded.f_season,f_state=excluded.f_state,f_identity=excluded.f_identity,f_size=excluded.f_size,f_m_time=excluded.f_m_time", key, v.Path, v.RunID, v.Source, v.Type, v.TMDBID, v.Season, v.State, v.Identity, v.Size, v.MTime)
	return err
}

func writeMediaMappings(q queryer, key, root, owner string, position int, v model.MediaMapping) error {
	if err := putMediaMappings(q, key, root, owner, position, v); err != nil {
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

const columnsOperations = "f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused"

func scanOperations(s scanner) (organizer.Work, error) {
	var v organizer.Work
	var err error
	err = s.Scan(&v.ID, &v.Kind, &v.Source, &v.Target, &v.Mode, &v.Size, &v.MTime, &v.Content, &v.URL, &v.ConvertJPEG, &v.Status, &v.Error, &v.Hash, &v.Temp, &v.ReplaceHold, &v.RemoveHold, &v.SourceRemoveHold, &v.Identity, &v.SourceIdentity, &v.Replace, &v.ReplaceIdentity, &v.ReplaceSize, &v.ReplaceMTime, &v.Owner, &v.Reused)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putOperations(q queryer, key, root, owner string, position int, v organizer.Work, undoParent any) error {
	_, err := q.Exec("INSERT INTO operations(k,root,owner,position,undo_parent,f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,undo_parent=excluded.undo_parent,f_id=excluded.f_id,f_kind=excluded.f_kind,f_source=excluded.f_source,f_target=excluded.f_target,f_mode=excluded.f_mode,f_size=excluded.f_size,f_m_time=excluded.f_m_time,f_content=excluded.f_content,f_url=excluded.f_url,f_convert_jpeg=excluded.f_convert_jpeg,f_status=excluded.f_status,f_error=excluded.f_error,f_hash=excluded.f_hash,f_temp=excluded.f_temp,f_replace_hold=excluded.f_replace_hold,f_remove_hold=excluded.f_remove_hold,f_source_remove_hold=excluded.f_source_remove_hold,f_identity=excluded.f_identity,f_source_identity=excluded.f_source_identity,f_replace=excluded.f_replace,f_replace_identity=excluded.f_replace_identity,f_replace_size=excluded.f_replace_size,f_replace_m_time=excluded.f_replace_m_time,f_owner=excluded.f_owner,f_reused=excluded.f_reused", key, root, owner, position, undoParent, v.ID, v.Kind, v.Source, v.Target, v.Mode, v.Size, v.MTime, v.Content, v.URL, v.ConvertJPEG, v.Status, v.Error, v.Hash, v.Temp, v.ReplaceHold, v.RemoveHold, v.SourceRemoveHold, v.Identity, v.SourceIdentity, v.Replace, v.ReplaceIdentity, v.ReplaceSize, v.ReplaceMTime, v.Owner, v.Reused)
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
	rows, err := q.Query("SELECT k,owner,position,f_id,f_kind,f_source,f_target,f_mode,f_size,f_m_time,f_content,f_url,f_convert_jpeg,f_status,f_error,f_hash,f_temp,f_replace_hold,f_remove_hold,f_source_remove_hold,f_identity,f_source_identity,f_replace,f_replace_identity,f_replace_size,f_replace_m_time,f_owner,f_reused FROM operations WHERE root=? ORDER BY owner,position", root)
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

func scanExtractionSteps(s scanner) (recognition.Step, error) {
	var v recognition.Step
	var err error
	var raw6 sql.NullString
	var raw7 sql.NullString
	var raw8 sql.NullString
	var raw11 sql.NullString
	err = s.Scan(&v.RuleID, &v.RuleName, &v.Order, &v.Target, &v.Status, &v.MatchedText, &raw6, &raw7, &raw8, &v.Applied, &v.Reason, &raw11, &v.DurationMS)
	if err != nil {
		return v, err
	}
	if raw6.Valid {
		if err = decode(raw6.String, &v.ExtractedValue); err != nil {
			return v, err
		}
	}
	if raw7.Valid {
		if err = decode(raw7.String, &v.Before); err != nil {
			return v, err
		}
	}
	if raw8.Valid {
		if err = decode(raw8.String, &v.After); err != nil {
			return v, err
		}
	}
	if v.StartedAt, err = time.Parse(time.RFC3339Nano, raw11.String); err != nil {
		return v, err
	}
	return v, nil
}

func putExtractionSteps(q queryer, key, root, owner string, position int, v recognition.Step) error {
	j6, err := dynamicJSON(v.ExtractedValue)
	if err != nil {
		return err
	}
	j7, err := dynamicJSON(v.Before)
	if err != nil {
		return err
	}
	j8, err := dynamicJSON(v.After)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO extraction_steps(k,root,owner,position,f_rule_id,f_rule_name,f_order,f_target,f_status,f_matched_text,f_extracted_value,f_before,f_after,f_applied,f_reason,f_started_at,f_duration_ms) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.RuleID, v.RuleName, v.Order, v.Target, v.Status, v.MatchedText, j6, j7, j8, v.Applied, v.Reason, v.StartedAt.Format(time.RFC3339Nano), v.DurationMS)
	return err
}

func writeExtractionSteps(q queryer, key, root, owner string, position int, v recognition.Step) error {
	if err := putExtractionSteps(q, key, root, owner, position, v); err != nil {
		return err
	}
	if err := writeStepRules(q, childKey(key, 0), root, key, 0, v.RuleConfig); err != nil {
		return err
	}
	for i, x := range v.Inputs {
		if err := writeStepInputs(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	{
		keys := make([]string, 0, len(v.Captures))
		for k := range v.Captures {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if err := writeStepCaptures(q, childKey(key, i), root, key, i, capture{k, v.Captures[k]}); err != nil {
				return err
			}
		}
	}
	for i, x := range v.Attempts {
		if err := writeStepAttempts(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowExtractionSteps struct {
	Key, Owner string
	Position   int
	Value      recognition.Step
}

func readExtractionSteps(q queryer, root string) ([]rowExtractionSteps, error) {
	cStepRules, err := readStepRules(q, root)
	if err != nil {
		return nil, err
	}
	cStepInputs, err := readStepInputs(q, root)
	if err != nil {
		return nil, err
	}
	cStepCaptures, err := readStepCaptures(q, root)
	if err != nil {
		return nil, err
	}
	cStepAttempts, err := readStepAttempts(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_rule_id,f_rule_name,f_order,f_target,f_status,f_matched_text,f_extracted_value,f_before,f_after,f_applied,f_reason,f_started_at,f_duration_ms FROM extraction_steps WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowExtractionSteps{}
	for rows.Next() {
		var r rowExtractionSteps
		var v recognition.Step
		v, err = scanExtractionSteps(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		if len(cStepRules) != len(out) {
			return nil, fmt.Errorf("missing required StepRules record")
		}
		for _, c := range cStepRules {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan step_rules")
			}
			out[i].Value.RuleConfig = c.Value
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Inputs = []string{}
		}
		for _, c := range cStepInputs {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan step_inputs")
			}
			out[i].Value.Inputs = append(out[i].Value.Inputs, c.Value)
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Captures = map[string]string{}
		}
		for _, c := range cStepCaptures {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan step_captures")
			}
			out[i].Value.Captures[c.Value.Name] = c.Value.Value
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Attempts = []recognition.Attempt{}
		}
		for _, c := range cStepAttempts {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan step_attempts")
			}
			out[i].Value.Attempts = append(out[i].Value.Attempts, c.Value)
		}
	}
	return out, nil
}

func scanStepRules(s scanner) (recognition.Rule, error) {
	var v recognition.Rule
	var err error
	var raw11 sql.NullString
	var raw14 sql.NullString
	err = s.Scan(&v.ID, &v.Name, &v.Description, &v.Target, &v.Source, &v.Order, &v.Enabled, &v.Kind, &v.Overwrite, &v.Config.Code, &v.Config.SourceCode, &raw11, &v.Config.Pattern, &v.Config.Capture, &raw14)
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

func putStepRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	j11, err := dynamicJSON(v.Config.Params)
	if err != nil {
		return err
	}
	j14, err := dynamicJSON(v.Config.Value)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO step_rules(k,root,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_overwrite,f_config_code,f_config_source_code,f_config_params,f_config_pattern,f_config_capture,f_config_value) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.ID, v.Name, v.Description, v.Target, v.Source, v.Order, v.Enabled, v.Kind, v.Overwrite, v.Config.Code, v.Config.SourceCode, j11, v.Config.Pattern, v.Config.Capture, j14)
	return err
}

func writeStepRules(q queryer, key, root, owner string, position int, v recognition.Rule) error {
	if err := putStepRules(q, key, root, owner, position, v); err != nil {
		return err
	}
	for i, x := range v.Examples {
		if err := writeStepRuleExamples(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowStepRules struct {
	Key, Owner string
	Position   int
	Value      recognition.Rule
}

func readStepRules(q queryer, root string) ([]rowStepRules, error) {
	cStepRuleExamples, err := readStepRuleExamples(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_id,f_name,f_description,f_target,f_source,f_order,f_enabled,f_kind,f_overwrite,f_config_code,f_config_source_code,f_config_params,f_config_pattern,f_config_capture,f_config_value FROM step_rules WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowStepRules{}
	for rows.Next() {
		var r rowStepRules
		var v recognition.Rule
		v, err = scanStepRules(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		for _, c := range cStepRuleExamples {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan step_rule_examples")
			}
			out[i].Value.Examples = append(out[i].Value.Examples, c.Value)
		}
	}
	return out, nil
}

func scanStepRuleExamples(s scanner) (recognition.Example, error) {
	var v recognition.Example
	var err error
	var raw2 sql.NullString
	err = s.Scan(&v.Path, &v.Root, &raw2)
	if err != nil {
		return v, err
	}
	if raw2.Valid {
		if err = decode(raw2.String, &v.Expected); err != nil {
			return v, err
		}
	}
	return v, nil
}

func writeStepRuleExamples(q queryer, key, root, owner string, position int, v recognition.Example) error {
	j2, err := dynamicJSON(v.Expected)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO step_rule_examples(k,root,owner,position,f_path,f_root,f_expected) VALUES(?,?,?,?,?,?,?)", key, root, owner, position, v.Path, v.Root, j2)
	return err
}

type rowStepRuleExamples struct {
	Key, Owner string
	Position   int
	Value      recognition.Example
}

func readStepRuleExamples(q queryer, root string) ([]rowStepRuleExamples, error) {
	rows, err := q.Query("SELECT k,owner,position,f_path,f_root,f_expected FROM step_rule_examples WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowStepRuleExamples{}
	for rows.Next() {
		var r rowStepRuleExamples
		var v recognition.Example
		v, err = scanStepRuleExamples(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanStepInputs(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeStepInputs(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO step_inputs(k,root,owner,position,value) VALUES(?,?,?,?,?)", key, root, owner, position, v)
	return err
}

type rowStepInputs struct {
	Key, Owner string
	Position   int
	Value      string
}

func readStepInputs(q queryer, root string) ([]rowStepInputs, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM step_inputs WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowStepInputs{}
	for rows.Next() {
		var r rowStepInputs
		var v string
		v, err = scanStepInputs(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanStepCaptures(s scanner) (capture, error) {
	var v capture
	var err error
	err = s.Scan(&v.Name, &v.Value)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeStepCaptures(q queryer, key, root, owner string, position int, v capture) error {
	_, err := q.Exec("INSERT INTO step_captures(k,root,owner,position,f_name,f_value) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.Name, v.Value)
	return err
}

type rowStepCaptures struct {
	Key, Owner string
	Position   int
	Value      capture
}

func readStepCaptures(q queryer, root string) ([]rowStepCaptures, error) {
	rows, err := q.Query("SELECT k,owner,position,f_name,f_value FROM step_captures WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowStepCaptures{}
	for rows.Next() {
		var r rowStepCaptures
		var v capture
		v, err = scanStepCaptures(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanStepAttempts(s scanner) (recognition.Attempt, error) {
	var v recognition.Attempt
	var err error
	err = s.Scan(&v.Input, &v.Matched, &v.MatchedText, &v.Error)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putStepAttempts(q queryer, key, root, owner string, position int, v recognition.Attempt) error {
	_, err := q.Exec("INSERT INTO step_attempts(k,root,owner,position,f_input,f_matched,f_matched_text,f_error) VALUES(?,?,?,?,?,?,?,?)", key, root, owner, position, v.Input, v.Matched, v.MatchedText, v.Error)
	return err
}

func writeStepAttempts(q queryer, key, root, owner string, position int, v recognition.Attempt) error {
	if err := putStepAttempts(q, key, root, owner, position, v); err != nil {
		return err
	}
	{
		keys := make([]string, 0, len(v.Captures))
		for k := range v.Captures {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if err := writeAttemptCaptures(q, childKey(key, i), root, key, i, capture{k, v.Captures[k]}); err != nil {
				return err
			}
		}
	}
	for i, x := range v.Trace {
		if err := writeAttemptEvents(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowStepAttempts struct {
	Key, Owner string
	Position   int
	Value      recognition.Attempt
}

func readStepAttempts(q queryer, root string) ([]rowStepAttempts, error) {
	cAttemptCaptures, err := readAttemptCaptures(q, root)
	if err != nil {
		return nil, err
	}
	cAttemptEvents, err := readAttemptEvents(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_input,f_matched,f_matched_text,f_error FROM step_attempts WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowStepAttempts{}
	for rows.Next() {
		var r rowStepAttempts
		var v recognition.Attempt
		v, err = scanStepAttempts(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
			out[i].Value.Captures = map[string]string{}
		}
		for _, c := range cAttemptCaptures {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan attempt_captures")
			}
			out[i].Value.Captures[c.Value.Name] = c.Value.Value
		}
	}
	{
		index := map[string]int{}
		for i := range out {
			index[out[i].Key] = i
		}
		for i := range out {
			out[i].Value.Trace = []recognition.ScriptEvent{}
		}
		for _, c := range cAttemptEvents {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan attempt_events")
			}
			out[i].Value.Trace = append(out[i].Value.Trace, c.Value)
		}
	}
	return out, nil
}

func scanAttemptCaptures(s scanner) (capture, error) {
	var v capture
	var err error
	err = s.Scan(&v.Name, &v.Value)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeAttemptCaptures(q queryer, key, root, owner string, position int, v capture) error {
	_, err := q.Exec("INSERT INTO attempt_captures(k,root,owner,position,f_name,f_value) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.Name, v.Value)
	return err
}

type rowAttemptCaptures struct {
	Key, Owner string
	Position   int
	Value      capture
}

func readAttemptCaptures(q queryer, root string) ([]rowAttemptCaptures, error) {
	rows, err := q.Query("SELECT k,owner,position,f_name,f_value FROM attempt_captures WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowAttemptCaptures{}
	for rows.Next() {
		var r rowAttemptCaptures
		var v capture
		v, err = scanAttemptCaptures(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanAttemptEvents(s scanner) (recognition.ScriptEvent, error) {
	var v recognition.ScriptEvent
	var err error
	var raw6 sql.NullString
	err = s.Scan(&v.Phase, &v.Line, &v.Column, &v.Function, &v.Operation, &v.Message, &raw6)
	if err != nil {
		return v, err
	}
	if raw6.Valid {
		if err = decode(raw6.String, &v.Data); err != nil {
			return v, err
		}
	}
	return v, nil
}

func writeAttemptEvents(q queryer, key, root, owner string, position int, v recognition.ScriptEvent) error {
	j6, err := dynamicJSON(v.Data)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO attempt_events(k,root,owner,position,f_phase,f_line,f_column,f_function,f_operation,f_message,f_data) VALUES(?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.Phase, v.Line, v.Column, v.Function, v.Operation, v.Message, j6)
	return err
}

type rowAttemptEvents struct {
	Key, Owner string
	Position   int
	Value      recognition.ScriptEvent
}

func readAttemptEvents(q queryer, root string) ([]rowAttemptEvents, error) {
	rows, err := q.Query("SELECT k,owner,position,f_phase,f_line,f_column,f_function,f_operation,f_message,f_data FROM attempt_events WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowAttemptEvents{}
	for rows.Next() {
		var r rowAttemptEvents
		var v recognition.ScriptEvent
		v, err = scanAttemptEvents(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
