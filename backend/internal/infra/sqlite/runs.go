package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
)

func (a *Store) SaveRun(r organizer.Run) error {
	return transaction(a.db, func(tx queryer) error { return saveRun(tx, r) })
}
func saveRun(q queryer, r organizer.Run) error {
	if err := putRuns(q, r.ID, r.ID, "", 0, r); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM operations WHERE root=?", r.ID); err != nil {
		return err
	}
	if _, err := q.Exec("DELETE FROM run_mappings WHERE root=?", r.ID); err != nil {
		return err
	}
	for i, op := range r.Ops {
		if err := writeOperation(q, r.ID, i, op); err != nil {
			return err
		}
	}
	for i, m := range r.Mappings {
		if err := writeRunMappings(q, childKey(r.ID, i), r.ID, r.ID, i, m); err != nil {
			return err
		}
	}
	return nil
}
func writeOperation(q queryer, run string, pos int, op organizer.Work) error {
	original := pos
	key := childKey(run, pos)
	if _, err := q.Exec("DELETE FROM operations WHERE undo_parent=?", key); err != nil {
		return err
	}
	seen := map[*organizer.Operation]bool{}
	var parent any
	for depth := 0; ; depth++ {
		k := key
		if depth > 0 {
			k = "undo:" + childKey(key, depth)
		}
		if err := putOperations(q, k, run, run, pos, op, parent); err != nil {
			return err
		}
		if _, err := q.Exec("DELETE FROM operation_owners WHERE owner=?", k); err != nil {
			return err
		}
		for i, owner := range op.Owners {
			if err := writeOperationOwners(q, childKey(k, i), run, k, i, owner); err != nil {
				return err
			}
		}
		if op.Undo == nil {
			break
		}
		if seen[op.Undo] {
			return errors.New("撤销操作存在循环引用")
		}
		seen[op.Undo] = true
		parent = k
		op = organizer.Work{Operation: *op.Undo}
		d := depth + 1
		pos = -((original+d)*(original+d+1)/2 + d + 1)
	}
	return nil
}
func (a *Store) Run(id string) (organizer.Run, error) {
	return snapshotRead(a.db, func(q queryer) (organizer.Run, error) { return readRun(q, id) })
}
func readRun(q queryer, id string) (organizer.Run, error) {
	r, err := scanRuns(q.QueryRow("SELECT "+columnsRuns+" FROM runs WHERE id=?", id))
	r.ID = id
	if err != nil {
		return r, err
	}
	ops, err := readOperations(q, id)
	if err != nil {
		return r, err
	}
	// Only root operations belong to the ordered public list. Undo descendants retain their own fields.
	rows, err := q.Query("SELECT k,undo_parent FROM operations WHERE root=?", id)
	if err != nil {
		return r, err
	}
	parents := map[string]string{}
	for rows.Next() {
		var k string
		var p sql.NullString
		if err = rows.Scan(&k, &p); err != nil {
			rows.Close()
			return r, err
		}
		parents[k] = p.String
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return r, err
	}
	index := map[string]*organizer.Work{}
	for i := range ops {
		index[ops[i].Key] = &ops[i].Value
	}
	for _, o := range ops {
		cursor := o.Key
		visited := map[string]bool{}
		for parents[cursor] != "" {
			if visited[cursor] {
				return r, errors.New("撤销操作存在循环引用")
			}
			visited[cursor] = true
			cursor = parents[cursor]
			if _, ok := index[cursor]; !ok {
				return r, errors.New("撤销操作父记录不存在")
			}
		}
		if p := parents[o.Key]; p != "" {
			parent, ok := index[p]
			if !ok {
				return r, errors.New("撤销操作父记录不存在")
			}
			parent.Undo = &index[o.Key].Operation
		}
	}
	r.Ops = []organizer.Work{}
	for _, o := range ops {
		if parents[o.Key] == "" {
			r.Ops = append(r.Ops, *index[o.Key])
		}
	}
	mappings, err := readRunMappings(q, id)
	if err != nil {
		return r, err
	}
	for _, m := range mappings {
		r.Mappings = append(r.Mappings, m.Value)
	}
	return r, nil
}
func (a *Store) readRuns(query string, args ...any) ([]organizer.Run, error) {
	return queryRuns(a.db, query, args...)
}
func queryRuns(q queryer, query string, args ...any) ([]organizer.Run, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []organizer.Run{}
	for rows.Next() {
		var id string
		r, e := scanRuns(prefixScanner{rows, []any{&id}})
		if e != nil {
			return nil, e
		}
		r.ID = id
		out = append(out, r)
	}
	return out, rows.Err()
}
func (a *Store) fullRuns(query string, args ...any) ([]organizer.Run, error) {
	items, err := a.readRuns(query, args...)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i], err = a.Run(items[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func runColumns(alias string) string {
	out := alias + ".id"
	for _, c := range splitColumns(columnsRuns) {
		out += "," + alias + "." + c
	}
	return out
}
func (a *Store) RunsByPurpose(page int, purpose string) (organizer.RunPage, error) {
	v := organizer.RunPage{Page: page}
	if err := a.db.QueryRow("SELECT COUNT(*) FROM runs WHERE f_purpose=?", purpose).Scan(&v.Total); err != nil {
		return v, err
	}
	var err error
	v.Items, err = a.readRuns("SELECT id,"+columnsRuns+" FROM runs WHERE f_purpose=? ORDER BY sequence DESC LIMIT 20 OFFSET ?", purpose, (page-1)*20)
	return v, err
}
func (a *Store) BatchSummaries(id string) ([]organizer.Run, error) {
	return a.readRuns("SELECT id,"+columnsRuns+" FROM runs WHERE f_batch_id=? ORDER BY sequence", id)
}
func (a *Store) UnfinishedRuns() ([]organizer.Run, error) {
	return a.fullRuns("SELECT id," + columnsRuns + " FROM runs WHERE f_state IN ('running','queued')")
}
func (a *Store) Shared(id, target, identity string) (bool, error) {
	var n int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM runs r JOIN operations o ON o.root=r.id WHERE r.id<>? AND r.f_purpose='scrape' AND o.undo_parent IS NULL AND o.f_target=? AND ((r.f_state<>'undone' AND o.f_status IN ('completed','published') AND o.f_identity=?) OR (o.f_replace_identity=? AND o.f_status IN ('completed','published','undone')))`, id, target, identity, identity).Scan(&n)
	return n > 0, err
}
func (a *Store) ScrapeArtifact(target string) (*organizer.Work, error) {
	return snapshotRead(a.db, func(q queryer) (*organizer.Work, error) { return scrapeArtifact(q, target) })
}
func scrapeArtifact(q queryer, target string) (*organizer.Work, error) {
	var key string
	err := q.QueryRow(`SELECT o.k FROM runs r JOIN operations o ON o.root=r.id WHERE r.f_purpose='scrape' AND r.f_state<>'undone' AND o.undo_parent IS NULL AND o.f_target=? AND o.f_status='completed' ORDER BY r.sequence DESC LIMIT 1`, target).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load this artifact's recursive undo chain, without reading the other operations or content in its run.
	const chain = "WITH RECURSIVE chain(k) AS (SELECT ? UNION SELECT o.k FROM operations o JOIN chain c ON o.undo_parent=c.k) "
	rows, err := q.Query(chain+"SELECT k,undo_parent,"+columnsOperations+" FROM operations WHERE k IN (SELECT k FROM chain)", key)
	if err != nil {
		return nil, err
	}
	values := map[string]*organizer.Work{}
	parents := map[string]string{}
	for rows.Next() {
		var k string
		var parent sql.NullString
		op, e := scanOperations(prefixScanner{rows, []any{&k, &parent}})
		if e != nil {
			rows.Close()
			return nil, e
		}
		values[k] = &op
		parents[k] = parent.String
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for k, op := range values {
		if k != key {
			parent, ok := values[parents[k]]
			if !ok {
				return nil, errors.New("撤销操作父记录不存在")
			}
			parent.Undo = &op.Operation
		}
	}
	value, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("产物操作不存在")
	}
	rows, err = q.Query("SELECT value FROM operation_owners WHERE owner=? ORDER BY position", key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	value.Owners = []string{}
	for rows.Next() {
		var owner string
		if err = rows.Scan(&owner); err != nil {
			return nil, err
		}
		value.Owners = append(value.Owners, owner)
	}
	return value, rows.Err()
}

// SaveCheckpoint commits only the supplied operation and mapping positions. Registration shares the transaction.
func (a *Store) SaveCheckpoint(r organizer.Run, operations, mappings []int, registered []model.MediaMapping) error {
	return transaction(a.db, func(tx queryer) error {
		var exists int
		if err := tx.QueryRow("SELECT COUNT(*) FROM runs WHERE id=?", r.ID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return errors.New("任务不存在")
		}
		if err := putRuns(tx, r.ID, r.ID, "", 0, r); err != nil {
			return err
		}
		for _, i := range operations {
			if i < 0 || i >= len(r.Ops) {
				return errors.New("操作位置无效")
			}
			if err := writeOperation(tx, r.ID, i, r.Ops[i]); err != nil {
				return err
			}
		}
		for _, i := range mappings {
			if i < 0 || i >= len(r.Mappings) {
				return errors.New("映射位置无效")
			}
			if err := writeRunMappings(tx, childKey(r.ID, i), r.ID, r.ID, i, r.Mappings[i]); err != nil {
				return err
			}
		}
		for _, m := range registered {
			if err := saveMapping(tx, m); err != nil {
				return err
			}
		}
		return nil
	})
}
