package organizer

import (
	"context"
	"errors"
	"sync"
)

var ErrBusy = errors.New("已有任务正在运行，请等待完成")

type Coordinator struct {
	mu                 sync.Mutex
	running            string
	cancel             context.CancelFunc
	batch              string
	batchCancel        context.CancelFunc
	recognitionCancels map[string]context.CancelFunc
	closed             bool
	wg                 sync.WaitGroup
}

func NewCoordinator() *Coordinator {
	return &Coordinator{recognitionCancels: map[string]context.CancelFunc{}}
}

// Lock serializes admission, configuration commits and workspace commits.
// Never wait for a worker while holding it.
func (c *Coordinator) Lock()                         { c.mu.Lock() }
func (c *Coordinator) Unlock()                       { c.mu.Unlock() }
func (c *Coordinator) BusyLocked() bool              { return c.running != "" || c.batch != "" }
func (c *Coordinator) ClosingLocked() bool           { return c.closed }
func (c *Coordinator) RecognitionActiveLocked() bool { return len(c.recognitionCancels) > 0 }
func (c *Coordinator) TrackRecognitionLocked(id string, cancel context.CancelFunc) {
	c.recognitionCancels[id] = cancel
	c.wg.Add(1)
}
func (c *Coordinator) FinishRecognition(id string) {
	c.mu.Lock()
	delete(c.recognitionCancels, id)
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
	if c.cancel != nil {
		c.cancel()
	}
	if c.batchCancel != nil {
		c.batchCancel()
	}
	for _, cancel := range c.recognitionCancels {
		cancel()
	}
	c.mu.Unlock()
	c.wg.Wait()
}
