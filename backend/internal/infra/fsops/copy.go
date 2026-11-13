package fsops

import (
	"context"
	"io"
	"os"
	"sync"
)

const copyChunkSize = 8 << 20

var copyBuffers = sync.Pool{New: func() any { return make([]byte, 1<<20) }}

// Copy copies between already authorized file handles. Linux can transfer data
// in the filesystem; other filesystems use a large reusable buffer. Checking
// cancellation between bounded transfers keeps both paths interruptible.
// The caller remains responsible for syncing and publishing the destination.
func Copy(ctx context.Context, target, source *os.File) (int64, error) {
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		n, handled, err := copyFileRange(ctx, target, source)
		copied += int64(n)
		if err != nil {
			return copied, err
		}
		if !handled {
			n, err := copyBuffered(ctx, target, source)
			return copied + n, err
		}
		if n == 0 {
			return copied, ctx.Err()
		}
	}
}

func copyBuffered(ctx context.Context, target io.Writer, source io.Reader) (int64, error) {
	buffer := copyBuffers.Get().([]byte)
	defer copyBuffers.Put(buffer)
	var copied int64
	for {
		if err := ctx.Err(); err != nil {
			return copied, err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written, err := target.Write(buffer[:n])
			copied += int64(written)
			if err != nil {
				return copied, err
			}
			if written != n {
				return copied, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return copied, ctx.Err()
			}
			return copied, readErr
		}
	}
}
