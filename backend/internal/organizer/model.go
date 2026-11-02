package organizer

import (
	"joycurator/internal/model"
)

type Run struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	Mode          string               `json:"mode"`
	State         string               `json:"state"`
	Created       string               `json:"created"`
	Error         string               `json:"error"`
	Completed     int                  `json:"completed"`
	Total         int                  `json:"total"`
	Undo          bool                 `json:"undo"`
	Ops           []Work               `json:"ops,omitempty"`
	BatchID       string               `json:"batchId,omitempty"`
	Version       int                  `json:"version,omitempty"`
	RecognitionID string               `json:"recognitionId,omitempty"`
	Purpose       string               `json:"purpose"`
	Mappings      []model.MediaMapping `json:"mappings,omitempty"`
}

type ScrapePlan struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	TMDBID    int      `json:"tmdbId"`
	Targets   []string `json:"targets"`
	Artifacts []string `json:"artifacts"`
	Issues    []string `json:"issues"`
	Required  int64    `json:"required"`
}

type Batch struct {
	ID     string `json:"id"`
	Runs   []Run  `json:"runs"`
	Active bool   `json:"active"`
}

// RunPage contains summaries; file operations are loaded through Run(id).
type RunPage struct {
	Items []Run `json:"items"`
	Total int   `json:"total"`
	Page  int   `json:"page"`
}

// Operation is one file action the organizer knows how to run and reverse.
type Operation struct {
	ID               int64      `json:"id"`
	Kind             string     `json:"kind"`
	Source           string     `json:"source"`
	Target           string     `json:"target"`
	Mode             string     `json:"mode"`
	Size             int64      `json:"size"`
	MTime            int64      `json:"mtime"`
	Content          []byte     `json:"content,omitempty"`
	URL              string     `json:"url,omitempty"`
	ConvertJPEG      bool       `json:"convertJpeg,omitempty"`
	Status           string     `json:"status"`
	Error            string     `json:"error"`
	Hash             string     `json:"hash"`
	Temp             string     `json:"temp"`
	ReplaceHold      string     `json:"replaceHold,omitempty"`
	RemoveHold       string     `json:"removeHold,omitempty"`
	SourceRemoveHold string     `json:"sourceRemoveHold,omitempty"`
	Identity         string     `json:"identity"`
	SourceIdentity   string     `json:"sourceIdentity"`
	Replace          bool       `json:"replace,omitempty"`
	ReplaceIdentity  string     `json:"replaceIdentity,omitempty"`
	ReplaceSize      int64      `json:"replaceSize,omitempty"`
	ReplaceMTime     int64      `json:"replaceMtime,omitempty"`
	Undo             *Operation `json:"undo,omitempty"`
}

type Work struct {
	Operation
	Owner  string   `json:"owner"`
	Owners []string `json:"owners,omitempty"`
	Reused bool     `json:"reused,omitempty"`
}
