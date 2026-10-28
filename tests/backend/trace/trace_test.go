package trace

import (
	"strings"
	"testing"
)

func TestRecorderSnapshotsDataAndSupportsNil(t *testing.T) {
	var disabled *Recorder
	disabled.Add("parse", "rule", "info", "message", nil)
	if len(disabled.Events()) != 0 {
		t.Fatal("disabled trace recorded events")
	}
	recorder := New()
	values := []int{1}
	data := map[string]any{"episodes": values}
	recorder.Add("parse", "episodes", "matched", "parsed", data)
	values[0] = 9
	data["later"] = true
	event := recorder.Events()[0]
	if !strings.Contains(string(event.Data), `"episodes":[1]`) || strings.Contains(string(event.Data), "later") {
		t.Fatal(string(event.Data))
	}
}
