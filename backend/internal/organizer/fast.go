package organizer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

var errManualReview = errors.New("文件操作中断或状态不明，请人工核对来源、目标和暂存文件")

func fastPath(target string, id int64, label string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(target), fmt.Sprintf(".joycurator-%s-%d-%x.part", label, id, suffix)), nil
}

func (e *Engine) fastLeaf(path string, identity string) (os.FileInfo, error) {
	info, err := e.FS.Stat(path)
	if err != nil || !info.Mode().IsRegular() || identity == "" || fsops.Identity(info) != identity {
		return nil, errors.New("文件已变化，请人工检查：" + path)
	}
	return info, nil
}

func (e *Engine) discardFastTemp(op *model.Operation, identity string, save func() error) error {
	if err := e.FS.RemoveOwned(op.Temp, identity); err != nil {
		return err
	}
	op.Status, op.Temp, op.ReplaceHold, op.SourceIdentity, op.Identity = "pending", "", "", "", ""
	return save()
}

func (e *Engine) fastRun(ctx context.Context, op *model.Operation, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if op.Status == "completed" {
		return nil
	}
	if op.Status != "pending" && op.Status != "" {
		return errManualReview
	}
	source, err := e.FS.Open(op.Source)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != op.Size || info.ModTime().UnixNano() != op.MTime {
		return errors.New("源文件已变化，请重新预览")
	}
	if op.SourceIdentity != "" && op.SourceIdentity != fsops.Identity(info) {
		return errors.New("源文件已被替换，请重新预览")
	}
	if named, statErr := e.FS.Stat(op.Source); statErr != nil || !os.SameFile(info, named) {
		return errors.New("源文件已被替换，请重新预览")
	}
	current, err := e.FS.Stat(op.Target)
	if err == nil {
		if !op.Replace || !current.Mode().IsRegular() || fsops.Identity(current) != op.ReplaceIdentity || current.Size() != op.ReplaceSize || current.ModTime().UnixNano() != op.ReplaceMTime {
			return errors.New("待覆盖文件已变化，请重新预览")
		}
	} else if !errors.Is(err, os.ErrNotExist) || op.Replace {
		return errors.New("目标文件已变化，请重新预览")
	}
	if err = e.FS.MkdirAll(filepath.Dir(op.Target)); err != nil {
		return err
	}
	_, device, err := e.FS.CheckTarget(op.Target)
	if err != nil {
		return err
	}
	same := device == e.FS.Device(info)
	if op.Mode == "hardlink" && !same {
		return errors.New("硬链接不能跨磁盘")
	}
	op.SourceIdentity = fsops.Identity(info)
	directMove := op.Mode == "move" && same
	if op.Replace {
		op.ReplaceHold, err = fastPath(op.Target, op.ID, "hold")
		if err != nil {
			return err
		}
	}
	if !directMove {
		op.Temp, err = fastPath(op.Target, op.ID, "fast")
		if err != nil {
			return err
		}
	}
	op.Status = "preparing"
	if err = save(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		op.Status, op.Temp, op.ReplaceHold, op.SourceIdentity = "pending", "", "", ""
		if saveErr := save(); saveErr != nil {
			return saveErr
		}
		return err
	}
	if directMove {
		if op.Replace {
			err = e.FS.OverwriteFast(op.Source, op.Target, op.ReplaceHold, op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime, op.SourceIdentity)
		} else {
			err = e.FS.RenameOwned(op.Source, op.Target, op.SourceIdentity)
		}
	} else if op.Mode == "hardlink" {
		err = e.FS.Link(op.Source, op.Temp)
	} else {
		var temp *os.File
		var created os.FileInfo
		temp, err = e.FS.CreateExclusive(op.Temp)
		if err == nil {
			created, err = temp.Stat()
			var copied int64
			if err == nil {
				copied, err = io.Copy(temp, contextReader{ctx: ctx, r: source})
			}
			if err == nil && copied != op.Size {
				err = errors.New("复制期间源文件大小发生变化")
			}
			if err == nil {
				err = temp.Sync()
			}
			closeErr := temp.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil && created != nil {
			if temporary, statErr := e.FS.Stat(op.Temp); statErr == nil && os.SameFile(created, temporary) {
				if removeErr := e.discardFastTemp(op, fsops.Identity(created), save); removeErr != nil {
					return errors.Join(err, removeErr)
				}
			}
		} else if err != nil && created == nil && temp != nil {
			return errors.Join(err, errManualReview)
		}
	}
	if err != nil {
		return err
	}
	if !directMove {
		staged, stagedErr := e.FS.Stat(op.Temp)
		if stagedErr != nil || !staged.Mode().IsRegular() {
			return errManualReview
		}
		op.Identity = fsops.Identity(staged)
		if staged.Size() != op.Size {
			return errors.New("暂存文件大小不符，请人工检查")
		}
		if named, statErr := e.FS.Stat(op.Source); statErr != nil || !os.SameFile(info, named) || named.Size() != op.Size || named.ModTime().UnixNano() != op.MTime {
			return errors.New("复制期间源文件发生变化，请人工检查暂存文件")
		}
		if op.Mode == "move" || op.Replace {
			op.Status = "staged"
			if err = save(); err != nil {
				return err
			}
		}
		if err = ctx.Err(); err != nil {
			if removeErr := e.discardFastTemp(op, op.Identity, save); removeErr != nil {
				return removeErr
			}
			return err
		}
		if op.Replace {
			err = e.FS.OverwriteFast(op.Temp, op.Target, op.ReplaceHold, op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime, op.Identity)
		} else {
			err = e.FS.RenameOwned(op.Temp, op.Target, op.Identity)
		}
		if err != nil {
			return err
		}
	}
	if op.Identity == "" {
		op.Identity = op.SourceIdentity
	}
	if _, err = e.fastLeaf(op.Target, op.Identity); err != nil {
		return err
	}
	op.Status = "published"
	if err = save(); err != nil {
		return err
	}
	if op.Mode == "move" && !directMove {
		if err = e.FS.RemoveOwned(op.Source, op.SourceIdentity); err != nil {
			return err
		}
	}
	if op.ReplaceHold != "" {
		op.Status = "cleanup"
		if err = save(); err != nil {
			return err
		}
		if err = e.FS.RemoveOwned(op.ReplaceHold, op.ReplaceIdentity); err != nil {
			return err
		}
	}
	op.Temp, op.ReplaceHold, op.Status, op.Error = "", "", "completed", ""
	return save()
}

func (e *Engine) fastUndo(ctx context.Context, op *model.Operation, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if op.Status == "undone" {
		return nil
	}
	if op.Status == "pending" || op.Status == "" {
		op.Status = "undone"
		return save()
	}
	if op.Status != "completed" {
		return errManualReview
	}
	info, err := e.FS.Stat(op.Target)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("目标不存在或不是普通文件，不能撤销")
	}
	if op.Mode == "move" {
		if _, err = e.FS.Stat(op.Source); !errors.Is(err, os.ErrNotExist) {
			return errors.New("来源路径已被占用，停止撤销")
		}
		if err = e.FS.MkdirAll(filepath.Dir(op.Source)); err != nil {
			return err
		}
		_, device, err := e.FS.CheckTarget(op.Source)
		if err != nil {
			return err
		}
		if device != e.FS.Device(info) {
			tempPath, pathErr := fastPath(op.Source, op.ID, "undo")
			if pathErr != nil {
				return pathErr
			}
			op.Undo = &model.Operation{Target: op.Source, Temp: tempPath, Status: "preparing"}
			op.Status = "undoing"
			if err = save(); err != nil {
				return err
			}
			from, openErr := e.FS.Open(op.Target)
			if openErr != nil {
				return openErr
			}
			defer from.Close()
			opened, statErr := from.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				return errors.New("目标文件在撤销期间发生变化")
			}
			to, createErr := e.FS.CreateExclusive(tempPath)
			if createErr != nil {
				return createErr
			}
			copied, copyErr := io.Copy(to, contextReader{ctx: ctx, r: from})
			err = copyErr
			if err == nil && copied != opened.Size() {
				err = errors.New("撤销复制期间目标文件大小发生变化")
			}
			if err == nil {
				err = to.Sync()
			}
			closeErr := to.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
			current, statErr := e.FS.Stat(op.Target)
			if statErr != nil || !os.SameFile(opened, current) {
				return errors.New("目标文件在撤销期间发生变化，来源副本需人工检查")
			}
			tempInfo, statErr := e.FS.Stat(tempPath)
			if statErr != nil {
				return statErr
			}
			if err = e.FS.RenameOwned(tempPath, op.Source, fsops.Identity(tempInfo)); err != nil {
				return err
			}
			if err = e.FS.RemoveOwned(op.Target, fsops.Identity(opened)); err != nil {
				return err
			}
			op.Undo = nil
			op.Status, op.Error = "undone", ""
			return save()
		}
	}
	op.Status = "undoing"
	if err = save(); err != nil {
		return err
	}
	if op.Mode == "move" {
		err = e.FS.RenameOwned(op.Target, op.Source, fsops.Identity(info))
	} else {
		err = e.FS.RemoveOwned(op.Target, fsops.Identity(info))
	}
	if err != nil {
		return err
	}
	op.Status, op.Error = "undone", ""
	return save()
}
