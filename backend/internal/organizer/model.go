package organizer

import (
	"joycurator/internal/model"
)

type Run struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Mode          string       `json:"mode"`
	State         string       `json:"state"`
	Created       string       `json:"created"`
	Error         string       `json:"error"`
	Completed     int          `json:"completed"`
	Total         int          `json:"total"`
	Undo          bool         `json:"undo"`
	Ops           []model.Work `json:"ops,omitempty"`
	BatchID       string       `json:"batchId,omitempty"`
	Version       int          `json:"version,omitempty"`
	RecognitionID string       `json:"recognitionId,omitempty"`
}

type Batch struct {
	ID     string `json:"id"`
	Runs   []Run  `json:"runs"`
	Active bool   `json:"active"`
}
