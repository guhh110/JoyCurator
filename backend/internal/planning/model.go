package planning

import (
	"time"

	"joycurator/internal/model"
)

type Row struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Season    int      `json:"season"`
	Episodes  []int    `json:"episodes"`
	Artifacts []string `json:"artifacts"`
	Issues    []string `json:"issues"`
	Action    string   `json:"action"`
}

type Plan struct {
	ID              string               `json:"id"`
	Title           string               `json:"title"`
	Mode            string               `json:"mode"`
	DuplicatePolicy string               `json:"duplicatePolicy"`
	Rows            []Row                `json:"rows"`
	Required        int64                `json:"required"`
	Issues          []string             `json:"issues"`
	Version         int                  `json:"-"`
	Expires         time.Time            `json:"-"`
	Ops             []model.Work         `json:"-"`
	Mappings        []model.MediaMapping `json:"-"`
	Purpose         string               `json:"-"`
	Groups          []*Plan              `json:"groups,omitempty"`
	Type            string               `json:"type,omitempty"`
	TMDBID          int                  `json:"tmdbId,omitempty"`
	RecognitionID   string               `json:"-"`
	Revision        int                  `json:"-"`
}

type PreviewRequest struct {
	RecognitionID   string   `json:"recognitionId"`
	Revision        int      `json:"revision"`
	Paths           []string `json:"paths"`
	Mode            string   `json:"mode"`
	DuplicatePolicy string   `json:"duplicatePolicy"`
}

type BatchPreviewRequest struct {
	RecognitionID   string `json:"recognitionId"`
	Revision        int    `json:"revision"`
	Mode            string `json:"mode"`
	DuplicatePolicy string `json:"duplicatePolicy"`
}
