//go:build linux || darwin

package fsops

import (
	"os"

	"golang.org/x/sys/unix"
)

func (f *FS) Link(source, target string) error {
	s, src, err := f.parent(source)
	if err != nil {
		return err
	}
	defer s.Close()
	d, dst, err := f.parent(target)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = unix.Linkat(int(s.Fd()), src, int(d.Fd()), dst, 0); err != nil {
		return err
	}
	return d.Sync()
}

func (f *FS) Remove(name string) error {
	p, base, err := f.parent(name)
	if err != nil {
		return err
	}
	defer p.Close()
	if err = unix.Unlinkat(int(p.Fd()), base, 0); err != nil {
		return err
	}
	return p.Sync()
}

func checkTargetDir(dir *os.File) (int64, uint64, error) {
	if err := unix.Faccessat(int(dir.Fd()), ".", unix.W_OK, 0); err != nil {
		return 0, 0, err
	}
	var disk unix.Statfs_t
	if err := unix.Fstatfs(int(dir.Fd()), &disk); err != nil {
		return 0, 0, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &stat); err != nil {
		return 0, 0, err
	}
	return int64(disk.Bavail) * int64(disk.Bsize), uint64(stat.Dev), nil
}
