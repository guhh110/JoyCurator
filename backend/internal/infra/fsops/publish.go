//go:build linux || darwin

package fsops

func (f *FS) Publish(source, target string) error {
	src, name, err := f.parent(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, base, err := f.parent(target)
	if err != nil {
		return err
	}
	defer dst.Close()
	if err = renameExclusive(int(src.Fd()), name, int(dst.Fd()), base); err != nil {
		return err
	}
	return dst.Sync()
}
