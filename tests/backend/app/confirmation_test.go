package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"joycurator/internal/matching"
	"joycurator/internal/organizer"
	"joycurator/internal/recognition"
)

func unconfirmedTarget(t *testing.T, a *App, root string) matching.Workspace {
	t.Helper()
	w, err := a.dependencies.Workspace.Start([]string{root}, "", "scrape")
	if err != nil {
		t.Fatal(err)
	}
	w = waitRecognition(t, a, w.ID)
	if w.State != "completed" {
		t.Fatal(w)
	}
	return w
}
func digitConfirmation(t *testing.T, a *App, w matching.Workspace) matching.ConfirmRequest {
	t.Helper()
	paths := []string{}
	for _, it := range w.Items {
		paths = append(paths, it.Path)
	}
	views, err := a.dependencies.Workspace.Candidates(w.ID, CandidateRequest{Revision: w.Revision, Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	in := matching.ConfirmRequest{Revision: w.Revision}
	for _, v := range views {
		source := v.Document.Sources[0]
		n := 1
		if strings.HasSuffix(v.Path, "/2.mp4") {
			n = 2
		}
		in.Items = append(in.Items, matching.ConfirmItem{Path: v.Path, Type: "tv", TMDBID: 2, Season: 1, Episodes: []int{n}, Basis: matching.EpisodeBasis{Kind: "digits", TraceID: v.TraceID, SourceID: source.ID, Index: 0, Pattern: digitStructure(source.Text)}})
	}
	return in
}
func TestUserConfirmsNumericFilesAndPreservesEvidence(t *testing.T) {
	a, root, _ := directoryFixture(t)
	dir := "🏠主页/quark2/来自：分享/【X】《亻山》NI（4K）/1-159"
	videos := []string{targetVideo(t, root, dir+"/1.mp4"), targetVideo(t, root, dir+"/2.mp4")}
	before := map[string]string{}
	for _, p := range videos {
		b, _ := os.ReadFile(p)
		before[p] = string(b)
	}
	w := unconfirmedTarget(t, a, filepath.Dir(videos[0]))
	for _, it := range w.Items {
		if it.Entry != nil || it.Selected || it.Confirmation != nil || len(it.Metadata.Episodes) != 0 {
			t.Fatal("scan made a final decision", it)
		}
	}
	in := digitConfirmation(t, a, w)
	// A rejected batch must not confirm its first, valid row.
	invalid := in
	invalid.Items = append([]matching.ConfirmItem{}, in.Items...)
	invalid.Items[1].Episodes = []int{999}
	if _, err := a.dependencies.Workspace.Confirm(context.Background(), w.ID, invalid); err == nil {
		t.Fatal("mismatched evidence accepted")
	}
	untouched, _ := a.store.Workspace(w.ID)
	for _, it := range untouched.Items {
		if it.Confirmation != nil {
			t.Fatal("partial confirmation")
		}
	}
	confirmed, err := a.dependencies.Workspace.Confirm(context.Background(), w.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Revision != w.Revision+1 {
		t.Fatal(confirmed.Revision)
	}
	stored, err := a.store.Workspace(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, it := range stored.Items {
		if !it.Confirmed() || !it.Selected || it.State != "ready" || it.Entry.TMDBID != 2 || !reflect.DeepEqual(it.Episodes, []int{i + 1}) {
			t.Fatal(it)
		}
		if !reflect.DeepEqual(it.Confirmation, confirmed.Items[i].Confirmation) {
			t.Fatal("confirmation snapshot lost")
		}
		views, err := a.dependencies.Workspace.Candidates(w.ID, CandidateRequest{Revision: stored.Revision, Paths: []string{it.Path}})
		if err != nil || views[0].TraceID != it.Confirmation.Basis.TraceID {
			t.Fatal(views, err)
		}
		if err = a.dependencies.Recognition.Delete(it.TraceID); err == nil {
			t.Fatal("referenced evidence removed")
		}
		bytes, _ := os.ReadFile(it.Path)
		if string(bytes) != before[it.Path] {
			t.Fatal("confirmation changed video")
		}
	}
	if _, err = a.dependencies.Workspace.Confirm(context.Background(), w.ID, in); err == nil {
		t.Fatal("stale confirmation accepted")
	}
	plan, err := a.dependencies.Organizer.ScrapeBatchPreview(context.Background(), organizer.ScrapeBatchPreviewRequest{RecognitionID: w.ID, Revision: stored.Revision, Content: "nfo", OnlyMissing: true})
	if err != nil || len(plan.Issues) > 0 {
		t.Fatal(plan, err)
	}
	// Even a forged ready flag cannot replace the server's confirmation record.
	stored.Items[0].Confirmation = nil
	stored.Items[0].State = "ready"
	stored.Items[0].Selected = true
	stored.Items[1].Selected = false
	if err = a.store.SaveWorkspace(&stored); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Organizer.ScrapeBatchPreview(context.Background(), organizer.ScrapeBatchPreviewRequest{RecognitionID: w.ID, Revision: stored.Revision, Content: "nfo"}); err == nil {
		t.Fatal("unconfirmed item previewed")
	}
	if _, err = a.dependencies.Organizer.ScrapeExecuteBatch(plan.ID); err == nil {
		t.Fatal("old preview bypassed confirmation")
	}
}
func TestExplicitFormatsAndExactMatchesRemainSuggestions(t *testing.T) {
	for _, name := range []string{"Show/Season 01/Show.S01E01.mp4", "FilmA/FilmA.2024.mp4", "FilmA/FilmA.tmdb-1.mp4"} {
		t.Run(name, func(t *testing.T) {
			a, root, _ := directoryFixture(t)
			path := targetVideo(t, root, name)
			w := unconfirmedTarget(t, a, filepath.Dir(path))
			it := w.Items[0]
			if it.Entry != nil || it.Confirmation != nil || it.Selected || it.State == "ready" {
				t.Fatal(it)
			}
			if len(it.Candidates) == 0 {
				t.Fatal("catalog suggestions lost")
			}
		})
	}
}
func TestConfirmationRefreshAndFileChange(t *testing.T) {
	a, root, _ := directoryFixture(t)
	path := targetVideo(t, root, "数字/1.mp4")
	w := unconfirmedTarget(t, a, filepath.Dir(path))
	in := digitConfirmation(t, a, w)
	w, err := a.dependencies.Workspace.Confirm(context.Background(), w.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	saved := w.Items[0].Confirmation
	if _, err = a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{path}); err != nil {
		t.Fatal(err)
	}
	w = waitRecognition(t, a, w.ID)
	if !w.Items[0].Confirmed() || !reflect.DeepEqual(saved, w.Items[0].Confirmation) {
		t.Fatal("refresh discarded user confirmation", w.Items[0])
	}
	if err = os.WriteFile(path, []byte("changed video"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = a.dependencies.Workspace.Confirm(context.Background(), w.ID, digitConfirmation(t, a, w)); err == nil {
		t.Fatal("changed file confirmed against old snapshot")
	}
	if _, err = a.dependencies.Workspace.Reidentify(w.ID, w.Revision, []string{path}); err != nil {
		t.Fatal(err)
	}
	w = waitRecognition(t, a, w.ID)
	if w.Items[0].Confirmation != nil || w.Items[0].Selected || w.Items[0].State != "review" || w.Items[0].Entry.TMDBID != 2 || !reflect.DeepEqual(w.Items[0].Episodes, []int{1}) {
		t.Fatal("changed file did not retain unconfirmed draft", w.Items[0])
	}
}
func TestEpisodeEvidenceChecksAmbiguityAndRanges(t *testing.T) {
	paths := []string{"/source/作品第一季03.mp4", "/source/作品第一季04.mp4", "/source/Show.S01E01-E02.mkv", "/source/Show.第十一集.mkv"}
	batch, err := recognition.DefaultEngine().ExtractBatch(context.Background(), recognition.BatchInput{SourceBoundary: "/source", ContextRoot: "/source", Paths: paths, Targets: paths}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, doc := range batch.Documents {
		v := CandidateView{Path: doc.Path, TraceID: "snapshot", Document: doc, Suggestion: batch.Files[i]}
		for _, c := range doc.Candidates {
			if c.Role != recognition.Episodes || c.Status != "valid" {
				continue
			}
			data := matching.EpisodeBasis{Kind: "candidate", TraceID: "snapshot", CandidateID: c.ID}
			var values []int
			switch {
			case strings.Contains(doc.Path, "第一季03"):
				values = []int{3}
			case strings.Contains(doc.Path, "第一季04"):
				values = []int{4}
			case strings.Contains(doc.Path, "S01E01"):
				values = []int{1, 2}
			default:
				values = []int{11}
			}

			if err = checkedBasis(v, data, values); err != nil {
				t.Fatal(doc.Path, c, err)
			}
		}
	}
	v := CandidateView{TraceID: "trace", Document: recognition.Document{Sources: []recognition.TextSource{{ID: "name", Text: "Show.S01E03"}}}}
	b := matching.EpisodeBasis{Kind: "digits", TraceID: "trace", SourceID: "name", Index: 1, Pattern: digitStructure("Show.S01E03"), Offset: 2}
	if err = checkedBasis(v, b, []int{5}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*matching.EpisodeBasis){func(b *matching.EpisodeBasis) { b.Pattern = "Other#" }, func(b *matching.EpisodeBasis) { b.Index = 2 }, func(b *matching.EpisodeBasis) { b.TraceID = "stale" }, func(b *matching.EpisodeBasis) { b.SourceID = "dir-99" }} {
		invalid := b
		change(&invalid)
		if checkedBasis(v, invalid, []int{5}) == nil {
			t.Fatal(invalid)
		}
	}
}
