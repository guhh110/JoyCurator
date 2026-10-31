//go:build linux || darwin

package fsops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func fmtIdentity(stat *unix.Stat_t) string { return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino) }

func randomSuffix() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

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

// RemoveOwned isolates the path before checking its inode; it never unlinks a
// replacement that appeared after the caller's content verification.
func (f *FS) RemoveOwned(name, identity string) error {
	p, base, err := f.parent(name)
	if err != nil {
		return err
	}
	defer p.Close()
	if identity == "" {
		return errors.New("目标文件归属未知")
	}
	var before unix.Stat_t
	if err = unix.Fstatat(int(p.Fd()), base, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&before) != identity {
		return errors.New("目标文件已被替换")
	}
	isolated := ".joycurator-remove-" + randomSuffix() + "-" + base
	if err = renameExclusive(int(p.Fd()), base, int(p.Fd()), isolated); err != nil {
		return err
	}
	var captured unix.Stat_t
	if err = unix.Fstatat(int(p.Fd()), isolated, &captured, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if captured.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&captured) != identity {
		return errors.New("目标在撤销期间被替换，文件已隔离供人工检查")
	}
	if err = unix.Unlinkat(int(p.Fd()), isolated, 0); err != nil {
		return err
	}
	return p.Sync()
}

// IsolateOwned moves an owned leaf to a caller-journaled path. Recovery must
// inspect that exact path, not scan for an inode shared with other hard links.
func (f *FS) IsolateOwned(name, hold, identity string) error {
	p, base, err := f.parent(name)
	if err != nil {
		return err
	}
	defer p.Close()
	h, held, err := f.parent(hold)
	if err != nil {
		return err
	}
	defer h.Close()
	var stat unix.Stat_t
	if err = unix.Fstatat(int(p.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&stat) != identity {
		return errors.New("目标文件已被替换")
	}
	if err = renameExclusive(int(p.Fd()), base, int(h.Fd()), held); err != nil {
		return err
	}
	if err = unix.Fstatat(int(h.Fd()), held, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&stat) != identity {
		return errors.New("隔离文件归属不明，请人工检查")
	}
	return h.Sync()
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
