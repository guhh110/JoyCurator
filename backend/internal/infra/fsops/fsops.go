// Package fsops anchors all file access to explicitly authorized roots.
package fsops

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FS struct {
	Roots   []string
	aliases map[string]string
	// DeviceOf overrides device resolution. Tests replace it to simulate a
	// second filesystem without mounting one.
	DeviceOf func(os.FileInfo) uint64
}

func New(roots []string) (*FS, error) {
	f := &FS{aliases: map[string]string{}}
	for _, root := range roots {
		p, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		p, err = filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("media root is not a directory: %s", root)
		}
		f.Roots = append(f.Roots, p)
		alias, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		f.aliases[alias] = p
	}
	return f, nil
}
func (f *FS) Canonical(name string) string {
	name = filepath.Clean(name)
	for _, base := range f.Roots {
		if Within(base, name) {
			return name
		}
	}
	for alias, base := range f.aliases {
		if Within(alias, name) {
			rel, _ := filepath.Rel(alias, name)
			return filepath.Join(base, rel)
		}
	}
	return name
}
func Within(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && filepath.IsLocal(rel)
}
func (f *FS) Root(name string) (*os.Root, string, error) {
	if !filepath.IsAbs(name) {
		return nil, "", errors.New("需要绝对路径")
	}
	name = f.Canonical(name)
	for _, base := range f.Roots {
		if Within(base, name) {
			rel, _ := filepath.Rel(base, name)
			root, err := os.OpenRoot(base)
			return root, rel, err
		}
	}
	return nil, "", errors.New("路径不在授权媒体根目录内")
}
func (f *FS) Open(name string) (*os.File, error) {
	root, rel, err := f.Root(name)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("只支持普通文件，不跟随文件符号链接")
	}
	return root.Open(rel)
}
func (f *FS) Stat(name string) (os.FileInfo, error) {
	root, rel, err := f.Root(name)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Lstat(rel)
}
func (f *FS) Read(name string) ([]byte, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, 4<<20))
}
func (f *FS) MkdirAll(name string) error {
	root, rel, err := f.Root(name)
	if err != nil {
		return err
	}
	defer root.Close()
	parts := strings.Split(rel, string(filepath.Separator))
	current := ""
	for _, part := range parts {
		current = filepath.Join(current, part)
		if err := root.Mkdir(current, 0755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := root.Stat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("目标父路径不是目录")
		}
	}
	return nil
}
func (f *FS) parent(name string) (*os.File, string, error) {
	root, rel, err := f.Root(name)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	p, err := root.Open(filepath.Dir(rel))
	return p, filepath.Base(rel), err
}
func (f *FS) CreateExclusive(name string) (*os.File, error) {
	root, rel, err := f.Root(name)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
}
func (f *FS) CheckTarget(name string) (int64, uint64, error) {
	preflightRoot, _, err := f.Root(name)
	if err != nil {
		return 0, 0, err
	}
	preflightRoot.Close()
	parent := filepath.Dir(name)
	for {
		info, err := f.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return 0, 0, errors.New("目标父路径不是目录")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return 0, 0, err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return 0, 0, err
		}
		parent = next
	}
	root, rel, err := f.Root(parent)
	if err != nil {
		return 0, 0, err
	}
	defer root.Close()
	dir, err := root.Open(rel)
	if err != nil {
		return 0, 0, err
	}
	defer dir.Close()
	return checkTargetDir(dir)
}

// Device resolves a file's device id, honoring DeviceOf when it is set.
func (f *FS) Device(info os.FileInfo) uint64 {
	if f.DeviceOf != nil {
		return f.DeviceOf(info)
	}
	return Device(info)
}
