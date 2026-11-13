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
