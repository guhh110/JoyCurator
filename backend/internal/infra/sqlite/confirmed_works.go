package sqlite

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"joycurator/internal/model"
)

func writeWork(q queryer, scope, owner string, entry *model.Entry) error {
	if _, err := q.Exec("DELETE FROM "+scope+"_works WHERE owner=?", owner); err != nil {
		return err
	}
	if entry == nil {
		return errors.New("缺少已确认作品快照")
	}
	value := *entry
	if value.Genres == nil {
		value.Genres = []string{}
	}
	if value.Countries == nil {
		value.Countries = []string{}
	}
	if value.Cast == nil {
		value.Cast = []model.Actor{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	id := fmt.Sprintf("%x", sha256.Sum256(raw))
	result, err := q.Exec("INSERT OR IGNORE INTO confirmed_work_sets(id) VALUES(?)", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		if err = writeConfirmedWorks(q, childKey(id, 0), id, id, 0, value); err != nil {
			return err
		}
	}
	_, err = q.Exec("INSERT INTO "+scope+"_works(owner,work_id) VALUES(?,?)", owner, id)
	return err
}
func readWorks(q queryer, scope, parent, key, where string, args ...any) (map[string]*model.Entry, error) {
	rows, err := q.Query("SELECT w.owner,w.work_id FROM "+scope+"_works w JOIN "+parent+" p ON p."+key+"=w.owner "+where, args...)
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for rows.Next() {
		var owner, id string
		if err = rows.Scan(&owner, &id); err != nil {
			rows.Close()
			return nil, err
		}
		ids[owner] = id
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	cache := map[string]*model.Entry{}
	out := map[string]*model.Entry{}
	for owner, id := range ids {
		v := cache[id]
		if v == nil {
			works, err := readConfirmedWorks(q, id)
			if err != nil {
				return nil, err
			}
			if len(works) != 1 {
				return nil, errors.New("已确认作品快照不存在")
			}
			value := works[0].Value
			v = &value
			cache[id] = v
		}
		out[owner] = v
	}
	return out, nil
}
func scanConfirmedWorks(s scanner) (model.Entry, error) {
	var v model.Entry
	var err error
	err = s.Scan(&v.TMDBID, &v.Type, &v.Title, &v.OriginalTitle, &v.Year, &v.Overview, &v.Date, &v.PosterPath, &v.BackdropPath)
	if err != nil {
		return v, err
	}
	return v, nil
}

func putConfirmedWorks(q queryer, key, root, owner string, position int, v model.Entry) error {
	_, err := q.Exec("INSERT INTO confirmed_works(k,root,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_tmdbid=excluded.f_tmdbid,f_type=excluded.f_type,f_title=excluded.f_title,f_original_title=excluded.f_original_title,f_year=excluded.f_year,f_overview=excluded.f_overview,f_date=excluded.f_date,f_poster_path=excluded.f_poster_path,f_backdrop_path=excluded.f_backdrop_path", key, root, owner, position, v.TMDBID, v.Type, v.Title, v.OriginalTitle, v.Year, v.Overview, v.Date, v.PosterPath, v.BackdropPath)
	return err
}

func writeConfirmedWorks(q queryer, key, root, owner string, position int, v model.Entry) error {
	if err := putConfirmedWorks(q, key, root, owner, position, v); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM confirmed_works_genres WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Genres {
		if err := writeConfirmedWorksGenres(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM confirmed_works_countries WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Countries {
		if err := writeConfirmedWorksCountries(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	if _, err := q.Exec("DELETE FROM confirmed_works_cast WHERE owner=?", key); err != nil {
		return err
	}
	for i, x := range v.Cast {
		if err := writeConfirmedWorksCast(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowConfirmedWorks struct {
	Key, Owner string
	Position   int
	Value      model.Entry
}

func readConfirmedWorks(q queryer, root string) ([]rowConfirmedWorks, error) {
	cConfirmedWorksGenres, err := readConfirmedWorksGenres(q, root)
	if err != nil {
		return nil, err
	}
	cConfirmedWorksCountries, err := readConfirmedWorksCountries(q, root)
	if err != nil {
		return nil, err
	}
	cConfirmedWorksCast, err := readConfirmedWorksCast(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,position,f_tmdbid,f_type,f_title,f_original_title,f_year,f_overview,f_date,f_poster_path,f_backdrop_path FROM confirmed_works WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowConfirmedWorks{}
	for rows.Next() {
		var r rowConfirmedWorks
		var v model.Entry
		v, err = scanConfirmedWorks(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
		for _, c := range cConfirmedWorksGenres {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan confirmed_works_genres")
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
		for _, c := range cConfirmedWorksCountries {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan confirmed_works_countries")
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
		for _, c := range cConfirmedWorksCast {
			i, ok := index[c.Owner]
			if !ok {
				return nil, fmt.Errorf("orphan confirmed_works_cast")
			}
			out[i].Value.Cast = append(out[i].Value.Cast, c.Value)
		}
	}
	return out, nil
}

func scanConfirmedWorksGenres(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeConfirmedWorksGenres(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO confirmed_works_genres(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowConfirmedWorksGenres struct {
	Key, Owner string
	Position   int
	Value      string
}

func readConfirmedWorksGenres(q queryer, root string) ([]rowConfirmedWorksGenres, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM confirmed_works_genres WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowConfirmedWorksGenres{}
	for rows.Next() {
		var r rowConfirmedWorksGenres
		var v string
		v, err = scanConfirmedWorksGenres(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanConfirmedWorksCountries(s scanner) (string, error) {
	var v string
	var err error
	err = s.Scan(&v)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeConfirmedWorksCountries(q queryer, key, root, owner string, position int, v string) error {
	_, err := q.Exec("INSERT INTO confirmed_works_countries(k,root,owner,position,value) VALUES(?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,value=excluded.value", key, root, owner, position, v)
	return err
}

type rowConfirmedWorksCountries struct {
	Key, Owner string
	Position   int
	Value      string
}

func readConfirmedWorksCountries(q queryer, root string) ([]rowConfirmedWorksCountries, error) {
	rows, err := q.Query("SELECT k,owner,position,value FROM confirmed_works_countries WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowConfirmedWorksCountries{}
	for rows.Next() {
		var r rowConfirmedWorksCountries
		var v string
		v, err = scanConfirmedWorksCountries(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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

func scanConfirmedWorksCast(s scanner) (model.Actor, error) {
	var v model.Actor
	var err error
	err = s.Scan(&v.Name, &v.Role)
	if err != nil {
		return v, err
	}
	return v, nil
}

func writeConfirmedWorksCast(q queryer, key, root, owner string, position int, v model.Actor) error {
	_, err := q.Exec("INSERT INTO confirmed_works_cast(k,root,owner,position,f_name,f_role) VALUES(?,?,?,?,?,?) ON CONFLICT(k) DO UPDATE SET root=excluded.root,owner=excluded.owner,position=excluded.position,f_name=excluded.f_name,f_role=excluded.f_role", key, root, owner, position, v.Name, v.Role)
	return err
}

type rowConfirmedWorksCast struct {
	Key, Owner string
	Position   int
	Value      model.Actor
}

func readConfirmedWorksCast(q queryer, root string) ([]rowConfirmedWorksCast, error) {
	rows, err := q.Query("SELECT k,owner,position,f_name,f_role FROM confirmed_works_cast WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rowConfirmedWorksCast{}
	for rows.Next() {
		var r rowConfirmedWorksCast
		var v model.Actor
		v, err = scanConfirmedWorksCast(prefixScanner{rows, []any{&r.Key, &r.Owner, &r.Position}})
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
