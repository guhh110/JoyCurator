package matching

import (
	"encoding/json"
	"reflect"
	"testing"

	"joycurator/internal/recognition"
)

func TestMetadataIsTheOnlyWorkspaceRepresentation(t *testing.T) {
	item := Item{Metadata: recognition.Metadata{Title: ptr("Show"), MediaType: ptr("tv"), Season: ptr(0), Episodes: []int{1}}, State: "review"}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, ok := wire["parsed"]; ok {
		t.Fatal("obsolete field returned")
	}
	if _, ok := wire["metadata"]; !ok {
		t.Fatal("metadata missing")
	}
	var reloaded Item
	if err = json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(item.Metadata, reloaded.Metadata) {
		t.Fatal(reloaded.Metadata)
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
