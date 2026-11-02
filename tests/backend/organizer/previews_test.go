package organizer

import (
	"fmt"
	"testing"
	"time"
)

func TestPreviewCacheSharesCapacityAndEvictsOnlyOldest(t *testing.T) {
	cache := NewPreviewCache()
	now := time.Now()
	for i := 0; i < previewCapacity+1; i++ {
		purpose := "organize"
		if i%2 == 1 {
			purpose = "scrape"
		}
		cache.put(&Plan{ID: fmt.Sprint(i), Purpose: purpose, Expires: now.Add(previewTTL + time.Duration(i)*time.Second)})
	}
	if cache.get("0") != nil {
		t.Fatal("oldest preview retained")
	}
	for i := 1; i <= previewCapacity; i++ {
		if cache.get(fmt.Sprint(i)) == nil {
			t.Fatal("unrelated preview evicted", i)
		}
	}
	cache.Invalidate()
	if cache.get("1") != nil || cache.get("2") != nil {
		t.Fatal("invalidation missed a purpose")
	}
}
func TestPreviewCacheExpiresAndConsumes(t *testing.T) {
	cache := NewPreviewCache()
	cache.put(&Plan{ID: "expired", Expires: time.Now().Add(-time.Second)})
	if cache.get("expired") != nil {
		t.Fatal("expired preview returned")
	}
	cache.put(&Plan{ID: "current", Expires: time.Now().Add(previewTTL)})
	cache.remove("current")
	if cache.get("current") != nil {
		t.Fatal("consumed preview returned")
	}
}
func TestBatchConsumesPreviewAndRejectsOtherPurpose(t *testing.T) {
	s, _, plans := executionFixture(t, nil)
	if _, err := s.ExecuteBatch("manual"); err != nil {
		t.Fatal(err)
	}
	awaitIdle(t, s)
	if _, err := s.ExecuteBatch("manual"); err == nil {
		t.Fatal("duplicate confirmation accepted")
	}
	p := plans.Get("batch")
	p.Purpose = "scrape"
	if _, err := s.ExecuteBatch("batch"); err == nil {
		t.Fatal("scrape preview accepted as organize")
	}
}
