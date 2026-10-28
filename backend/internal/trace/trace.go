// Package trace collects request-local diagnostic events. A nil recorder is a
// no-op, so production recognition uses exactly the same code without tracing.
package trace

import (
	"encoding/json"
	"time"
)

type Event struct {
	Stage     string          `json:"stage"`
	Rule      string          `json:"rule"`
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	ElapsedMS float64         `json:"elapsedMs"`
}

// Recorder is used by one sequential recognition request, never globally.
// Data is serialized immediately so later parser mutations cannot change logs.
type Recorder struct {
	started time.Time
	events  []Event
}

func New() *Recorder { return &Recorder{started: time.Now(), events: []Event{}} }
func (r *Recorder) Add(stage, rule, status, message string, data any) {
	if r == nil {
		return
	}
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	r.events = append(r.events, Event{Stage: stage, Rule: rule, Status: status, Message: message, Data: raw, ElapsedMS: float64(time.Since(r.started).Microseconds()) / 1000})
}
func (r *Recorder) Events() []Event {
	if r == nil {
		return []Event{}
	}
	return r.events
}
