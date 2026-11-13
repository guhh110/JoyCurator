package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

func TestRelationalWorkspaceSnapshotsAndCheckpoint(t *testing.T) {
	s := openStore(t)
	zero := 0
	empty := ""
	entry := model.Entry{TMDBID: 1, Type: "tv", Title: "original", Genres: []string{"g2", "g1"}, Countries: []string{"CN"}, Cast: []model.Actor{{Name: "actor", Role: "role"}}}
	w := matching.Workspace{Purpose: "organize", ID: "workspace", State: "completed", Paths: []string{"/b", "/a"}, Items: []matching.Item{
		{File: model.File{Path: "/b", MTime: 123456789012345678, Season: 0, Episodes: []int{0, 2}}, Metadata: recognition.Metadata{TMDBID: &zero, Title: &empty, Season: &zero, Episodes: []int{0, 1}}, Entry: &entry, Execution: &matching.FileExecution{Locked: true}},
		{File: model.File{Path: "/a"}, Metadata: recognition.Metadata{Episodes: []int{}}},
	}}
	if err := s.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	got, err := s.Workspace(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].Execution != nil || got.Items[1].Metadata.Title != nil || got.Items[0].Metadata.Title == nil || *got.Items[0].Metadata.Season != 0 || !reflect.DeepEqual(got.Paths, w.Paths) || !reflect.DeepEqual(got.Items[0].Entry, &entry) || got.Items[0].MTime != w.Items[0].MTime {
		t.Fatal(got)
	}
	// Checkpointing one item must not touch any row of its neighbour.
	untouched := childKey(w.ID, 1)
	if _, err = s.db.Exec(`CREATE TRIGGER untouched_item_update BEFORE UPDATE ON workspace_items WHEN OLD.k='` + untouched + `' BEGIN SELECT RAISE(ABORT,'untouched'); END;
 CREATE TRIGGER untouched_item_delete BEFORE DELETE ON workspace_items WHEN OLD.k='` + untouched + `' BEGIN SELECT RAISE(ABORT,'untouched'); END;`); err != nil {
		t.Fatal(err)
	}
	got.Items[0].Entry.Title = "changed"
	got.Items[0].Metadata.Episodes = []int{3}
	got.Completed = 1
	if err = s.CheckpointWorkspace(&got, []int{0}); err != nil {
		t.Fatal(err)
	}
	current, err := s.Workspace(w.ID)
	if err != nil || current.Items[0].Entry.Title != "changed" || !reflect.DeepEqual(current.Items[0].File.Episodes, []int{0, 2}) {
		t.Fatal(current, err)
	}
	oldRevision := current.Revision
	if _, err = s.db.Exec(`CREATE TRIGGER fail_workspace_child BEFORE INSERT ON metadata_episodes BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	current.Items[0].Reason = "uncommitted"
	if err = s.CheckpointWorkspace(&current, []int{0}); err == nil || current.Revision != oldRevision {
		t.Fatal(current, err)
	}
	stored, err := s.Workspace(w.ID)
	if err != nil || stored.Revision != oldRevision || stored.Items[0].Reason == "uncommitted" {
		t.Fatal(stored, err)
	}
	stale := stored
	stale.Revision--
	if err = s.CheckpointWorkspace(&stale, nil); err == nil {
		t.Fatal("stale checkpoint accepted")
	}
}

func TestRelationalOperationsUndoAndAtomicCheckpoint(t *testing.T) {
	s := openStore(t)
	duration, speed := 42.3, 118600000.0
	r := organizer.Run{ID: "run", Purpose: "organize", State: "running", Ops: []organizer.Work{
		{Operation: organizer.Operation{ID: 99, Kind: "video", Target: "/target", Content: []byte{0, 255, 1}, Status: "published", DurationMs: &duration, SpeedBps: &speed, Undo: &organizer.Operation{ID: 99, Status: "pending", Undo: &organizer.Operation{Status: "preparing"}}}, Owner: "/source", Owners: []string{"/b", "/a"}},
		{Operation: organizer.Operation{ID: 99, Target: "/second", Status: "pending", Undo: &organizer.Operation{Status: "pending"}}},
	}, Mappings: []model.MediaMapping{{Entry: &model.Entry{Type: "movie", TMDBID: 1, Title: "fixture"}, Path: "/target", Source: "/source", Episodes: []int{0, 2}, State: "pending"}}}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Run(r.ID)
	wantOps, _ := json.Marshal(r.Ops)
	gotOps, _ := json.Marshal(got.Ops)
	if err != nil || !bytes.Equal(gotOps, wantOps) {
		t.Fatal(got, err)
	}
	untouched := childKey(r.ID, 1)
	if _, err = s.db.Exec(`CREATE TRIGGER untouched_op_update BEFORE UPDATE ON operations WHEN OLD.k='` + untouched + `' BEGIN SELECT RAISE(ABORT,'untouched'); END;
 CREATE TRIGGER untouched_op_delete BEFORE DELETE ON operations WHEN OLD.k='` + untouched + `' BEGIN SELECT RAISE(ABORT,'untouched'); END;
 CREATE TRIGGER reject_registration BEFORE INSERT ON media_mappings BEGIN SELECT RAISE(ABORT,'full'); END;`); err != nil {
		t.Fatal(err)
	}
	r.Ops[0].Status = "completed"
	updatedDuration, updatedSpeed := 50.25, 100000000.0
	r.Ops[0].DurationMs, r.Ops[0].SpeedBps = &updatedDuration, &updatedSpeed
	r.Ops[0].Undo = nil
	r.Completed = 1
	r.Mappings[0].RunID = r.ID
	r.Mappings[0].Identity = "inode"
	if err = s.SaveCheckpoint(r, []int{0}, []int{0}, r.Mappings); err == nil {
		t.Fatal("partial checkpoint accepted")
	}
	current, err := s.Run(r.ID)
	if err != nil || current.Completed != 0 || current.Ops[0].Status != "published" || current.Mappings[0].RunID != "" {
		t.Fatal(current, err)
	}
	if current.Ops[0].DurationMs == nil || *current.Ops[0].DurationMs != duration || current.Ops[0].SpeedBps == nil || *current.Ops[0].SpeedBps != speed {
		t.Fatal("failed checkpoint changed metrics", current.Ops[0])
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_registration"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveCheckpoint(r, []int{0}, []int{0}, r.Mappings); err != nil {
		t.Fatal(err)
	}
	current, err = s.Run(r.ID)
	mapped, e := s.Mapping("/target")
	if err != nil || e != nil || mapped == nil || current.Ops[0].Undo != nil || current.Completed != 1 || !reflect.DeepEqual(mapped.Episodes, []int{0, 2}) {
		t.Fatal(current, mapped, err, e)
	}
	if current.Ops[0].DurationMs == nil || *current.Ops[0].DurationMs != updatedDuration || current.Ops[0].SpeedBps == nil || *current.Ops[0].SpeedBps != updatedSpeed || current.Ops[1].DurationMs != nil || current.Ops[1].SpeedBps != nil {
		t.Fatal("checkpoint did not save metrics only for the changed item", current.Ops)
	}
}

func TestOperationMetricsSurviveRestartWithoutEstimatingUnfinishedItems(t *testing.T) {
	dir := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot, settings.TargetRoot = "/source", "/target"
	s, err := Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	duration, speed, failedDuration := 42300.25, 118600000.0, 17.5
	r := organizer.Run{ID: "metrics", Purpose: "organize", State: "queued", Ops: []organizer.Work{
		{Operation: organizer.Operation{Status: "completed", DurationMs: &duration, SpeedBps: &speed}},
		{Operation: organizer.Operation{Status: "preparing"}},
		{Operation: organizer.Operation{Status: "pending", DurationMs: &failedDuration}},
	}}
	if err = s.Enqueue([]organizer.Run{r}); err != nil {
		s.Close()
		t.Fatal(err)
	}
	r.State = "running"
	if err = s.SaveCheckpoint(r, nil, nil, nil); err != nil {
		s.Close()
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Run(r.ID)
	if err != nil || got.State != "interrupted" {
		t.Fatal(got, err)
	}
	wantOps, _ := json.Marshal(r.Ops)
	gotOps, _ := json.Marshal(got.Ops)
	if !bytes.Equal(wantOps, gotOps) {
		t.Fatal("restart changed or invented item metrics", string(gotOps))
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatal(version, err)
	}
}

func TestRelationalTraceDynamicValuesAndRules(t *testing.T) {
	s := openStore(t)
	r := recognition.DefaultPipeline().Tokenizer
	r.Config.Params = json.RawMessage(`{"x":[1,true]}`)
	r.Examples = []recognition.Example{{Paths: []string{"/a"}, Root: "/", Targets: []string{}, Expected: json.RawMessage(`null`)}}
	if err := s.SaveRules(pipelineWith(r)); err != nil {
		t.Fatal(err)
	}
	rules, err := s.LoadRules()
	if err != nil || !reflect.DeepEqual(rules, pipelineWith(r)) {
		t.Fatal(rules, err)
	}
	trace := recognition.Trace{ID: "trace", Status: "running", Path: "/a", Root: "/", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = s.StartTrace(trace); err != nil {
		t.Fatal(err)
	}
	snapshot := sampleSnapshot(t)
	snapshot.Pipeline = pipelineWith(r)
	trace.Status = "completed"
	trace.Metadata.Episodes = []int{0, 1}
	snapshot.Documents[0].Candidates[0].Value = math.NaN()
	if err = s.FinishTrace(trace, snapshot); err == nil {
		t.Fatal("non JSON dynamic value accepted")
	}
	current, saved, err := s.TraceDetail(trace.ID)
	if err != nil || current.Status != "running" || len(saved.Documents) != 0 {
		t.Fatal(current, saved, err)
	}
	snapshot = sampleSnapshot(t)
	snapshot.Pipeline = pipelineWith(r)
	if err = s.FinishTrace(trace, snapshot); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRules(emptyPipeline()); err != nil {
		t.Fatal(err)
	}
	_, saved, err = s.TraceDetail(trace.ID)
	if err != nil || !sameSnapshot(t, saved, snapshot) {
		t.Fatal(saved, err)
	}
	var extra any
	if err = decode(`true false`, &extra); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	r.Config.Params = json.RawMessage(`{"x":`)
	if err = s.SaveRules(pipelineWith(r)); err == nil {
		t.Fatal("invalid raw JSON accepted")
	}
	rules, err = s.LoadRules()
	if err != nil || len(rules.Annotators) != 0 {
		t.Fatal("failed save changed rules", rules, err)
	}
	if err = s.DeleteTrace(trace.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"trace_metadata", "trace_episodes", "snapshot_documents", "snapshot_rules", "snapshot_rule_examples", "snapshot_context", "snapshot_candidates", "snapshot_sources", "snapshot_decisions", "snapshot_references"} {
		var n int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
}

func TestStorageFormatRejectionDoesNotRewriteDatabase(t *testing.T) {
	for _, kind := range []string{"legacy", "previous-version", "broken"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "joycurator.db")
			settings := model.DefaultSettings()
			settings.SourceRoot = "/source"
			settings.TargetRoot = "/target"
			if kind == "legacy" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`CREATE TABLE runs(id TEXT PRIMARY KEY,value TEXT NOT NULL);INSERT INTO runs VALUES('preserve','{"state":"running"}')`); err != nil {
					t.Fatal(err)
				}
				db.Close()
			} else {
				s, err := Open(dir, settings)
				if err != nil {
					t.Fatal(err)
				}
				statement := "DROP INDEX runs_state"
				if kind == "previous-version" {
					statement = "PRAGMA user_version=6"
				}
				if _, err = s.db.Exec(statement); err != nil {
					t.Fatal(err)
				}
				s.Close()
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			next, err := Open(dir, settings)
			if err == nil {
				next.Close()
				t.Fatal("unsupported schema accepted")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatal("missing database path", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected database was rewritten", err)
			}
		})
	}
}

func TestEmptyRuleChainSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	settings := model.DefaultSettings()
	settings.SourceRoot = "/source"
	settings.TargetRoot = "/target"
	s, err := Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRules(emptyPipeline()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rules, err := s.LoadRules()
	if err != nil || len(rules.Annotators) != 0 {
		t.Fatal(rules, err)
	}
}

func TestRecoveryIsAtomicAcrossAllBusinessRecords(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "r", Purpose: "organize", State: "queued"}
	if err := s.Enqueue([]organizer.Run{r}); err != nil {
		t.Fatal(err)
	}
	r.State = "running"
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	w := matching.Workspace{Purpose: "organize", ID: "w", State: "running"}
	if err := s.SaveWorkspace(&w); err != nil {
		t.Fatal(err)
	}
	if err := s.StartTrace(recognition.Trace{ID: "t", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_recovery BEFORE UPDATE ON extraction_traces BEGIN SELECT RAISE(ABORT,'full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.recoverQueue(); err == nil {
		t.Fatal("failed recovery accepted")
	}
	got, err := s.Run(r.ID)
	workspace, e := s.Workspace(w.ID)
	trace, _, te := s.TraceDetail("t")
	queued, qe := s.QueuedRuns()
	if err != nil || e != nil || te != nil || qe != nil || got.State != "running" || workspace.State != "running" || trace.Status != "running" || len(queued) != 1 {
		t.Fatal(got, workspace, trace, queued, err, e, te, qe)
	}
}

func TestNewDatabaseInitializationRollsBackAsOneTransaction(t *testing.T) {
	dir := t.TempDir()
	invalid := model.DefaultSettings()
	invalid.SourceRoot = "relative"
	invalid.TargetRoot = "/target"
	if s, err := Open(dir, invalid); err == nil {
		s.Close()
		t.Fatal("invalid initial settings accepted")
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "joycurator.db"))
	if err != nil {
		t.Fatal(err)
	}
	var count, version int
	if err = db.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if count != 0 || version != 0 {
		t.Fatal("partial initialization committed", count, version)
	}
	invalid.SourceRoot = "/source"
	s, err := Open(dir, invalid)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestOperationStorageKeysDoNotAliasNestedBusinessIDs(t *testing.T) {
	s := openStore(t)
	first := organizer.Run{ID: "a", Purpose: "organize", State: "completed", Ops: []organizer.Work{{Operation: organizer.Operation{Undo: &organizer.Operation{Status: "pending"}}}}}
	second := organizer.Run{ID: childKey(first.ID, 0), Purpose: "organize", State: "completed", Ops: []organizer.Work{{}, {}}}
	if err := s.SaveRun(first); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(second); err != nil {
		t.Fatal(err)
	}
	got, err := s.Run(first.ID)
	if err != nil || len(got.Ops) != 1 || got.Ops[0].Undo == nil {
		t.Fatal(got, err)
	}
}

func TestScrapeArtifactLoadsOnlyItsOwnRecursiveOperation(t *testing.T) {
	s := openStore(t)
	r := organizer.Run{ID: "scrape", Purpose: "scrape", State: "completed", Ops: []organizer.Work{{Operation: organizer.Operation{Kind: "nfo", Target: "/target/a.nfo", Status: "completed", Content: []byte("nfo"), Undo: &organizer.Operation{Status: "pending", Undo: &organizer.Operation{Status: "preparing"}}}, Owners: []string{"/b", "/a"}}, {Operation: organizer.Operation{Content: []byte("other")}}}}
	if err := s.SaveRun(r); err != nil {
		t.Fatal(err)
	}
	artifact, err := s.ScrapeArtifact("/target/a.nfo")
	if err != nil || artifact == nil || !reflect.DeepEqual(*artifact, r.Ops[0]) {
		t.Fatal(artifact, err)
	}
}
