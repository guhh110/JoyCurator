package fsops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyPreservesContentAndOffsets(t *testing.T) {
	for _, size := range []int{0, 1, copyChunkSize + 17} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dir := t.TempDir()
			body := make([]byte, size+7)
			for i := range body {
				body[i] = byte(i*31 + i/251)
			}
			path := filepath.Join(dir, "source")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			source, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			target, err := os.Create(filepath.Join(dir, "target"))
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			if _, err = source.Seek(7, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if _, err = target.WriteString("prefix"); err != nil {
				t.Fatal(err)
			}
			copied, err := Copy(context.Background(), target, source)
			if err != nil || copied != int64(size) {
				t.Fatal(copied, err)
			}
			if _, err = target.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(target)
			if err != nil || !bytes.Equal(got, append([]byte("prefix"), body[7:]...)) {
				t.Fatal("copy content or file offsets changed", err)
			}
		})
	}
}

type cancelCopyReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelCopyReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}

type shortCopyWriter struct{}

func (shortCopyWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestBufferedCopyCancellationAndShortWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var target bytes.Buffer
	n, err := copyBuffered(ctx, &target, cancelCopyReader{bytes.NewReader(make([]byte, 2<<20)), cancel})
	if !errors.Is(err, context.Canceled) || n != 1<<20 || target.Len() != 1<<20 {
		t.Fatal("copy did not stop between buffers", n, target.Len(), err)
	}
	n, err = copyBuffered(context.Background(), shortCopyWriter{}, bytes.NewReader([]byte("data")))
	if !errors.Is(err, io.ErrShortWrite) || n != 3 {
		t.Fatal("short write was lost", n, err)
	}
	// Cancellation must happen before touching either handle, including native copies.
	if n, err = Copy(ctx, nil, nil); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled copy touched files", n, err)
	}
}

type copyContextReader struct {
	ctx context.Context
	io.Reader
}

// Mirrors the previous transfer path, including its wrapper and 32 KiB buffer.
func (r copyContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func BenchmarkDurableCopy64MiB(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "source")
	if err := os.WriteFile(path, make([]byte, 64<<20), 0600); err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"previous", "optimized"} {
		b.Run(mode, func(b *testing.B) {
			source, err := os.Open(path)
			if err != nil {
				b.Fatal(err)
			}
			defer source.Close()
			target, err := os.Create(filepath.Join(dir, mode))
			if err != nil {
				b.Fatal(err)
			}
			defer target.Close()
			ctx := context.Background()
			b.SetBytes(64 << 20)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := source.Seek(0, io.SeekStart); err != nil {
					b.Fatal(err)
				}
				if _, err := target.Seek(0, io.SeekStart); err != nil {
					b.Fatal(err)
				}
				var copied int64
				if mode == "previous" {
					copied, err = io.Copy(target, copyContextReader{ctx, source})
				} else {
					copied, err = Copy(ctx, target, source)
				}
				if err != nil || copied != 64<<20 {
					b.Fatal(copied, err)
				}
				if err = target.Sync(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
