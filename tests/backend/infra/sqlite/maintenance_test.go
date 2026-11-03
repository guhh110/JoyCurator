package sqlite

import (
	"context"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"testing"
	"time"
)

func cleanupTrace(t *testing.T, s *Store, id, status string, finished time.Time) {
	t.Helper()
	v := recognition.Trace{ID: id, Status: "running", FinishedAt: finished.UTC().Format(time.RFC3339Nano)}
	if err := s.StartTrace(v); err != nil {
		t.Fatal(err)
	}
	if status == "running" {
		return
	}
	v.Status = status
	step := recognition.Step{RuleConfig: recognition.DefaultRules()[0], Inputs: []string{"b", "a"}, Captures: map[string]string{"x": "y"}, Attempts: []recognition.Attempt{{Captures: map[string]string{"x": "y"}, Trace: []recognition.ScriptEvent{{Message: "event"}}}}}
	if err := s.FinishTrace(v, []recognition.Step{step}); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupPreviewAndAtomicDeletion(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	before := now.Add(-10 * 24 * time.Hour)
	for _, status := range []string{"completed", "failed", "cancelled", "interrupted", "running"} {
		cleanupTrace(t, s, status, status, now.Add(-20*24*time.Hour))
	}
	cleanupTrace(t, s, "recent", "completed", now.Add(-time.Hour))
	cleanupTrace(t, s, "boundary", "completed", before)
	w := matching.Workspace{ID: "workspace", Items: []matching.Item{{TraceID: "completed"}}}
	if err := s.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	cutoff := now.Unix() - 10
	for key, expires := range map[string]int64{"old": cutoff - 1, "equal": cutoff, "after-preview": cutoff + 1, "live": now.Unix() + 1000} {
		if err := s.SaveCache(context.Background(), key, []byte("{}"), expires); err != nil {
			t.Fatal(err)
		}
	}
	r := model.CleanupRequest{Before: before.Format(time.RFC3339Nano), Traces: true, Cache: true, CacheBefore: cutoff}
	p, err := s.PreviewCleanup(r)
	if err != nil || p.Traces != 4 || p.Cache != 2 {
		t.Fatal(p, err)
	}
	if _, _, err := s.TraceDetail("completed"); err != nil {
		t.Fatal("preview changed data", err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_cleanup BEFORE DELETE ON tmdb_cache BEGIN SELECT RAISE(ABORT,'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	if counts, err := s.Cleanup(p.Request); err == nil || counts != (model.CleanupCounts{}) {
		t.Fatal("partial deletion reported", counts, err)
	}
	if _, steps, err := s.TraceDetail("completed"); err != nil || len(steps) != 1 || len(steps[0].Attempts[0].Trace) != 1 {
		t.Fatal("trace children did not roll back", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_cleanup"); err != nil {
		t.Fatal(err)
	}
	counts, err := s.Cleanup(p.Request)
	if err != nil || counts != p.CleanupCounts {
		t.Fatal(counts, p, err)
	}
	for _, id := range []string{"completed", "failed", "cancelled", "interrupted"} {
		if _, _, err := s.TraceDetail(id); err == nil {
			t.Fatal("expired trace survived", id)
		}
		for _, table := range []string{"trace_metadata", "trace_episodes", "extraction_steps", "step_rules", "step_rule_examples", "step_inputs", "step_attempts", "step_captures", "attempt_captures", "attempt_events"} {
			var n int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE root=?", id).Scan(&n); err != nil || n != 0 {
				t.Fatal(table, n, err)
			}
		}
	}
	for _, id := range []string{"running", "recent", "boundary"} {
		if _, _, err := s.TraceDetail(id); err != nil {
			t.Fatal("unselected trace removed", id, err)
		}
	}
	for _, key := range []string{"after-preview", "live"} {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM tmdb_cache WHERE key=?", key).Scan(&n); err != nil || n != 1 {
			t.Fatal(key, n, err)
		}
	}
	current, err := s.Workspace(w.ID)
	if err != nil || current.Revision != w.Revision || current.Items[0].TraceID != "completed" {
		t.Fatal("workspace changed", current, err)
	}
}
