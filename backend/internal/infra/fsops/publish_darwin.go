package fsops

import "golang.org/x/sys/unix"

func renameExclusive(src int, source string, dst int, target string) error {
	return unix.RenameatxNp(src, source, dst, target, unix.RENAME_EXCL)
}
