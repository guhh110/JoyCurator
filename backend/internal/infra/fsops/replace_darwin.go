package fsops

import (
	"errors"
)

func (f *FS) Replace(source, target, hold, identity string, size, mtime int64, stagedIdentity string) error {
	return errors.New("当前系统不支持安全的无备份覆盖")
}
