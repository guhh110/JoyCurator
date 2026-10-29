package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentScanAndWorkspaceExposeMetadata(t *testing.T) {
	a, root := fixture(t)
	file := mediaFile(t, a, root, "Show.S00E01.mkv")
	files, err := a.dependencies.Files.Scan(context.Background(), []string{file.Path})
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	raw, err := json.Marshal(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value["metadata"]; !ok {
		t.Fatal("scan must return metadata")
	}
	if _, ok := value["parsed"]; ok {
		t.Fatal("obsolete field returned")
	}
	if files[0].Metadata.Season == nil || *files[0].Metadata.Season != 0 {
		t.Fatal(files[0].Metadata)
	}
	if err = a.dependencies.Auth.Setup(context.Background(), "admin", "fixture-password-2026"); err != nil {
		t.Fatal(err)
	}
	token, _, err := a.dependencies.Auth.Login(context.Background(), "admin", "fixture-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	call := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", path, strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: "joycurator_session", Value: token})
		response := httptest.NewRecorder()
		a.Handler(nil).ServeHTTP(response, request)
		return response
	}
	if response := call("/api/extraction/test", `{"path":"/absent/Show.S00E01.mkv"}`); response.Code != 200 || !strings.Contains(response.Body.String(), `"metadata"`) {
		t.Fatal(response.Code, response.Body.String())
	}
	if response := call("/api/recognitions/debug", `{"path":"/absent/file.mkv","mode":"parse"}`); response.Code != 404 {
		t.Fatal("removed endpoint still available", response.Code)
	}
	if response := call("/api/extraction/test", `{"path":"/absent/file.mkv","rules":[{"id":"one","name":"one","target":"title","source":"filename","order":1,"enabled":true,"kind":"script","config":{"handler":"title","code":"def extract(ctx):\n    return None"},"overwrite":false,"examples":[]}]}`); response.Code != 400 {
		t.Fatal("unknown rule property accepted", response.Code)
	}
}
