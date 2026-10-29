package planning

import (
	"joycurator/internal/model"
	"time"
)

type Row struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Season    int      `json:"season"`
	Episodes  []int    `json:"episodes"`
	Artifacts []string `json:"artifacts"`
	Issues    []string `json:"issues"`
}

type Plan struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Mode          string       `json:"mode"`
	Rows          []Row        `json:"rows"`
	Required      int64        `json:"required"`
	Issues        []string     `json:"issues"`
	Version       int          `json:"-"`
	Expires       time.Time    `json:"-"`
	Ops           []model.Work `json:"-"`
	Groups        []*Plan      `json:"groups,omitempty"`
	Type          string       `json:"type,omitempty"`
	TMDBID        int          `json:"tmdbId,omitempty"`
	RecognitionID string       `json:"-"`
	Revision      int          `json:"-"`
}

type PreviewRequest struct {
	Type   string       `json:"type"`
	TMDBID int          `json:"tmdbId"`
	Mode   string       `json:"mode"`
	Files  []model.File `json:"files"`
}

type BatchPreviewRequest struct {
	RecognitionID string `json:"recognitionId"`
	Revision      int    `json:"revision"`
	Mode          string `json:"mode"`
}
