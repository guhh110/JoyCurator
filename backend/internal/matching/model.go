package matching

import (
	"encoding/json"

	"joycurator/internal/model"
	"joycurator/internal/recognition"
)

type Item struct {
	model.File
	Metadata   recognition.Metadata `json:"-"`
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

// The legacy parsed field stays on the wire and in SQLite, not in matching logic.
func (i Item) MarshalJSON() ([]byte, error) {
	type fields Item
	return json.Marshal(struct {
		fields
		Parsed model.Parsed `json:"parsed"`
	}{fields(i), recognition.ParsedMetadata(i.Metadata)})
}
func (i *Item) UnmarshalJSON(data []byte) error {
	type fields Item
	var wire struct {
		fields
		Parsed model.Parsed `json:"parsed"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*i = Item(wire.fields)
	i.Metadata = recognition.MetadataFromParsed(wire.Parsed)
	return nil
}
