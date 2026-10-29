package organizer_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"joycurator/internal/organizer"
)

func TestRecognitionCancellationAndShutdownWait(t *testing.T) {
	// Recognition lifecycle is shared with execution rather than a second worker registry.
	c := organizer.NewCoordinator()
	ctx, cancel := context.WithCancel(context.Background())
	c.Lock()
	c.TrackRecognitionLocked("workspace", cancel)
	c.Unlock()
	finished, closing := make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() { <-ctx.Done(); <-finished; c.FinishRecognition("workspace") }()
	go func() { c.Close(); once.Do(func() { close(closing) }) }()
	<-ctx.Done()
	if !c.IsClosing() {
		t.Fatal("missing closing state")
	}
	select {
	case <-closing:
		t.Fatal("shutdown ignored recognition worker")
	default:
	}
	close(finished)
	select {
	case <-closing:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not wait correctly")
	}
	if c.RecognitionActive("workspace") {
		t.Fatal("finished recognition retained")
	}
}
