package sqlite

import (
	"database/sql"
	"errors"
	"strings"
)

// A trace's children are immutable and are created only at finalization. Group
// rows by table, preserving parent-before-child order, within the same transaction.
type traceBatch struct {
	queryer
	groups []*traceRows
	bySQL  map[string]*traceRows
}
type traceRows struct {
	prefix, values string
	args           []any
	width          int
}

func newTraceBatch(q queryer) *traceBatch {
	return &traceBatch{queryer: q, bySQL: map[string]*traceRows{}}
}
func (b *traceBatch) Exec(query string, args ...any) (sql.Result, error) {
	prefix, values, ok := strings.Cut(query, " VALUES")
	if !ok || !strings.HasPrefix(prefix, "INSERT INTO ") || strings.Contains(values, "ON CONFLICT") || len(args) == 0 {
		return nil, errors.New("轨迹批量写入只接受新增子记录")
	}
	g := b.bySQL[query]
	if g == nil {
		g = &traceRows{prefix: prefix + " VALUES", values: values, width: len(args)}
		b.groups = append(b.groups, g)
		b.bySQL[query] = g
	}
	g.args = append(g.args, args...)
	return nil, nil // Child writers do not consume sql.Result.
}
func (b *traceBatch) flush() error {
	for _, g := range b.groups {
		// Stay below SQLite's bind limit, also bounding each statement's size.
		rowsPerBatch := 900 / g.width
		for start := 0; start < len(g.args); {
			end := min(len(g.args), start+rowsPerBatch*g.width)
			count := (end - start) / g.width
			query := g.prefix + strings.TrimSuffix(strings.Repeat(g.values+",", count), ",")
			if _, err := b.queryer.Exec(query, g.args[start:end]...); err != nil {
				return err
			}
			start = end
		}
	}
	return nil
}
