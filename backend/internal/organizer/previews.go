package organizer

import (
	"sync"
	"time"
)

const previewTTL = 15 * time.Minute
const previewCapacity = 32

// PreviewCache owns both organize and scrape previews. Cached plans are immutable.
// Execution admission and consumption are serialized by Coordinator.
type PreviewCache struct {
	mu    sync.Mutex
	plans map[string]*Plan
}

func NewPreviewCache() *PreviewCache { return &PreviewCache{plans: make(map[string]*Plan)} }
func (c *PreviewCache) get(id string) *Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.plans[id]
	if p != nil && !time.Now().Before(p.Expires) {
		delete(c.plans, id)
		return nil
	}
	return p
}
func (c *PreviewCache) remove(id string) { c.mu.Lock(); defer c.mu.Unlock(); delete(c.plans, id) }
func (c *PreviewCache) Invalidate()      { c.mu.Lock(); defer c.mu.Unlock(); clear(c.plans) }
func (c *PreviewCache) put(p *Plan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	oldest := ""
	for id, other := range c.plans {
		if !now.Before(other.Expires) {
			delete(c.plans, id)
			continue
		}
		if oldest == "" || other.Expires.Before(c.plans[oldest].Expires) {
			oldest = id
		}
	}
	if len(c.plans) >= previewCapacity {
		delete(c.plans, oldest)
	}
	c.plans[p.ID] = p
}
