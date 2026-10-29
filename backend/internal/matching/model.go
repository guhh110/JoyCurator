package matching

import (
	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

type Item struct {
	model.File
	Metadata   recognition.Metadata `json:"metadata"`
	Group      string               `json:"group"`
	Entry      *model.Entry         `json:"entry"`
	Candidates []model.Entry        `json:"candidates"`
	State      string               `json:"state"`
	Reason     string               `json:"reason"`
	Selected   bool                 `json:"selected"`
	Removed    bool                 `json:"removed"`
	Edited     bool                 `json:"edited,omitempty"`
	TraceID    string               `json:"traceId,omitempty"`
}
type Workspace struct {
	ID        string   `json:"id"`
	Revision  int      `json:"revision"`
	State     string   `json:"state"`
	Paths     []string `json:"paths"`
	Items     []Item   `json:"items"`
	Completed int      `json:"completed"`
	Total     int      `json:"total"`
	Error     string   `json:"error"`
	Created   string   `json:"created"`
}
type Edit struct {
	Path     string `json:"path"`
	Group    string `json:"group"`
	Type     string `json:"type"`
	TMDBID   int    `json:"tmdbId"`
	Season   int    `json:"season"`
	Episodes []int  `json:"episodes"`
	Selected bool   `json:"selected"`
	Removed  bool   `json:"removed"`
}

type Update struct {
	Revision int    `json:"revision"`
	Items    []Edit `json:"items"`
}
