package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/rules"
)

func (a *Service) buildPreview(ctx context.Context, items []matching.Item, mode, policy string, s model.Settings) (*Plan, error) {
	if len(items) == 0 || len(items) > 1000 {
		return nil, errors.New("每次整理 1–1000 个文件")
	}
	if mode != "copy" && mode != "move" && mode != "hardlink" {
		return nil, errors.New("整理方式无效")
	}
	if policy != "error" && policy != "skip" && policy != "overwrite" {
		return nil, errors.New("重复处理方式无效")
	}
	entry := *items[0].Entry
	if entry.Type != "movie" && entry.Type != "tv" || entry.TMDBID <= 0 {
		return nil, errors.New("请先选择作品")
	}
	for _, item := range items {
		if !item.Confirmed() || item.State != "ready" || item.Removed || item.Entry == nil || matching.EntryGroup(*item.Entry) != matching.EntryGroup(entry) || !a.sources.SourceAllowed(item.Path, s) {
			return nil, errors.New("预览包含源目录之外或目标目录内的文件，请重新添加")
		}
	}
	plan := &Plan{ID: model.Key(), Title: entry.Title, Type: entry.Type, TMDBID: entry.TMDBID, Mode: mode, DuplicatePolicy: policy, Rows: []Row{}, Issues: []string{}, Version: s.Version, Expires: time.Now().Add(previewTTL), Purpose: "organize"}
	seen := map[string]Operation{}
	free := map[uint64]int64{}
	needed := map[uint64]int64{}
	files := map[string]bool{}
	subtitles := map[string][]os.DirEntry{}
	add := func(row *Row, op Operation) {
		issue := func(msg string) { row.Issues = append(row.Issues, msg); plan.Issues = append(plan.Issues, msg) }
		targetKey := strings.ToLower(op.Target)
		if _, ok := seen[targetKey]; ok {
			issue("目标重复：" + op.Target)
			return
		}
		seen[targetKey] = op
		if existing, e := a.access.Stat(op.Target); e == nil {
			if !existing.Mode().IsRegular() {
				issue("目标不是普通文件，不能跳过或覆盖：" + op.Target)
				return
			}
			switch policy {
			case "skip":
				if op.Kind == "video" {
					row.Action = "skip"
				}
				row.Artifacts = append(row.Artifacts, "跳过已有 "+filepath.Base(op.Target))
				return
			case "overwrite":
				row.Action = "overwrite"
				op.Replace = true
				op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime = fsops.Identity(existing), existing.Size(), existing.ModTime().UnixNano()
				if op.Kind != "video" {
					row.Artifacts = append(row.Artifacts, "覆盖已有 "+filepath.Base(op.Target))
				}
			default:
				issue("目标已存在：" + op.Target)
				return
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			issue(e.Error())
			return
		}
		avail, dev, e := a.access.CheckTarget(op.Target)
		if e != nil {
			issue(e.Error())
			return
		}
		free[dev] = avail
		need := int64(len(op.Content))
		if op.Kind == "image" {
			need = 30 << 20
		}
		if op.Source != "" {
			need = op.Size
			info, e := a.access.Stat(op.Source)
			if e != nil {
				issue(e.Error())
				return
			}
			same := a.access.Device(info) == dev
			if mode == "hardlink" && !same {
				issue("硬链接不能跨磁盘：" + filepath.Base(op.Source))
				return
			}
			if mode == "hardlink" || mode == "move" && same {
				need = 0
			}
		}
		needed[dev] += need
		plan.Required += need
		op.Status = "pending"
		plan.Ops = append(plan.Ops, Work{Operation: op, Owner: row.Source})
		if op.Kind != "video" {
			row.Artifacts = append(row.Artifacts, filepath.Base(op.Target))
		}
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f := item.File
		if files[f.Path] {
			continue
		}
		files[f.Path] = true
		row := Row{Source: f.Path, Season: f.Season, Episodes: f.Episodes, Artifacts: []string{}, Issues: []string{}, Action: "create"}
		fail := func(msg string) { row.Issues = append(row.Issues, msg); plan.Issues = append(plan.Issues, msg) }
		info, e := a.access.Stat(f.Path)
		if e != nil || !info.Mode().IsRegular() || info.Size() != f.Size || info.ModTime().UnixNano() != f.MTime || fsops.Identity(info)!=item.Identity {
			fail("源文件已变化，请重新添加：" + filepath.Base(f.Path))
			plan.Rows = append(plan.Rows, row)
			continue
		}
		if entry.Type == "tv" {
			if !matching.EpisodeInputValid(f.Season, f.Episodes) {
				fail("季集无效：" + filepath.Base(f.Path))
				plan.Rows = append(plan.Rows, row)
				continue
			}
		}
		src := model.Source{Path: f.Path, Metadata: item.Metadata, Entry: item.Entry, Season: f.Season, Episodes: f.Episodes}
		target, e := rules.Target(src, s)
		if e != nil {
			fail(e.Error())
			plan.Rows = append(plan.Rows, row)
			continue
		}
		row.Target = target
		mapping := model.MediaMapping{Path: target, Source: f.Path, Type: entry.Type, TMDBID: entry.TMDBID, Season: f.Season, Episodes: f.Episodes, State: "pending"}
		add(&row, Operation{Kind: "video", Mode: mode, Source: f.Path, Target: target, Size: f.Size, MTime: f.MTime, SourceIdentity: fsops.Identity(info)})
		if row.Action == "skip" {
			if existing, err := a.access.Stat(target); err == nil {
				mapping.Identity, mapping.Size, mapping.MTime = fsops.Identity(existing), existing.Size(), existing.ModTime().UnixNano()
			}
			plan.Mappings = append(plan.Mappings, mapping)
			plan.Rows = append(plan.Rows, row)
			continue
		}
		plan.Mappings = append(plan.Mappings, mapping)
		// Associate only subtitles with the exact video stem plus optional language/flags.
		dir := filepath.Dir(f.Path)
		children, ok := subtitles[dir]
		if !ok {
			root, rel, e := a.access.Root(dir)
			if e != nil {
				return nil, e
			}
			entries, openErr := root.Open(rel)
			if openErr != nil {
				root.Close()
				return nil, openErr
			}
			children, e = entries.ReadDir(-1)
			entries.Close()
			root.Close()
			if e != nil {
				return nil, e
			}
			subtitles[dir] = children
		}
		stem := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
		targetStem := strings.TrimSuffix(target, filepath.Ext(target))
		for _, child := range children {
			ext := strings.ToLower(filepath.Ext(child.Name()))
			if !strings.Contains("|.srt|.ass|.ssa|.vtt|.sub|.idx|", "|"+ext+"|") {
				continue
			}
			base := strings.TrimSuffix(child.Name(), filepath.Ext(child.Name()))
			if base != stem && !strings.HasPrefix(base, stem+".") {
				continue
			}
			path := filepath.Join(filepath.Dir(f.Path), child.Name())
			stat, e := a.access.Stat(path)
			if e != nil || !stat.Mode().IsRegular() {
				continue
			}
			if ext == ".sub" || ext == ".idx" {
				other := ".sub"
				if ext == ".sub" {
					other = ".idx"
				}
				if _, e = a.access.Stat(strings.TrimSuffix(path, filepath.Ext(path)) + other); e != nil {
					fail("IDX/SUB 字幕必须成对：" + child.Name())
					continue
				}
			}
			add(&row, Operation{Kind: "subtitle", Mode: mode, Source: path, Target: targetStem + strings.TrimPrefix(base, stem) + ext, Size: stat.Size(), MTime: stat.ModTime().UnixNano(), SourceIdentity: fsops.Identity(stat)})
		}
		plan.Rows = append(plan.Rows, row)
	}
	for dev, n := range needed {
		if n > free[dev] {
			plan.Issues = append(plan.Issues, "目标磁盘空间不足")
		}
	}
	inferMappingLayouts(plan.Mappings, s.TargetRoot)
	return plan, nil
}

// Infer layouts from actual target paths, including custom naming rules.

func inferMappingLayouts(mappings []model.MediaMapping, target string) {
	items := make([]matching.Item, len(mappings))
	for i, m := range mappings {
		items[i] = matching.Item{File: model.File{Path: m.Path, Season: m.Season, Episodes: m.Episodes}, Layout: m.Layout, Entry: &model.Entry{Type: m.Type, TMDBID: m.TMDBID}}
	}
	matching.InferLayouts(items, target)
	for i := range mappings {
		mappings[i].Layout = items[i].Layout
	}
}
