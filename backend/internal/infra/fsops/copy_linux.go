package fsops

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func copyFileRange(ctx context.Context, target, source *os.File) (int, bool, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, true, err
		}
		n, err := unix.CopyFileRange(int(source.Fd()), nil, int(target.Fd()), nil, copyChunkSize, 0)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		// Cross-device copies, older kernels, unsupported filesystems and
		// sandboxed syscalls can still use ordinary reads and writes.
		if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EPERM) {
			return 0, false, nil
		}
		return max(0, n), true, err
	}
}
