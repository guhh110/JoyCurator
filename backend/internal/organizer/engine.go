package organizer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

type Engine struct {
	FS    *fsops.FS
	Image func(context.Context, string) ([]byte, error)
}
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func Hash(ctx context.Context, file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (e *Engine) Check(ctx context.Context, path, identity, checksum string) error {
	file, err := e.FS.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if identity == "" || fsops.Identity(info) != identity {
		return errors.New("文件已被替换，停止操作：" + path)
	}
	hash, err := Hash(ctx, file)
	if err != nil {
		return err
	}
	if checksum == "" || hash != checksum {
		return errors.New("文件内容已被修改，停止操作：" + path)
	}
	return nil
}
func (e *Engine) Run(ctx context.Context, op *model.Operation, save func() error) error {
	// Windows FileInfo does not provide a stable file ID for recovery checks.
	// Fail before creating or moving files rather than writing unowned temp files.
	if runtime.GOOS == "windows" {
		return errors.New("Windows 暂不支持安全地执行文件整理")
	}
	if op.Backup != nil {
		if err := e.Run(ctx, op.Backup, save); err != nil {
			return fmt.Errorf("备份已有文件失败：%w", err)
		}
		if err := e.Check(ctx, op.Backup.Target, op.Backup.Identity, op.Backup.Hash); err != nil {
			return fmt.Errorf("覆盖备份校验失败：%w", err)
		}
	}
	return e.run(ctx, op, save)
}

func (e *Engine) run(ctx context.Context, op *model.Operation, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if op.Status == "completed" {
		if op.Temp != "" {
			if info, err := e.FS.Stat(op.Temp); err == nil && op.Identity != "" && fsops.Identity(info) == op.Identity {
				if err = e.FS.Remove(op.Temp); err != nil {
					return err
				}
			}
			op.Temp = ""
			return save()
		}
		return nil
	}
	if op.Status == "undone" || op.Status == "retained" {
		return errors.New("该操作已撤销")
	}
	if op.Status == "preparing" {
		if info, err := e.FS.Stat(op.Temp); err == nil {
			if op.Identity == "" || fsops.Identity(info) != op.Identity {
				return errors.New("临时文件归属不明，请人工检查：" + op.Temp)
			}
			if err = e.FS.Remove(op.Temp); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		op.Status = "pending"
		op.Temp = ""
		op.Identity = ""
		op.Hash = ""
		if err := save(); err != nil {
			return err
		}
	}
	if op.Status == "pending" || op.Status == "" {
		if _, err := e.FS.Stat(op.Target); err == nil {
			return errors.New("目标已存在，不允许覆盖：" + op.Target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := e.FS.MkdirAll(filepath.Dir(op.Target)); err != nil {
			return err
		}
		var source *os.File
		var reader io.Reader
		if op.Source != "" {
			var err error
			source, err = e.FS.Open(op.Source)
			if err != nil {
				return err
			}
			defer source.Close()
			info, err := source.Stat()
			if err != nil {
				return err
			}
			if info.Size() != op.Size || info.ModTime().UnixNano() != op.MTime {
				return errors.New("源文件已变化，请重新扫描和预览")
			}
			if op.SourceIdentity != "" && op.SourceIdentity != fsops.Identity(info) {
				return errors.New("源文件已被替换，请重新预览")
			}
			op.SourceIdentity = fsops.Identity(info)
			reader = source
		} else if op.Kind == "image" {
			if e.Image == nil {
				return errors.New("图片下载器不可用")
			}
			data, err := e.Image(ctx, op.URL)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(op.Content)
		}
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return err
		}
		op.Temp = filepath.Join(filepath.Dir(op.Target), fmt.Sprintf(".joycurator-%d-%x.part", op.ID, random))
		op.Status = "preparing"
		op.Error = ""
		if err := save(); err != nil {
			return err
		}
		linkMode := op.Mode == "hardlink"
		if op.Mode == "move" && source != nil {
			info, err := source.Stat()
			if err != nil {
				return err
			}
			_, dev, err := e.FS.CheckTarget(op.Target)
			if err != nil {
				return err
			}
			linkMode = dev == e.FS.Device(info)
		}
		if linkMode && source != nil {
			if err := e.FS.Link(op.Source, op.Temp); err != nil {
				return fmt.Errorf("硬链接失败，请检查文件系统与权限：%w", err)
			}
			info, err := e.FS.Stat(op.Temp)
			if err != nil {
				return err
			}
			op.Identity = fsops.Identity(info)
			if err = save(); err != nil {
				return err
			}
			op.Hash, err = Hash(ctx, source)
			if err != nil {
				return err
			}
		} else {
			temp, err := e.FS.CreateExclusive(op.Temp)
			if err != nil {
				return err
			}
			defer temp.Close()
			info, err := temp.Stat()
			if err != nil {
				return err
			}
			op.Identity = fsops.Identity(info)
			if err = save(); err != nil {
				return err
			}
			h := sha256.New()
			if _, err = io.Copy(io.MultiWriter(temp, h), contextReader{ctx, reader}); err != nil {
				return err
			}
			if err = temp.Sync(); err != nil {
				return err
			}
			if err = temp.Close(); err != nil {
				return err
			}
			op.Hash = hex.EncodeToString(h.Sum(nil))
		}
		if source != nil {
			checksum, err := Hash(ctx, source)
			if err != nil {
				return err
			}
			if checksum != op.Hash {
				return errors.New("源文件与临时文件校验不一致，停止发布")
			}
			info, err := source.Stat()
			if err != nil {
				return err
			}
			if info.Size() != op.Size || info.ModTime().UnixNano() != op.MTime {
				return errors.New("复制期间源文件发生变化")
			}
			current, err := e.FS.Stat(op.Source)
			if err != nil {
				return err
			}
			if !os.SameFile(info, current) {
				return errors.New("源文件已被替换")
			}
		}
		if err := e.Check(ctx, op.Temp, op.Identity, op.Hash); err != nil {
			return err
		}
		op.Status = "staged"
		if err := save(); err != nil {
			return err
		}
	}
	if op.Status == "staged" {
		if _, err := e.FS.Stat(op.Target); err == nil {
			if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			if err = e.Check(ctx, op.Temp, op.Identity, op.Hash); err != nil {
				return err
			}
			if err = e.FS.Publish(op.Temp, op.Target); err != nil {
				return err
			}
		}
		op.Status = "published"
		if err := save(); err != nil {
			return err
		}
	}
	if op.Status == "published" {
		if err := e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
			return err
		}
		if op.Mode == "move" && op.Source != "" {
			if _, err := e.FS.Stat(op.Source); err == nil {
				if err = e.Check(ctx, op.Source, op.SourceIdentity, op.Hash); err != nil {
					return err
				}
				if err = e.FS.Remove(op.Source); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		op.Status = "completed"
		op.Error = ""
		if err := save(); err != nil {
			return err
		}
	}
	if op.Status == "completed" && op.Temp != "" {
		if info, err := e.FS.Stat(op.Temp); err == nil && op.Identity != "" && fsops.Identity(info) == op.Identity {
			if err = e.FS.Remove(op.Temp); err != nil {
				return err
			}
		}
		op.Temp = ""
		return save()
	}
	return nil
}

func (e *Engine) Undo(ctx context.Context, op *model.Operation, save func() error) error {
	if runtime.GOOS == "windows" {
		return errors.New("Windows 暂不支持安全地撤销文件整理")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if backup := op.Backup; backup != nil && (backup.Status == "completed" || backup.Status == "published") {
		if err := e.Check(ctx, backup.Target, backup.Identity, backup.Hash); err != nil {
			return fmt.Errorf("覆盖备份校验失败，停止撤销：%w", err)
		}
	}
	if err := e.undo(ctx, op, save); err != nil {
		return err
	}
	if op.Backup != nil {
		return e.Undo(ctx, op.Backup, save)
	}
	return nil
}

func (e *Engine) undo(ctx context.Context, op *model.Operation, save func() error) error {
	if op.Status == "undone" || op.Status == "retained" {
		return nil
	}
	if op.Status == "pending" || op.Status == "" {
		op.Status = "undone"
		return save()
	}
	if op.Status == "preparing" || op.Status == "staged" {
		// A staged operation may have published just before a crash.
		if info, err := e.FS.Stat(op.Target); err == nil && op.Identity != "" && fsops.Identity(info) == op.Identity {
			op.Status = "published"
			if err = save(); err != nil {
				return err
			}
		} else {
			if op.Temp != "" {
				if info, err := e.FS.Stat(op.Temp); err == nil {
					if op.Identity == "" || fsops.Identity(info) != op.Identity {
						return errors.New("临时文件归属不明，请人工检查")
					}
					if err = e.FS.Remove(op.Temp); err != nil {
						return err
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
			op.Status = "undone"
			return save()
		}
	}
	if op.Status != "undoing" {
		if err := e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
			return err
		}
		op.Status = "undoing"
		if err := save(); err != nil {
			return err
		}
	}
	if op.Mode == "move" && op.Source != "" {
		if op.Undo == nil {
			info, err := e.FS.Stat(op.Source)
			if err == nil {
				if fsops.Identity(info) != op.SourceIdentity {
					return errors.New("原路径已被占用，停止撤销：" + op.Source)
				}
				if err = e.Check(ctx, op.Source, op.SourceIdentity, op.Hash); err != nil {
					return err
				}
			} else if errors.Is(err, os.ErrNotExist) {
				info, err = e.FS.Stat(op.Target)
				if err != nil {
					return err
				}
				mode := "copy"
				// Backups live beside the original. Preserve its identity so older
				// organizer records remain verifiable after restoring the replacement.
				if op.Kind == "backup" {
					mode = "hardlink"
				}
				op.Undo = &model.Operation{ID: op.ID, Source: op.Target, Target: op.Source, Mode: mode, Kind: "video", Size: info.Size(), MTime: info.ModTime().UnixNano(), Status: "pending"}
				if err = save(); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		if op.Undo != nil {
			if err := e.Run(ctx, op.Undo, save); err != nil {
				return err
			}
			if err := e.Check(ctx, op.Source, op.Undo.Identity, op.Hash); err != nil {
				return err
			}
		}
	}
	if _, err := e.FS.Stat(op.Target); err == nil {
		if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
			return err
		}
		if err = e.FS.Remove(op.Target); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if op.Temp != "" {
		if info, err := e.FS.Stat(op.Temp); err == nil && op.Identity != "" && fsops.Identity(info) == op.Identity {
			if err = e.FS.Remove(op.Temp); err != nil {
				return err
			}
		}
	}
	op.Status = "undone"
	op.Error = ""
	return save()
}
