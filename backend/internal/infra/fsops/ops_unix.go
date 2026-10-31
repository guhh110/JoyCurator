//go:build linux || darwin

package fsops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func fmtIdentity(stat *unix.Stat_t) string { return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino) }

func fastStatMatches(stat *unix.Stat_t, identity string, size, mtime int64) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && fmtIdentity(stat) == identity && stat.Size == size &&
		time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec).UnixNano() == mtime
}

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

// RenameOwned keeps publication anchored to verified parent directories.
// OverwriteFast never follows a target symlink and preserves the previous
// target in a journaled hold path until the caller removes it.
func (f *FS) RenameOwned(source, target, identity string) error {
	s, src, err := f.parent(source)
	if err != nil {
		return err
	}
	defer s.Close()
	var current unix.Stat_t
	if err = unix.Fstatat(int(s.Fd()), src, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if current.Mode&unix.S_IFMT != unix.S_IFREG || identity == "" || fmtIdentity(&current) != identity {
		return errors.New("来源文件已变化")
	}
	d, dst, err := f.parent(target)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = renameExclusive(int(s.Fd()), src, int(d.Fd()), dst); err != nil {
		return err
	}
	return d.Sync()
}

func (f *FS) OverwriteFast(source, target, hold, identity string, size, mtime int64, stagedIdentity string) error {
	s, src, err := f.parent(source)
	if err != nil {
		return err
	}
	defer s.Close()
	var staged unix.Stat_t
	if err = unix.Fstatat(int(s.Fd()), src, &staged, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if staged.Mode&unix.S_IFMT != unix.S_IFREG || stagedIdentity == "" || fmtIdentity(&staged) != stagedIdentity {
		return errors.New("来源文件已变化")
	}
	d, dst, err := f.parent(target)
	if err != nil {
		return err
	}
	defer d.Close()
	h, held, err := f.parent(hold)
	if err != nil {
		return err
	}
	defer h.Close()
	var current unix.Stat_t
	if err = unix.Fstatat(int(d.Fd()), dst, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !fastStatMatches(&current, identity, size, mtime) {
		return errors.New("待覆盖文件已变化，请重新预览")
	}
	if err = renameExclusive(int(d.Fd()), dst, int(h.Fd()), held); err != nil {
		return err
	}
	if err = unix.Fstatat(int(h.Fd()), held, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil || !fastStatMatches(&current, identity, size, mtime) {
		return errors.Join(errors.New("待覆盖文件在隔离期间发生变化，请人工检查暂存路径"), err)
	}
	if err = unix.Fstatat(int(s.Fd()), src, &staged, unix.AT_SYMLINK_NOFOLLOW); err != nil || staged.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&staged) != stagedIdentity {
		if rollback := renameExclusive(int(h.Fd()), held, int(d.Fd()), dst); rollback != nil {
			return errors.Join(errors.New("来源变化，旧文件保留在暂存路径"), err, rollback)
		}
		return errors.New("来源文件已变化")
	}
	if err = renameExclusive(int(s.Fd()), src, int(d.Fd()), dst); err != nil {
		if rollback := renameExclusive(int(h.Fd()), held, int(d.Fd()), dst); rollback != nil {
			return errors.Join(err, rollback)
		}
		return err
	}
	return d.Sync()
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
