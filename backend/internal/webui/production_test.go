//go:build production

package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// Run from a directory with no frontend files: every referenced bundle must be embedded.
func TestEmbeddedFrontend(t *testing.T) {
	t.Chdir(t.TempDir())
	files := Files()
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `id="app"`) {
		t.Fatal("missing Vue mount point")
	}
	assets := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`).FindAllStringSubmatch(string(index), -1)
	if len(assets) == 0 {
		t.Fatal("no bundled assets")
	}
	handler := http.FileServer(http.FS(files))
	for _, path := range append([][]string{{"", "/"}}, assets...) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path[1], nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("%s: status %d", path[1], response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/assets/missing.js", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing asset: %d", response.Code)
	}
}
