//go:build !production

// Package webui supplies the frontend embedded in release builds.
package webui

import "io/fs"

const Embedded = false

func Files() fs.FS { return nil }
