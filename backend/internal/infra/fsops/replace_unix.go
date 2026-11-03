//go:build linux || darwin

package fsops

import (
	"errors"

	"golang.org/x/sys/unix"
)

// Replace isolates the old target and publishes the staged file exclusively.
// An external file introduced during publication is never overwritten.
func (f *FS) Replace(source, target, hold, identity string, size, mtime int64, stagedIdentity string) error {
	s, src, err := f.parent(source)
	if err != nil {
		return err
	}
	defer s.Close()
	var staged unix.Stat_t
	if err = unix.Fstatat(int(s.Fd()), src, &staged, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if staged.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&staged) != stagedIdentity {
		return errors.New("暂存文件已变化，停止覆盖")
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
	var expected unix.Stat_t
	if err = unix.Fstatat(int(d.Fd()), dst, &expected, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !fastStatMatches(&expected, identity, size, mtime) {
		return errors.New("待覆盖文件已变化，请重新预览")
	}
	if err = renameExclusive(int(d.Fd()), dst, int(h.Fd()), held); err != nil {
		return err
	}
	var old unix.Stat_t
	err = unix.Fstatat(int(h.Fd()), held, &old, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil || !fastStatMatches(&old, identity, size, mtime) {
		if rollbackErr := renameExclusive(int(h.Fd()), held, int(d.Fd()), dst); rollbackErr != nil {
			return errors.Join(errors.New("待覆盖文件变化且不能恢复，请人工检查暂存文件"), rollbackErr)
		}
		return errors.New("待覆盖文件在发布期间变化，请重新预览")
	}
	if err = unix.Fstatat(int(s.Fd()), src, &staged, unix.AT_SYMLINK_NOFOLLOW); err != nil || staged.Mode&unix.S_IFMT != unix.S_IFREG || fmtIdentity(&staged) != stagedIdentity {
		if rollbackErr := renameExclusive(int(h.Fd()), held, int(d.Fd()), dst); rollbackErr != nil {
			return errors.Join(errors.New("暂存文件在发布期间变化，旧文件保留在暂存路径"), err, rollbackErr)
		}
		return errors.New("暂存文件在发布期间变化，请重新预览")
	}
	if err = renameExclusive(int(s.Fd()), src, int(d.Fd()), dst); err != nil {
		if rollbackErr := renameExclusive(int(h.Fd()), held, int(d.Fd()), dst); rollbackErr != nil {
			return errors.Join(errors.New("目标被占用，原文件保留在暂存路径"), err, rollbackErr)
		}
		return err
	}
	if err = d.Sync(); err == nil {
		err = h.Sync()
	}
	return err
}
