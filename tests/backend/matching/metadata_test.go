package matching

import (
	"encoding/json"
	"reflect"
	"testing"

	"joycurator/internal/recognition"
)

func TestLegacyWorkspaceDTOProducesMetadataWithoutChangingWireFields(t *testing.T) {
	raw := []byte(`{"path":"/source/Show.S00E01.mkv","season":0,"episodes":[1],"parsed":{"title":"Show","year":0,"type":"tv","season":0,"episodes":[1],"tmdbId":0,"releaseTag":""},"group":"manual","state":"review","selected":false,"removed":false,"edited":true}`)
	var item Item
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if item.Metadata.Title == nil || *item.Metadata.Title != "Show" || item.Metadata.Season == nil || *item.Metadata.Season != 0 || item.Metadata.Year != nil || item.Metadata.TMDBID != nil {
		t.Fatal(item.Metadata)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["metadata"]; ok {
		t.Fatal("legacy response gained an unexpected field")
	}
	if _, ok := wire["parsed"]; !ok {
		t.Fatal("legacy parsed field missing")
	}
	var reloaded Item
	if err = json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item.Metadata, reloaded.Metadata) {
		t.Fatal("metadata round-trip differs")
	}
}

func TestMatchingDoesNotRewritePathMetadata(t *testing.T) {
	engine := recognition.DefaultEngine()
	result, err := engine.Extract(t.Context(), recognition.Input{Path: "/source/The.Matrix.1999.tmdb-603.mkv", Root: "/source"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(result.Metadata)
	decision := DecideMatch(result.Metadata, nil)
	if decision.State != "unmatched" {
		t.Fatal(decision)
	}
	after, _ := json.Marshal(result.Metadata)
	if string(before) != string(after) {
		t.Fatal("matching rewrote original extraction")
	}
}
