package matching

import (
	"slices"
	"time"
)

// EpisodeBasis records how the user obtained the episode numbers, before offset.
type EpisodeBasis struct {
	Kind        string `json:"kind"`
	TraceID     string `json:"traceId,omitempty"`
	CandidateID string `json:"candidateId,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
	Index       int    `json:"index"`
	Pattern     string `json:"pattern,omitempty"`
	Offset      int    `json:"offset"`
}
type Confirmation struct {
	Source   string       `json:"source"`
	At       string       `json:"at"`
	Identity string       `json:"identity"`
	Size     int64        `json:"size"`
	MTime    int64        `json:"mtime,string"`
	Type     string       `json:"type"`
	TMDBID   int          `json:"tmdbId"`
	Season   int          `json:"season"`
	Episodes []int        `json:"episodes"`
	Basis    EpisodeBasis `json:"basis"`
}
type ConfirmItem struct {
	Path     string       `json:"path"`
	Type     string       `json:"type"`
	TMDBID   int          `json:"tmdbId"`
	Season   int          `json:"season"`
	Episodes []int        `json:"episodes"`
	Basis    EpisodeBasis `json:"basis"`
}
type ConfirmRequest struct {
	Revision int           `json:"revision"`
	Items    []ConfirmItem `json:"items"`
}

func (i *Item) Confirm(source string, basis EpisodeBasis) {
	i.Confirmation = &Confirmation{Source: source, At: time.Now().UTC().Format(time.RFC3339Nano), Identity: i.Identity, Size: i.Size, MTime: i.MTime, Type: i.Entry.Type, TMDBID: i.Entry.TMDBID, Season: i.Season, Episodes: append([]int{}, i.Episodes...), Basis: basis}
}
func (i Item) Confirmed() bool {
	c := i.Confirmation
	return c != nil && i.Entry != nil && c.Identity != "" && c.Identity == i.Identity && c.Size == i.Size && c.MTime == i.MTime && c.Type == i.Entry.Type && c.TMDBID == i.Entry.TMDBID && c.Season == i.Season && slices.Equal(c.Episodes, i.Episodes)
}
