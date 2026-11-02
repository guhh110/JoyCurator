package organizer

import (
	"context"
	"errors"
	"sync"
)

var ErrBusy = errors.New("仍有任务运行或排队，请等待完成")

type executionLane struct {
	pending []Run
	active  *Run
	cancel  context.CancelFunc
	claims  map[string]bool
	targets map[string]bool
	wait    string
}

type Coordinator struct {
	mu                 sync.Mutex
	lanes              map[string]*executionLane
	changed            chan struct{}
	recognitionCancels map[string]context.CancelFunc
	closed             bool
	revision           int64
	started            bool
	wg                 sync.WaitGroup
}

func NewCoordinator() *Coordinator {
	return &Coordinator{lanes: map[string]*executionLane{"organize": {}, "scrape": {}}, changed: make(chan struct{}), recognitionCancels: map[string]context.CancelFunc{}}
}
func (c *Coordinator) signalLocked() {
	c.revision++
	close(c.changed)
	c.changed = make(chan struct{})
}
func (c *Coordinator) Lock()   { c.mu.Lock() }
func (c *Coordinator) Unlock() { c.mu.Unlock() }
func (c *Coordinator) BusyLocked() bool {
	for _, lane := range c.lanes {
		if lane.active != nil || len(lane.pending) > 0 {
			return true
		}
	}
	return false
}
func (c *Coordinator) ClosingLocked() bool           { return c.closed }
func (c *Coordinator) RecognitionActiveLocked() bool { return len(c.recognitionCancels) > 0 }
func (c *Coordinator) TrackRecognitionLocked(id string, cancel context.CancelFunc) {
	c.recognitionCancels[id] = cancel
	c.signalLocked()
	c.wg.Add(1)
}
func (c *Coordinator) FinishRecognition(id string) {
	c.mu.Lock()
	delete(c.recognitionCancels, id)
	c.signalLocked()
	c.mu.Unlock()
	c.wg.Done()
}
func (c *Coordinator) CancelRecognition(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cancel := c.recognitionCancels[id]
	if cancel == nil {
		return errors.New("识别任务未在运行")
	}
	cancel()
	return nil
}
func (c *Coordinator) RecognitionActive(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recognitionCancels[id] != nil
}
func (c *Coordinator) IsClosing() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func (c *Coordinator) Busy() bool      { c.mu.Lock(); defer c.mu.Unlock(); return c.BusyLocked() }
func (c *Coordinator) Close() {
	c.mu.Lock()
	c.closed = true
	for _, lane := range c.lanes {
		if lane.cancel != nil {
			lane.cancel()
		}
	}
	for _, cancel := range c.recognitionCancels {
		cancel()
	}
	c.signalLocked()
	c.mu.Unlock()
	c.wg.Wait()
}
