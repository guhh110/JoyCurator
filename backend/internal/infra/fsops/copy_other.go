//go:build !linux

package fsops

import (
	"context"
	"os"
)

func copyFileRange(ctx context.Context, target, source *os.File) (int, bool, error) {
	return 0, false, nil
}
