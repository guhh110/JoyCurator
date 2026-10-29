package planning

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"joycurator/internal/infra/scanner"
	"joycurator/internal/matching"
	"joycurator/internal/metadata"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/rules"
)

func (a *Service) Preview(ctx context.Context, in PreviewRequest) (*Plan, error) {
	plan, err := a.buildPreview(ctx, in)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.storePlan(plan)
	return plan, nil
}
func (a *Service) buildPreview(ctx context.Context, in PreviewRequest) (*Plan, error) {
	if len(in.Files) == 0 || len(in.Files) > 1000 {
		return nil, errors.New("每次整理 1–1000 个文件")
	}
	if in.Mode != "copy" && in.Mode != "move" && in.Mode != "hardlink" {
		return nil, errors.New("整理方式无效")
	}
	if in.Type != "movie" && in.Type != "tv" || in.TMDBID <= 0 {
		return nil, errors.New("请先选择作品")
	}
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	for _, file := range in.Files {
		if !a.sources.SourceAllowed(file.Path, s) {
			return nil, errors.New("预览包含源目录之外或目标目录内的文件，请重新添加")
		}
	}
	resolved, err := a.media.Prepare(ctx, s, in.Type, in.TMDBID)
	if err != nil {
		return nil, err
	}
	entry := resolved.Entry
	plan := &Plan{ID: model.Key(), Title: entry.Title, Type: in.Type, TMDBID: in.TMDBID, Mode: in.Mode, Rows: []Row{}, Issues: []string{}, Version: s.Version, Expires: time.Now().Add(15 * time.Minute)}
	seen := map[string]model.Operation{}
	free := map[uint64]int64{}
	needed := map[uint64]int64{}
	files := map[string]bool{}
	add := func(row *Row, op model.Operation) {
		issue := func(msg string) { row.Issues = append(row.Issues, msg); plan.Issues = append(plan.Issues, msg) }
		targetKey := strings.ToLower(op.Target)
		if old, ok := seen[targetKey]; ok {
			if op.Kind == "nfo" && old.Kind == op.Kind && bytes.Equal(old.Content, op.Content) || op.Kind == "image" && old.Kind == op.Kind && old.URL == op.URL {
				return
			}
			issue("目标重复：" + op.Target)
			return
		}
		seen[targetKey] = op
		if _, e := a.access.Stat(op.Target); e == nil {
			if op.Kind == "nfo" || op.Kind == "image" {
				row.Artifacts = append(row.Artifacts, "保留已有 "+filepath.Base(op.Target))
				if owned, err := a.ownership.Owned(op.Target); err != nil {
					issue(err.Error())
				} else if owned != nil {
					owned.Owner = row.Source
					owned.Reused = true
					plan.Ops = append(plan.Ops, *owned)
				}
				return
			}
			issue("目标已存在：" + op.Target)
			return
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
			if in.Mode == "hardlink" && !same {
				issue("硬链接不能跨磁盘：" + filepath.Base(op.Source))
				return
			}
			if in.Mode == "hardlink" || in.Mode == "move" && same {
				need = 0
			}
		}
		needed[dev] += need
		plan.Required += need
		op.Status = "pending"
		plan.Ops = append(plan.Ops, model.Work{Operation: op, Owner: row.Source})
		if op.Kind != "video" {
			row.Artifacts = append(row.Artifacts, filepath.Base(op.Target))
		}
	}
	for _, f := range in.Files {
		if files[f.Path] {
			continue
		}
		files[f.Path] = true
		row := Row{Source: f.Path, Season: f.Season, Episodes: f.Episodes, Artifacts: []string{}, Issues: []string{}}
		fail := func(msg string) { row.Issues = append(row.Issues, msg); plan.Issues = append(plan.Issues, msg) }
		info, e := a.access.Stat(f.Path)
		if e != nil || !info.Mode().IsRegular() || info.Size() != f.Size || info.ModTime().UnixNano() != f.MTime {
			fail("源文件已变化，请重新添加：" + filepath.Base(f.Path))
			plan.Rows = append(plan.Rows, row)
			continue
		}
		if in.Type == "tv" {
			if !matching.EpisodeInputValid(f.Season, f.Episodes) {
				fail("季集无效：" + filepath.Base(f.Path))
				plan.Rows = append(plan.Rows, row)
				continue
			}
			checks, err := resolved.CheckEpisodes(ctx, f.Season, f.Episodes)
			if err != nil {
				return nil, err
			}
			for _, check := range checks {
				if !check.Exists || check.Duplicate {
					fail(fmt.Sprintf("第 %d 季没有有效的第 %d 集", f.Season, check.Number))
				}
			}

		}
		extracted, parseErr := a.extract.Parse(ctx, f.Path, s.SourceRoot)
		if parseErr != nil {
			return nil, parseErr
		}
		src := model.Source{Path: f.Path, Parsed: recognition.ParsedMetadata(extracted.Metadata), Probe: scanner.Probe(ctx, a.access, f.Path), Entry: &entry, Season: f.Season, Episodes: f.Episodes}
		target, e := rules.Target(src, s)
		if e != nil {
			fail(e.Error())
			plan.Rows = append(plan.Rows, row)
			continue
		}
		row.Target = target
		add(&row, model.Operation{Kind: "video", Mode: in.Mode, Source: f.Path, Target: target, Size: f.Size, MTime: f.MTime})
		// Associate only subtitles with the exact video stem plus optional language/flags.
		root, rel, e := a.access.Root(filepath.Dir(f.Path))
		if e != nil {
			return nil, e
		}
		entries, e := root.Open(rel)
		if e != nil {
			root.Close()
			return nil, e
		}
		children, e := entries.ReadDir(-1)
		entries.Close()
		root.Close()
		if e != nil {
			return nil, e
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
			add(&row, model.Operation{Kind: "subtitle", Mode: in.Mode, Source: path, Target: targetStem + strings.TrimPrefix(base, stem) + ext, Size: stat.Size(), MTime: stat.ModTime().UnixNano()})
		}
		if s.NFO {
			var body []byte
			if entry.Type == "movie" {
				body, e = metadata.MovieNFO(entry)
			} else {
				body, e = metadata.EpisodeNFO(entry, f.Season, f.Episodes)
			}
			if e != nil {
				fail(e.Error())
			} else {
				add(&row, model.Operation{Kind: "nfo", Mode: "copy", Target: targetStem + ".nfo", Content: body})
			}
		}
		dir := filepath.Dir(target)
		if entry.Type == "tv" {
			dir, e = rules.ShowRoot(src, s)
			if e != nil {
				fail(e.Error())
			} else if s.NFO {
				body, e := metadata.ShowNFO(entry)
				if e != nil {
					fail(e.Error())
				} else {
					add(&row, model.Operation{Kind: "nfo", Mode: "copy", Target: filepath.Join(dir, "tvshow.nfo"), Content: body})
				}
			}
		}
		if s.Images && dir != "" {
			for name, path := range map[string]string{"poster.jpg": entry.PosterPath, "fanart.jpg": entry.BackdropPath} {
				if path != "" {
					add(&row, model.Operation{Kind: "image", Mode: "copy", Target: filepath.Join(dir, name), URL: path})
				}
			}
		}
		plan.Rows = append(plan.Rows, row)
	}
	for dev, n := range needed {
		if n > free[dev] {
			plan.Issues = append(plan.Issues, "目标磁盘空间不足")
		}
	}
	return plan, nil
}

// storePlan is called under mu. Child previews are never independently cached.
