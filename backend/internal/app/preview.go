package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"joycurator/internal/fsops"
	"joycurator/internal/metadata"
	"joycurator/internal/model"
	"joycurator/internal/organizer"
	"joycurator/internal/rules"
	"joycurator/internal/scanner"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type File struct {
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
	MTime    int64       `json:"mtime,string"`
	Season   int         `json:"season"`
	Episodes []int       `json:"episodes"`
	Probe    model.Probe `json:"probe"`
}
type PreviewRequest struct {
	Type   string `json:"type"`
	TMDBID int    `json:"tmdbId"`
	Mode   string `json:"mode"`
	Files  []File `json:"files"`
}

func (a *App) scan(ctx context.Context, paths []string) ([]File, error) {
	if len(paths) == 0 || len(paths) > 200 {
		return nil, errors.New("请选择 1–200 个文件或目录")
	}
	s, err := a.settings()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if !a.sourceAllowed(path, s) {
			return nil, errors.New("只能添加源目录内的文件或文件夹，目标目录不能作为来源")
		}
	}
	items := []File{}
	err = scanner.Walk(ctx, a.access, paths, []string{s.TargetRoot}, func(path string, info os.FileInfo) error {
		if len(items) >= 2000 {
			return errors.New("单次最多导入 2000 个视频，请分批添加")
		}
		extracted, _, extractErr := a.extractFile(ctx, path, s.SourceRoot, "")
		if extractErr != nil {
			return extractErr
		}
		p := parsedMetadata(extracted.Metadata)
		items = append(items, File{Path: path, Size: info.Size(), MTime: info.ModTime().UnixNano(), Season: p.Season, Episodes: p.Episodes, Probe: scanner.Probe(ctx, a.access, path)})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, err
}
func (a *App) preview(ctx context.Context, in PreviewRequest) (*Plan, error) {
	plan, err := a.buildPreview(ctx, in)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.storePlan(plan)
	return plan, nil
}

func (a *App) buildPreview(ctx context.Context, in PreviewRequest) (*Plan, error) {
	if len(in.Files) == 0 || len(in.Files) > 1000 {
		return nil, errors.New("每次整理 1–1000 个文件")
	}
	if in.Mode != "copy" && in.Mode != "move" && in.Mode != "hardlink" {
		return nil, errors.New("整理方式无效")
	}
	if in.Type != "movie" && in.Type != "tv" || in.TMDBID <= 0 {
		return nil, errors.New("请先选择作品")
	}
	s, err := a.settings()
	if err != nil {
		return nil, err
	}
	for _, file := range in.Files {
		if !a.sourceAllowed(file.Path, s) {
			return nil, errors.New("预览包含源目录之外或目标目录内的文件，请重新添加")
		}
	}
	client, err := a.client(s)
	if err != nil {
		return nil, err
	}
	entry, err := client.Detail(ctx, in.Type, in.TMDBID)
	if err != nil {
		return nil, err
	}
	plan := &Plan{ID: key(), Title: entry.Title, Type: in.Type, TMDBID: in.TMDBID, Mode: in.Mode, Rows: []Row{}, Issues: []string{}, Version: s.Version, Expires: time.Now().Add(15 * time.Minute)}
	seen := map[string]model.Operation{}
	free := map[uint64]int64{}
	needed := map[uint64]int64{}
	files := map[string]bool{}
	episodeSets := map[int]map[int]bool{}
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
				if owned, err := a.owned(op.Target); err != nil {
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
		plan.Ops = append(plan.Ops, Work{Operation: op, Owner: row.Source})
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
			if f.Season < 0 || f.Season > 999 || len(f.Episodes) == 0 || len(f.Episodes) > 1000 {
				fail("季集无效：" + filepath.Base(f.Path))
				plan.Rows = append(plan.Rows, row)
				continue
			}
			if episodeSets[f.Season] == nil {
				eps, e := client.SeasonEpisodes(ctx, in.TMDBID, f.Season)
				if e != nil {
					return nil, e
				}
				episodeSets[f.Season] = map[int]bool{}
				for _, ep := range eps {
					episodeSets[f.Season][ep.Number] = true
				}
			}
			unique := map[int]bool{}
			for _, n := range f.Episodes {
				if !episodeSets[f.Season][n] || unique[n] {
					fail(fmt.Sprintf("第 %d 季没有有效的第 %d 集", f.Season, n))
				}
				unique[n] = true
			}
		}
		parsed, parseErr := a.parseForConsumer(ctx, f.Path, s.SourceRoot)
		if parseErr != nil {
			return nil, parseErr
		}
		src := model.Source{Path: f.Path, Parsed: parsed, Probe: scanner.Probe(ctx, a.access, f.Path), Entry: &entry, Season: f.Season, Episodes: f.Episodes}
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
func (a *App) storePlan(plan *Plan) {
	for id, p := range a.plans {
		if time.Now().After(p.Expires) {
			delete(a.plans, id)
		}
	}
	if len(a.plans) >= 32 {
		a.plans = map[string]*Plan{}
	}
	a.plans[plan.ID] = plan
}

func (a *App) validateOps(ops []Work) error {
	needed := map[uint64]int64{}
	free := map[uint64]int64{}
	for _, op := range ops {
		if op.Reused {
			engine := organizer.Engine{FS: a.access}
			if err := engine.Check(context.Background(), op.Target, op.Identity, op.Hash); err != nil {
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
		if _, err := a.access.Stat(op.Target); err == nil {
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

func (a *App) sourceAllowed(path string, s model.Settings) bool {
	path = a.access.Canonical(path)
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return false
	}
	return s.SourceRoot != "" && fsops.Within(a.access.Canonical(s.SourceRoot), path) && (s.TargetRoot == "" || !fsops.Within(a.access.Canonical(s.TargetRoot), path))
}
