package sqlite

import (
	"fmt"
	"joycurator/internal/recognition"
)

type snapshotDecision struct {
	Target   recognition.Target
	Decision recognition.Decision
}

func writeSnapshotContext(q queryer, key, root, owner string, position int, v recognition.BatchInput) error {
	if _, err := q.Exec("INSERT INTO snapshot_context(k,root,owner,position,context_root,source_boundary) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.ContextRoot, v.SourceBoundary); err != nil {
		return err
	}
	for i, x := range v.Paths {
		if err := writeSnapshotPaths(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	for i, x := range v.Targets {
		if err := writeSnapshotTargets(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotContext struct {
	Key, Owner string
	Value      recognition.BatchInput
}

func readSnapshotContext(q queryer, root string) ([]rowSnapshotContext, error) {
	childSnapshotPaths, err := readSnapshotPaths(q, root)
	if err != nil {
		return nil, err
	}
	childSnapshotTargets, err := readSnapshotTargets(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,context_root,source_boundary FROM snapshot_context WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotContext{}
	for rows.Next() {
		var r rowSnapshotContext
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.ContextRoot, &r.Value.SourceBoundary); err != nil {
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
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotPaths {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_paths")
		}
		out[i].Value.Paths = append(out[i].Value.Paths, c.Value)
	}
	for _, c := range childSnapshotTargets {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_targets")
		}
		out[i].Value.Targets = append(out[i].Value.Targets, c.Value)
	}
	return out, nil
}
func writeSnapshotPaths(q queryer, key, root, owner string, position int, v string) error {
	if _, err := q.Exec("INSERT INTO snapshot_paths(k,root,owner,position,value) VALUES(?,?,?,?,?)", key, root, owner, position, v); err != nil {
		return err
	}
	return nil
}

type rowSnapshotPaths struct {
	Key, Owner string
	Value      string
}

func readSnapshotPaths(q queryer, root string) ([]rowSnapshotPaths, error) {
	rows, err := q.Query("SELECT k,owner,value FROM snapshot_paths WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotPaths{}
	for rows.Next() {
		var r rowSnapshotPaths
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotTargets(q queryer, key, root, owner string, position int, v string) error {
	if _, err := q.Exec("INSERT INTO snapshot_targets(k,root,owner,position,value) VALUES(?,?,?,?,?)", key, root, owner, position, v); err != nil {
		return err
	}
	return nil
}

type rowSnapshotTargets struct {
	Key, Owner string
	Value      string
}

func readSnapshotTargets(q queryer, root string) ([]rowSnapshotTargets, error) {
	rows, err := q.Query("SELECT k,owner,value FROM snapshot_targets WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotTargets{}
	for rows.Next() {
		var r rowSnapshotTargets
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotTree(q queryer, key, root, owner string, position int, v recognition.TreeNode) error {
	if _, err := q.Exec("INSERT INTO snapshot_tree(k,root,owner,position,path,parent) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.Path, v.Parent); err != nil {
		return err
	}
	for i, x := range v.FileIDs {
		if err := writeSnapshotTreeFiles(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotTree struct {
	Key, Owner string
	Value      recognition.TreeNode
}

func readSnapshotTree(q queryer, root string) ([]rowSnapshotTree, error) {
	childSnapshotTreeFiles, err := readSnapshotTreeFiles(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,path,parent FROM snapshot_tree WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotTree{}
	for rows.Next() {
		var r rowSnapshotTree
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.Path, &r.Value.Parent); err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.FileIDs = []string{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotTreeFiles {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_tree_files")
		}
		out[i].Value.FileIDs = append(out[i].Value.FileIDs, c.Value)
	}
	return out, nil
}
func writeSnapshotTreeFiles(q queryer, key, root, owner string, position int, v string) error {
	if _, err := q.Exec("INSERT INTO snapshot_tree_files(k,root,owner,position,value) VALUES(?,?,?,?,?)", key, root, owner, position, v); err != nil {
		return err
	}
	return nil
}

type rowSnapshotTreeFiles struct {
	Key, Owner string
	Value      string
}

func readSnapshotTreeFiles(q queryer, root string) ([]rowSnapshotTreeFiles, error) {
	rows, err := q.Query("SELECT k,owner,value FROM snapshot_tree_files WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotTreeFiles{}
	for rows.Next() {
		var r rowSnapshotTreeFiles
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotDocuments(q queryer, key, root, owner string, position int, v recognition.Document) error {
	if _, err := q.Exec("INSERT INTO snapshot_documents(k,root,owner,position,file_id,path,directory) VALUES(?,?,?,?,?,?,?)", key, root, owner, position, v.FileID, v.Path, v.Directory); err != nil {
		return err
	}
	for i, x := range v.Sources {
		if err := writeSnapshotSources(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	for i, x := range v.Tokens {
		if err := writeSnapshotTokens(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	for i, x := range v.Candidates {
		if err := writeSnapshotCandidates(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotDocuments struct {
	Key, Owner string
	Value      recognition.Document
}

func readSnapshotDocuments(q queryer, root string) ([]rowSnapshotDocuments, error) {
	childSnapshotSources, err := readSnapshotSources(q, root)
	if err != nil {
		return nil, err
	}
	childSnapshotTokens, err := readSnapshotTokens(q, root)
	if err != nil {
		return nil, err
	}
	childSnapshotCandidates, err := readSnapshotCandidates(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,file_id,path,directory FROM snapshot_documents WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotDocuments{}
	for rows.Next() {
		var r rowSnapshotDocuments
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.FileID, &r.Value.Path, &r.Value.Directory); err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.Sources = []recognition.TextSource{}
		r.Value.Tokens = []recognition.Token{}
		r.Value.Candidates = []recognition.Candidate{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotSources {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_sources")
		}
		out[i].Value.Sources = append(out[i].Value.Sources, c.Value)
	}
	for _, c := range childSnapshotTokens {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_tokens")
		}
		out[i].Value.Tokens = append(out[i].Value.Tokens, c.Value)
	}
	for _, c := range childSnapshotCandidates {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_candidates")
		}
		out[i].Value.Candidates = append(out[i].Value.Candidates, c.Value)
	}
	return out, nil
}
func writeSnapshotSources(q queryer, key, root, owner string, position int, v recognition.TextSource) error {
	if _, err := q.Exec("INSERT INTO snapshot_sources(k,root,owner,position,id,kind,text,depth) VALUES(?,?,?,?,?,?,?,?)", key, root, owner, position, v.ID, v.Kind, v.Text, v.Depth); err != nil {
		return err
	}
	return nil
}

type rowSnapshotSources struct {
	Key, Owner string
	Value      recognition.TextSource
}

func readSnapshotSources(q queryer, root string) ([]rowSnapshotSources, error) {
	rows, err := q.Query("SELECT k,owner,id,kind,text,depth FROM snapshot_sources WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotSources{}
	for rows.Next() {
		var r rowSnapshotSources
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.ID, &r.Value.Kind, &r.Value.Text, &r.Value.Depth); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotTokens(q queryer, key, root, owner string, position int, v recognition.Token) error {
	if _, err := q.Exec("INSERT INTO snapshot_tokens(k,root,owner,position,id,source_id,parent_id,kind,text,start,end) VALUES(?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.ID, v.SourceID, v.ParentID, v.Kind, v.Text, v.Start, v.End); err != nil {
		return err
	}
	return nil
}

type rowSnapshotTokens struct {
	Key, Owner string
	Value      recognition.Token
}

func readSnapshotTokens(q queryer, root string) ([]rowSnapshotTokens, error) {
	rows, err := q.Query("SELECT k,owner,id,source_id,parent_id,kind,text,start,end FROM snapshot_tokens WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotTokens{}
	for rows.Next() {
		var r rowSnapshotTokens
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.ID, &r.Value.SourceID, &r.Value.ParentID, &r.Value.Kind, &r.Value.Text, &r.Value.Start, &r.Value.End); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotCandidates(q queryer, key, root, owner string, position int, v recognition.Candidate) error {
	raw, err := dynamicJSON(v.Value)
	if err != nil {
		return err
	}
	if _, err := q.Exec("INSERT INTO snapshot_candidates(k,root,owner,position,id,rule_id,role,value,strength,status,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.ID, v.RuleID, v.Role, raw, v.Strength, v.Status, v.Reason); err != nil {
		return err
	}
	for i, x := range v.Spans {
		if err := writeSnapshotSpans(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotCandidates struct {
	Key, Owner string
	Value      recognition.Candidate
}

func readSnapshotCandidates(q queryer, root string) ([]rowSnapshotCandidates, error) {
	childSnapshotSpans, err := readSnapshotSpans(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,id,rule_id,role,value,strength,status,reason FROM snapshot_candidates WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotCandidates{}
	for rows.Next() {
		var r rowSnapshotCandidates
		var raw string
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.ID, &r.Value.RuleID, &r.Value.Role, &raw, &r.Value.Strength, &r.Value.Status, &r.Value.Reason); err != nil {
			rows.Close()
			return nil, err
		}
		if err = decode(raw, &r.Value.Value); err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.Spans = []recognition.Span{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotSpans {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_spans")
		}
		out[i].Value.Spans = append(out[i].Value.Spans, c.Value)
	}
	return out, nil
}
func writeSnapshotSpans(q queryer, key, root, owner string, position int, v recognition.Span) error {
	if _, err := q.Exec("INSERT INTO snapshot_spans(k,root,owner,position,source_id,start,end) VALUES(?,?,?,?,?,?,?)", key, root, owner, position, v.SourceID, v.Start, v.End); err != nil {
		return err
	}
	return nil
}

type rowSnapshotSpans struct {
	Key, Owner string
	Value      recognition.Span
}

func readSnapshotSpans(q queryer, root string) ([]rowSnapshotSpans, error) {
	rows, err := q.Query("SELECT k,owner,source_id,start,end FROM snapshot_spans WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotSpans{}
	for rows.Next() {
		var r rowSnapshotSpans
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.SourceID, &r.Value.Start, &r.Value.End); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotFiles(q queryer, key, root, owner string, position int, v recognition.FileResult) error {
	if _, err := q.Exec("INSERT INTO snapshot_files(k,root,owner,position,file_id,path,tmdb_id,title,media_type,year,season) VALUES(?,?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.FileID, v.Path, v.Metadata.TMDBID, v.Metadata.Title, v.Metadata.MediaType, v.Metadata.Year, v.Metadata.Season); err != nil {
		return err
	}
	for i, x := range v.Metadata.Episodes {
		if err := writeSnapshotEpisodes(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	for i, target := range recognition.Targets() {
		if decision, ok := v.Decisions[target]; ok {
			if err := writeSnapshotDecisions(q, childKey(key, i), root, key, i, snapshotDecision{target, decision}); err != nil {
				return err
			}
		}
	}
	return nil
}

type rowSnapshotFiles struct {
	Key, Owner string
	Value      recognition.FileResult
}

func readSnapshotFiles(q queryer, root string) ([]rowSnapshotFiles, error) {
	childSnapshotEpisodes, err := readSnapshotEpisodes(q, root)
	if err != nil {
		return nil, err
	}
	childSnapshotDecisions, err := readSnapshotDecisions(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,file_id,path,tmdb_id,title,media_type,year,season FROM snapshot_files WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotFiles{}
	for rows.Next() {
		var r rowSnapshotFiles
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.FileID, &r.Value.Path, &r.Value.Metadata.TMDBID, &r.Value.Metadata.Title, &r.Value.Metadata.MediaType, &r.Value.Metadata.Year, &r.Value.Metadata.Season); err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.Metadata.Episodes = []int{}
		r.Value.Decisions = map[recognition.Target]recognition.Decision{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotEpisodes {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_episodes")
		}
		out[i].Value.Metadata.Episodes = append(out[i].Value.Metadata.Episodes, c.Value)
	}
	for _, c := range childSnapshotDecisions {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_decisions")
		}
		out[i].Value.Decisions[c.Value.Target] = c.Value.Decision
	}
	return out, nil
}
func writeSnapshotEpisodes(q queryer, key, root, owner string, position int, v int) error {
	if _, err := q.Exec("INSERT INTO snapshot_episodes(k,root,owner,position,value) VALUES(?,?,?,?,?)", key, root, owner, position, v); err != nil {
		return err
	}
	return nil
}

type rowSnapshotEpisodes struct {
	Key, Owner string
	Value      int
}

func readSnapshotEpisodes(q queryer, root string) ([]rowSnapshotEpisodes, error) {
	rows, err := q.Query("SELECT k,owner,value FROM snapshot_episodes WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotEpisodes{}
	for rows.Next() {
		var r rowSnapshotEpisodes
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotDecisions(q queryer, key, root, owner string, position int, v snapshotDecision) error {
	if _, err := q.Exec("INSERT INTO snapshot_decisions(k,root,owner,position,target,reason) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.Target, v.Decision.Reason); err != nil {
		return err
	}
	for i, x := range v.Decision.Candidates {
		if err := writeSnapshotReferences(q, childKey(key, i), root, key, i, x); err != nil {
			return err
		}
	}
	return nil
}

type rowSnapshotDecisions struct {
	Key, Owner string
	Value      snapshotDecision
}

func readSnapshotDecisions(q queryer, root string) ([]rowSnapshotDecisions, error) {
	childSnapshotReferences, err := readSnapshotReferences(q, root)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query("SELECT k,owner,target,reason FROM snapshot_decisions WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotDecisions{}
	for rows.Next() {
		var r rowSnapshotDecisions
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.Target, &r.Value.Decision.Reason); err != nil {
			rows.Close()
			return nil, err
		}
		r.Value.Decision.Candidates = []recognition.Reference{}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i := range out {
		index[out[i].Key] = i
	}
	for _, c := range childSnapshotReferences {
		i, ok := index[c.Owner]
		if !ok {
			return nil, fmt.Errorf("orphan snapshot_references")
		}
		out[i].Value.Decision.Candidates = append(out[i].Value.Decision.Candidates, c.Value)
	}
	return out, nil
}
func writeSnapshotReferences(q queryer, key, root, owner string, position int, v recognition.Reference) error {
	if _, err := q.Exec("INSERT INTO snapshot_references(k,root,owner,position,file_id,candidate_id) VALUES(?,?,?,?,?,?)", key, root, owner, position, v.FileID, v.CandidateID); err != nil {
		return err
	}
	return nil
}

type rowSnapshotReferences struct {
	Key, Owner string
	Value      recognition.Reference
}

func readSnapshotReferences(q queryer, root string) ([]rowSnapshotReferences, error) {
	rows, err := q.Query("SELECT k,owner,file_id,candidate_id FROM snapshot_references WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotReferences{}
	for rows.Next() {
		var r rowSnapshotReferences
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.FileID, &r.Value.CandidateID); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
func writeSnapshotDiagnostics(q queryer, key, root, owner string, position int, v recognition.Diagnostic) error {
	if _, err := q.Exec("INSERT INTO snapshot_diagnostics(k,root,owner,position,stage,file_id,rule_id,message,line,column) VALUES(?,?,?,?,?,?,?,?,?,?)", key, root, owner, position, v.Stage, v.FileID, v.RuleID, v.Message, v.Line, v.Column); err != nil {
		return err
	}
	return nil
}

type rowSnapshotDiagnostics struct {
	Key, Owner string
	Value      recognition.Diagnostic
}

func readSnapshotDiagnostics(q queryer, root string) ([]rowSnapshotDiagnostics, error) {
	rows, err := q.Query("SELECT k,owner,stage,file_id,rule_id,message,line,column FROM snapshot_diagnostics WHERE root=? ORDER BY owner,position", root)
	if err != nil {
		return nil, err
	}
	out := []rowSnapshotDiagnostics{}
	for rows.Next() {
		var r rowSnapshotDiagnostics
		if err = rows.Scan(&r.Key, &r.Owner, &r.Value.Stage, &r.Value.FileID, &r.Value.RuleID, &r.Value.Message, &r.Value.Line, &r.Value.Column); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
