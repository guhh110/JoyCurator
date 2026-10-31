package organizer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
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
	if op.Kind == "video" || op.Kind == "subtitle" {
		return e.fastRun(ctx, op, save)
	}
	return e.run(ctx, op, save)
}

func (e *Engine) run(ctx context.Context, op *model.Operation, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if op.Status == "completed" {
		return e.cleanup(op, save)
	}
	if op.Status == "undone" || op.Status == "retained" {
		return errors.New("该操作已撤销")
	}
	if op.Status == "preparing" {
		if err := e.recoverHold(ctx, op, save); err != nil {
			return err
		}
		if op.Status == "published" {
			return e.run(ctx, op, save)
		}
		if op.SourceRemoveHold != "" {
			return errors.New("来源正在隔离，停止重新暂存：" + op.Source)
		}
		if op.Identity != "" {
			if info, err := e.FS.Stat(op.Target); err == nil && info.Mode().IsRegular() && fsops.Identity(info) == op.Identity && op.Hash != "" {
				if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
					return err
				}
				op.Status = "published"
				if err = save(); err != nil {
					return err
				}
				return e.run(ctx, op, save)
			}
		}
		if op.Temp != "" {
			if info, err := e.FS.Stat(op.Temp); err == nil {
				if op.Identity == "" || !info.Mode().IsRegular() || fsops.Identity(info) != op.Identity {
					return errors.New("临时文件归属不明，请人工检查：" + op.Temp)
				}
				if err = e.FS.RemoveOwned(op.Temp, op.Identity); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
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
		if info, err := e.FS.Stat(op.Target); err == nil {
			if !op.Replace || op.ReplaceIdentity == "" || !info.Mode().IsRegular() || fsops.Identity(info) != op.ReplaceIdentity || info.Size() != op.ReplaceSize || info.ModTime().UnixNano() != op.ReplaceMTime {
				return errors.New("待覆盖文件已变化，请重新预览：" + op.Target)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else if op.Replace {
			return errors.New("待覆盖文件已不存在：" + op.Target)
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
			if op.ConvertJPEG {
				decoded, _, decodeErr := image.Decode(bytes.NewReader(data))
				if decodeErr != nil {
					return decodeErr
				}
				var converted bytes.Buffer
				if err = jpeg.Encode(&converted, decoded, &jpeg.Options{Quality: 90}); err != nil {
					return err
				}
				data = converted.Bytes()
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
		if err := e.recoverHold(ctx, op, save); err != nil {
			return err
		}
		if op.Status == "published" {
			return e.run(ctx, op, save)
		}
		if _, err := e.FS.Stat(op.Target); err == nil {
			if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
				if !op.Replace || op.ReplaceIdentity == "" {
					return err
				}
				info, statErr := e.FS.Stat(op.Target)
				if statErr != nil || fsops.Identity(info) != op.ReplaceIdentity || info.Size() != op.ReplaceSize || info.ModTime().UnixNano() != op.ReplaceMTime {
					return errors.New("待覆盖文件已变化：" + op.Target)
				}
				if err = e.Check(ctx, op.Temp, op.Identity, op.Hash); err != nil {
					if !errors.Is(err, os.ErrNotExist) {
						return err
					}
					return errors.New("暂存文件丢失，无法确认覆盖状态：" + op.Temp)
				}
				if op.ReplaceHold == "" {
					var random [8]byte
					if _, err = rand.Read(random[:]); err != nil {
						return err
					}
					op.ReplaceHold = filepath.Join(filepath.Dir(op.Target), fmt.Sprintf(".joycurator-hold-%d-%x.part", op.ID, random))
					if err = save(); err != nil {
						return err
					}
				}
				if err = e.FS.Replace(op.Temp, op.Target, op.ReplaceHold, op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime, op.Identity); err != nil {
					return err
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			if op.Replace && op.ReplaceHold == "" {
				return errors.New("待覆盖文件已不存在，请重新预览：" + op.Target)
			}
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
				if op.SourceRemoveHold != "" {
					return errors.New("来源在隔离期间重新出现，请人工检查：" + op.Source)
				}
				if err = e.Check(ctx, op.Source, op.SourceIdentity, op.Hash); err != nil {
					return err
				}
				if err = e.removeJournaled(ctx, op, true, save); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if op.SourceRemoveHold != "" {
				if err := e.removeJournaled(ctx, op, true, save); err != nil {
					return err
				}
			}
		}
		op.Status = "completed"
		op.Error = ""
		if err := save(); err != nil {
			return err
		}
	}
	return e.cleanup(op, save)
}

func (e *Engine) cleanup(op *model.Operation, save func() error) error {
	if op.ReplaceHold != "" {
		info, err := e.FS.Stat(op.ReplaceHold)
		if err == nil {
			if !info.Mode().IsRegular() || fsops.Identity(info) != op.ReplaceIdentity {
				return errors.New("旧文件暂存归属不明：" + op.ReplaceHold)
			}
			if err = e.FS.RemoveOwned(op.ReplaceHold, op.ReplaceIdentity); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		op.ReplaceHold = ""
		if err = save(); err != nil {
			return err
		}
	}
	if op.Temp == "" {
		return nil
	}
	identity := op.Identity
	if op.Replace {
		identity = op.ReplaceIdentity
	}
	info, err := e.FS.Stat(op.Temp)
	if err == nil {
		if identity == "" || !info.Mode().IsRegular() || fsops.Identity(info) != identity {
			return errors.New("暂存文件归属不明，请人工检查：" + op.Temp)
		}
		if err = e.FS.RemoveOwned(op.Temp, identity); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	op.Temp = ""
	return save()
}

func (e *Engine) recoverHold(ctx context.Context, op *model.Operation, save func() error) error {
	if op.ReplaceHold == "" {
		return nil
	}
	held, err := e.FS.Stat(op.ReplaceHold)
	if errors.Is(err, os.ErrNotExist) {
		if op.Identity != "" {
			if target, targetErr := e.FS.Stat(op.Target); targetErr == nil && target.Mode().IsRegular() && fsops.Identity(target) == op.Identity {
				if err := e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
					return err
				}
				op.Status = "published"
				return save()
			}
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !held.Mode().IsRegular() || fsops.Identity(held) != op.ReplaceIdentity {
		return errors.New("旧文件暂存归属不明：" + op.ReplaceHold)
	}
	target, err := e.FS.Stat(op.Target)
	if errors.Is(err, os.ErrNotExist) {
		// If the staged file is already gone, publication may have succeeded
		// before an external deletion. Do not resurrect the old content.
		if _, stagedErr := e.FS.Stat(op.Temp); errors.Is(stagedErr, os.ErrNotExist) {
			return errors.New("覆盖状态不明，旧文件保留在暂存路径，请人工检查")
		} else if stagedErr != nil {
			return stagedErr
		}
		if err = e.Check(ctx, op.Temp, op.Identity, op.Hash); err != nil {
			return err
		}
		if err = e.FS.Publish(op.ReplaceHold, op.Target); err != nil {
			return err
		}
		op.ReplaceHold = ""
		return save()
	}
	if err != nil {
		return err
	}
	if !target.Mode().IsRegular() || fsops.Identity(target) != op.Identity {
		return errors.New("目标在覆盖期间变化，请人工检查旧文件暂存")
	}
	if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
		return err
	}
	op.Status = "published"
	return save()
}

// removeJournaled records the exact isolation path before moving the leaf.
// A retry only touches that path after verifying both identity and contents.
func (e *Engine) removeJournaled(ctx context.Context, op *model.Operation, source bool, save func() error) error {
	path, identity, hold := op.Target, op.Identity, &op.RemoveHold
	if source {
		path, identity, hold = op.Source, op.SourceIdentity, &op.SourceRemoveHold
	}
	if *hold == "" {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return err
		}
		*hold = filepath.Join(filepath.Dir(path), fmt.Sprintf(".joycurator-remove-%d-%x.part", op.ID, random))
		if err := save(); err != nil {
			return err
		}
	}
	if _, err := e.FS.Stat(*hold); errors.Is(err, os.ErrNotExist) {
		if _, err = e.FS.Stat(path); err == nil {
			if err = e.Check(ctx, path, identity, op.Hash); err != nil {
				return err
			}
			if err = e.FS.IsolateOwned(path, *hold, identity); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err != nil {
		return err
	} else if _, err = e.FS.Stat(path); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("隔离期间目标路径被占用，请人工检查：" + path)
	}
	if _, err := e.FS.Stat(*hold); err == nil {
		if err = e.Check(ctx, *hold, identity, op.Hash); err != nil {
			return err
		}
		if err = e.FS.RemoveOwned(*hold, identity); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	*hold = ""
	return save()
}

func (e *Engine) Undo(ctx context.Context, op *model.Operation, save func() error) error {
	if runtime.GOOS == "windows" {
		return errors.New("Windows 暂不支持安全地撤销文件整理")
	}
	if op.Kind == "video" || op.Kind == "subtitle" {
		return e.fastUndo(ctx, op, save)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.undo(ctx, op, save)
}

func (e *Engine) undo(ctx context.Context, op *model.Operation, save func() error) error {
	if op.Status == "undone" || op.Status == "retained" {
		return nil
	}
	if op.SourceRemoveHold != "" {
		return errors.New("来源删除状态未确认，请先重试整理再撤销")
	}
	if op.ReplaceHold != "" {
		if err := e.recoverHold(ctx, op, save); err != nil {
			return err
		}
	}
	if op.Status == "pending" || op.Status == "" {
		op.Status = "undone"
		return save()
	}
	if op.Status == "preparing" || op.Status == "staged" {
		// A staged operation may have published just before a crash.
		if err := e.recoverHold(ctx, op, save); err != nil {
			return err
		}
		if info, err := e.FS.Stat(op.Target); err == nil && op.Identity != "" && fsops.Identity(info) == op.Identity {
			if err = e.Check(ctx, op.Target, op.Identity, op.Hash); err != nil {
				return err
			}
			op.Status = "published"
			if err = save(); err != nil {
				return err
			}
		} else if op.Status != "published" {
			if op.ReplaceHold != "" {
				if _, heldErr := e.FS.Stat(op.ReplaceHold); heldErr == nil {
					return errors.New("旧文件仍在暂存路径，停止撤销：" + op.ReplaceHold)
				} else if !errors.Is(heldErr, os.ErrNotExist) {
					return heldErr
				}
			}
			if op.Temp != "" {
				if info, err := e.FS.Stat(op.Temp); err == nil {
					if op.Replace && fsops.Identity(info) == op.ReplaceIdentity {
						return errors.New("覆盖可能已发布但目标已变化，停止撤销")
					}
					if op.Identity == "" || fsops.Identity(info) != op.Identity {
						return errors.New("临时文件归属不明，请人工检查")
					}
					if err = e.FS.RemoveOwned(op.Temp, op.Identity); err != nil {
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
				op.Undo = &model.Operation{ID: op.ID, Source: op.Target, Target: op.Source, Mode: "copy", Kind: "video", Size: info.Size(), MTime: info.ModTime().UnixNano(), Status: "pending"}
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
		if err = e.removeJournaled(ctx, op, false, save); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if op.RemoveHold != "" {
		if err := e.removeJournaled(ctx, op, false, save); err != nil {
			return err
		}
	}
	if op.Temp != "" {
		info, err := e.FS.Stat(op.Temp)
		if err == nil {
			identity := op.Identity
			if op.Replace {
				identity = op.ReplaceIdentity
			}
			if identity == "" || !info.Mode().IsRegular() || fsops.Identity(info) != identity {
				return errors.New("暂存文件归属不明，请人工检查：" + op.Temp)
			}
			if err = e.FS.RemoveOwned(op.Temp, identity); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		op.Temp = ""
	}
	if op.ReplaceHold != "" {
		info, err := e.FS.Stat(op.ReplaceHold)
		if err == nil {
			if !info.Mode().IsRegular() || fsops.Identity(info) != op.ReplaceIdentity {
				return errors.New("旧文件暂存归属不明：" + op.ReplaceHold)
			}
			if err = e.FS.RemoveOwned(op.ReplaceHold, op.ReplaceIdentity); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		op.ReplaceHold = ""
	}
	op.Status = "undone"
	op.Error = ""
	return save()
}
