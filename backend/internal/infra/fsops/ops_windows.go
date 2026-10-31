package fsops

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NT file information operations accept a destination directory HANDLE and a
// relative filename. Never reassemble parent.Name() into an absolute path:
// a concurrently replaced junction must not redirect a publish outside root.
type relativeFileInformation struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

func setRelativeFileInfo(source *os.File, target *os.File, base string, class uint32) error {
	utf16, err := windows.UTF16FromString(base)
	if err != nil {
		return err
	}
	var header relativeFileInformation
	length := int(unsafe.Offsetof(header.FileName)) + 2*(len(utf16)-1)
	buffer := make([]byte, length)
	info := (*relativeFileInformation)(unsafe.Pointer(&buffer[0]))
	info.RootDirectory = windows.Handle(target.Fd())
	info.FileNameLength = uint32(2 * (len(utf16) - 1))
	copy(unsafe.Slice(&info.FileName[0], len(utf16)-1), utf16)
	return windows.NtSetInformationFile(windows.Handle(source.Fd()), &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(length), class)
}

// openLeaf opens only a regular file relative to its already-verified parent.
// FILE_OPEN_REPARSE_POINT prevents a swapped leaf symlink from being followed.
func openLeaf(parent *os.File, base string, access uint32) (*os.File, error) {
	name, err := windows.NewNTUnicodeString(base)
	if err != nil {
		return nil, err
	}
	attrs := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: name}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var h windows.Handle
	err = windows.NtCreateFile(&h, access|windows.SYNCHRONIZE|windows.FILE_READ_ATTRIBUTES, &attrs,
		&windows.IO_STATUS_BLOCK{}, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err == nil && info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		err = errors.New("refusing to operate on a reparse point")
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), base), nil
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
	file, err := openLeaf(s, src, windows.FILE_WRITE_ATTRIBUTES)
	if err != nil {
		return err
	}
	defer file.Close()
	return setRelativeFileInfo(file, d, dst, windows.FileLinkInformation)
}

func (f *FS) Remove(name string) error {
	p, base, err := f.parent(name)
	if err != nil {
		return err
	}
	defer p.Close()
	file, err := openLeaf(p, base, windows.DELETE)
	if err != nil {
		return err
	}
	defer file.Close()
	// The legacy disposition structure has a single byte DeleteFile field.
	one := byte(1)
	return windows.NtSetInformationFile(windows.Handle(file.Fd()), &windows.IO_STATUS_BLOCK{}, &one, 1, windows.FileDispositionInformation)
}

func (f *FS) RemoveOwned(name, identity string) error {
	return errors.New("Windows 暂不支持安全撤销")
}

func (f *FS) IsolateOwned(name, hold, identity string) error {
	return errors.New("Windows 暂不支持安全隔离")
}

func (f *FS) Publish(source, target string) error {
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
	file, err := openLeaf(s, src, windows.DELETE)
	if err != nil {
		return err
	}
	defer file.Close()
	// Zero Flags means the kernel refuses to replace any existing target.
	return setRelativeFileInfo(file, d, dst, windows.FileRenameInformation)
}

func (f *FS) Replace(source, target, hold, identity string, size, mtime int64, stagedIdentity string) error {
	return errors.New("Windows 暂不支持替换媒体文件")
}

func checkTargetDir(dir *os.File) (int64, uint64, error) {
	h := windows.Handle(dir.Fd())
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return 0, 0, err
	}
	// A read-only directory cannot receive files. A full ACL check requires
	// an open with FILE_ADD_FILE; the eventual create enforces the kernel ACL.
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		return 0, 0, syscall.ERROR_ACCESS_DENIED
	}
	path := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &path[0], uint32(len(path)), 0)
	if err != nil {
		return 0, 0, err
	}
	if n >= uint32(len(path)) {
		return 0, 0, syscall.ERROR_INSUFFICIENT_BUFFER
	}
	name := path[:n]
	name = append(name, 0)
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(&name[0], &free, nil, nil); err != nil {
		return 0, 0, err
	}
	if free > uint64(^uint64(0)>>1) {
		free = uint64(^uint64(0) >> 1)
	}
	return int64(free), uint64(info.VolumeSerialNumber), nil
}
