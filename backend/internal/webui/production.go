//go:build production

package webui

import (
	"embed"
	"io/fs"
)

const Embedded = true

//go:embed dist/index.html dist/assets
var assets embed.FS

func Files() fs.FS {
	files, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
