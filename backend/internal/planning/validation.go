package planning

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/model"
)

func (a *Service) ValidateOps(ops []model.Work) error {
	needed := map[uint64]int64{}
	free := map[uint64]int64{}
	for _, op := range ops {
		if op.Reused {
			if err := a.verifier.Check(context.Background(), op.Target, op.Identity, op.Hash); err != nil {
				return err
			}
			continue
		}
		if op.Source != "" {
			info, err := a.access.Stat(op.Source)
			if err != nil || !info.Mode().IsRegular() || info.Size() != op.Size || info.ModTime().UnixNano() != op.MTime {
				return errors.New("文件已变化，请重新预览：" + filepath.Base(op.Source))
			}
		}
		if op.Replace {
			info, err := a.access.Stat(op.Target)
			if err != nil || !info.Mode().IsRegular() || info.Size() != op.ReplaceSize || info.ModTime().UnixNano() != op.ReplaceMTime || fsops.Identity(info) != op.ReplaceIdentity {
				return errors.New("待覆盖文件已变化，请重新预览：" + op.Target)
			}
		} else if _, err := a.access.Stat(op.Target); err == nil {
			return errors.New("目标已存在，请重新预览：" + op.Target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		avail, dev, err := a.access.CheckTarget(op.Target)
		if err != nil {
			return err
		}
		free[dev] = avail
		n := int64(len(op.Content))
		if op.Kind == "image" {
			n = 30 << 20
		}
		if op.Source != "" {
			n = op.Size
			info, err := a.access.Stat(op.Source)
			if err != nil {
				return err
			}
			same := a.access.Device(info) == dev
			if op.Mode == "hardlink" && !same {
				return errors.New("硬链接不能跨磁盘")
			}
			if op.Mode == "hardlink" || op.Mode == "move" && same {
				n = 0
			}
		}
		needed[dev] += n
	}
	for dev, n := range needed {
		if n > free[dev] {
			return errors.New("目标磁盘空间不足")
		}
	}
	return nil
}
