package sqlite

import (
	"fmt"
	"joycurator/internal/organizer"
	"testing"
)

func TestRunFiltersApplyBeforePagingAndReadOnlySummaries(t *testing.T) {
	s := openStore(t)
	for i := 0; i < 26; i++ {
		r := organizer.Run{ID: fmt.Sprint(i), Title: "Show 100%_特别篇", Purpose: "organize", State: "failed", Ops: []organizer.Work{{Operation: organizer.Operation{Content: []byte("large content")}}}}
		if i == 24 {
			r.State = "completed"
		}
		if i == 25 {
			r.Purpose = "scrape"
		}
		if err := s.SaveRun(r); err != nil {
			t.Fatal(err)
		}
	}
	f := organizer.RunFilter{Search: " show 100%_特 ", State: "failed"}
	p, err := s.RunsByPurpose(2, "organize", f)
	if err != nil || p.Total != 24 || len(p.Items) != 4 || p.Items[0].ID != "3" {
		t.Fatal(p, err)
	}
	for _, r := range p.Items {
		if r.Ops != nil || r.Mappings != nil {
			t.Fatal("loaded full run")
		}
	}
	p, err = s.RunsByPurpose(1, "scrape", f)
	if err != nil || p.Total != 1 || p.Items[0].ID != "25" {
		t.Fatal(p, err)
	}
	if _, err := s.RunsByPurpose(1, "organize", organizer.RunFilter{State: "failed' OR 1=1--"}); err == nil {
		t.Fatal("invalid state accepted")
	}
	p, err = s.RunsByPurpose(1, "organize", organizer.RunFilter{Search: "' OR 1=1--"})
	if err != nil || p.Total != 0 {
		t.Fatal(p, err)
	}
}
