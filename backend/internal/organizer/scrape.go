package organizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/metadata"
	"joycurator/internal/model"
)

func withinMedia(root, path string) bool { return fsops.Within(root, path) }

func (a *Service) validateSkipped(mappings []model.MediaMapping, ops []model.Work) error {
	active := map[string]bool{}
	for _, op := range ops {
		if op.Kind == "video" {
			active[op.Target] = true
		}
	}
	for _, item := range mappings {
		if active[item.Path] || item.Identity == "" {
			continue
		}
		info, err := a.access.Stat(item.Path)
		if err != nil || !info.Mode().IsRegular() || fsops.Identity(info) != item.Identity || info.Size() != item.Size || info.ModTime().UnixNano() != item.MTime {
			return errors.New("跳过的视频已变化，请重新预览：" + item.Path)
		}
	}
	return nil
}

func (a *Service) scrapeQueue() ([]model.MediaMapping, error) {
	if a.mappings == nil {
		return nil, errors.New("刮削工作区未配置")
	}
	items, err := a.mappings.Mappings()
	if err != nil {
		return nil, err
	}
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if !withinMedia(s.TargetRoot, items[i].Path) {
			items[i].State = "unavailable"
			continue
		}
		info, e := a.access.Stat(items[i].Path)
		if e != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && (items[i].Identity == "" || fsops.Identity(info) != items[i].Identity)) || info.Size() != items[i].Size || info.ModTime().UnixNano() != items[i].MTime {
			items[i].State = "unavailable"
		}
	}
	return items, nil
}

func (a *Service) ScrapePreview(ctx context.Context, selected []string) (ScrapePlan, error) {
	if len(selected) == 0 || len(selected) > 1000 {
		return ScrapePlan{}, errors.New("请选择 1–1000 个目标视频")
	}
	s, err := a.settings.Load()
	if err != nil {
		return ScrapePlan{}, err
	}
	available, err := a.scrapeQueue()
	if err != nil {
		return ScrapePlan{}, err
	}
	known := make(map[string]model.MediaMapping, len(available))
	for _, item := range available {
		known[item.Path] = item
	}
	items := make([]model.MediaMapping, 0, len(selected))
	seenPaths := map[string]bool{}
	for _, path := range selected {
		item, exists := known[path]
		if !exists || item.State == "unavailable" || seenPaths[path] {
			return ScrapePlan{}, errors.New("目标视频不可用或未登记：" + path)
		}
		seenPaths[path] = true
		items = append(items, item)
	}
	kind, id := items[0].Type, items[0].TMDBID
	for _, item := range items {
		if item.Type != kind || item.TMDBID != id {
			return ScrapePlan{}, errors.New("每次只刮削一部作品")
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	provider, err := a.factory(s, nil)
	if err != nil {
		return ScrapePlan{}, err
	}
	entry, err := provider.Detail(ctx, kind, id)
	if err != nil {
		return ScrapePlan{}, err
	}
	plan := ScrapePlan{ID: model.Key(), Title: entry.Title, Type: kind, TMDBID: id, Targets: selected, Artifacts: []string{}, Issues: []string{}}
	ops := []model.Work{}
	seen := map[string]model.Operation{}
	remaining := map[uint64]int64{}
	add := func(target, kind, url string, content []byte, owner string) {
		key := strings.ToLower(target)
		op := model.Operation{Target: target, Kind: kind, Mode: "copy", URL: url, Content: content, Status: "pending"}
		if kind == "image" && strings.HasSuffix(strings.ToLower(target), ".jpg") && strings.HasSuffix(strings.ToLower(url), ".png") {
			op.ConvertJPEG = true
		}
		if prev, ok := seen[key]; ok {
			if prev.Target != target || prev.Kind != kind || prev.URL != url || !bytes.Equal(prev.Content, content) {
				plan.Issues = append(plan.Issues, "刮削目标重复："+target)
			}
			return
		}
		seen[key] = op
		if !withinMedia(s.TargetRoot, target) {
			plan.Issues = append(plan.Issues, "目标越界："+target)
			return
		}
		info, statErr := a.access.Stat(target)
		if statErr == nil {
			if !info.Mode().IsRegular() {
				plan.Issues = append(plan.Issues, "目标不是普通文件："+target)
				return
			}
			op.Replace = true
			op.ReplaceIdentity, op.ReplaceSize, op.ReplaceMTime = fsops.Identity(info), info.Size(), info.ModTime().UnixNano()
		} else if !errors.Is(statErr, os.ErrNotExist) {
			plan.Issues = append(plan.Issues, statErr.Error())
			return
		}
		free, device, checkErr := a.access.CheckTarget(target)
		if checkErr != nil {
			plan.Issues = append(plan.Issues, checkErr.Error())
			return
		}
		need := int64(len(content))
		if kind == "image" {
			need = 30 << 20
		}
		plan.Required += need
		remaining[device] += need
		if remaining[device] > free {
			plan.Issues = append(plan.Issues, "目标空间不足")
		}
		op.ID = int64(len(ops) + 1)
		ops = append(ops, model.Work{Operation: op, Owner: owner})
		plan.Artifacts = append(plan.Artifacts, target)
	}
	showDirs := map[string]string{}
	for _, item := range items {
		dir := filepath.Dir(item.Path)
		if kind == "tv" {
			dir = filepath.Dir(dir)
		}
		if _, exists := showDirs[dir]; !exists {
			showDirs[dir] = item.Path
		}
	}
	dirs := make([]string, 0, len(showDirs))
	for dir := range showDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	if kind == "movie" {
		body, e := metadata.MovieNFO(entry)
		if e != nil {
			return plan, e
		}
		for _, item := range items {
			add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".nfo", "nfo", "", body, item.Path)
		}
	} else {
		body, e := metadata.ShowNFO(entry)
		if e != nil {
			return plan, e
		}
		for _, dir := range dirs {
			add(filepath.Join(dir, "tvshow.nfo"), "nfo", "", body, showDirs[dir])
		}
		seasons := map[int]bool{}
		for _, item := range items {
			seasons[item.Season] = true
		}
		seasonNumbers := make([]int, 0, len(seasons))
		for season := range seasons {
			seasonNumbers = append(seasonNumbers, season)
		}
		sort.Ints(seasonNumbers)
		for _, season := range seasonNumbers {
			seasonInfo, episodes, e := provider.SeasonDetail(ctx, id, season)
			if e != nil {
				return plan, e
			}
			byNumber := map[int]model.Episode{}
			for _, episode := range episodes {
				byNumber[episode.Number] = episode
			}
			for _, item := range items {
				if item.Season != season {
					continue
				}
				matched := make([]model.Episode, 0, len(item.Episodes))
				for _, number := range item.Episodes {
					episode, exists := byNumber[number]
					if !exists {
						return plan, fmt.Errorf("第 %d 季第 %d 集已不存在", season, number)
					}
					matched = append(matched, episode)
				}
				body, e := metadata.EpisodeNFO(entry, matched)
				if e != nil {
					return plan, e
				}
				add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".nfo", "nfo", "", body, item.Path)
				for _, episode := range matched {
					ext := strings.ToLower(filepath.Ext(episode.Still))
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".jpg", "image", episode.Still, nil, item.Path)
						break
					}
				}
			}
			body, e := metadata.SeasonNFO(entry, seasonInfo)
			if e != nil {
				return plan, e
			}
			seasonDirs := map[string]string{}
			for _, item := range items {
				if item.Season == season {
					dir := filepath.Dir(item.Path)
					if _, ok := seasonDirs[dir]; !ok {
						seasonDirs[dir] = item.Path
					}
				}
			}
			ordered := make([]string, 0, len(seasonDirs))
			for dir := range seasonDirs {
				ordered = append(ordered, dir)
			}
			sort.Strings(ordered)
			for _, dir := range ordered {
				add(filepath.Join(dir, "season.nfo"), "nfo", "", body, seasonDirs[dir])
				if seasonInfo.PosterPath != "" {
					ext := strings.ToLower(filepath.Ext(seasonInfo.PosterPath))
					if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
						add(filepath.Join(dir, fmt.Sprintf("season%02d-poster%s", season, ext)), "image", seasonInfo.PosterPath, nil, seasonDirs[dir])
					}
				}
			}
		}
	}
	art, err := provider.Artwork(ctx, kind, id, s.Language)
	if err != nil {
		return plan, err
	}
	best := selectArtwork(art, s.Language)
	for _, kind := range []string{"poster", "fanart", "clearlogo"} {
		image, ok := best[kind]
		if !ok {
			continue
		}
		ext := strings.ToLower(filepath.Ext(image.Path))
		name := kind + ".jpg"
		if kind == "clearlogo" && ext == ".png" {
			name = "clearlogo.png"
		}
		for _, dir := range dirs {
			add(filepath.Join(dir, name), "image", image.Path, nil, showDirs[dir])
		}
	}
	for i := range ops {
		op := &ops[i]
		for _, item := range items {
			sharedDir := filepath.Dir(item.Path)
			if kind == "tv" {
				sharedDir = filepath.Dir(sharedDir)
			}
			shared := filepath.Dir(op.Target) == sharedDir
			if kind == "tv" && filepath.Dir(op.Target) == filepath.Dir(item.Path) {
				base := filepath.Base(op.Target)
				shared = base == "season.nfo" || strings.HasPrefix(base, fmt.Sprintf("season%02d-poster.", item.Season))
			}
			if op.Owner == item.Path || shared {
				op.Owners = append(op.Owners, item.Path)
			}
		}
	}
	sort.Strings(plan.Artifacts)
	a.scrapeMu.Lock()
	for key, old := range a.scrapePlans {
		if time.Now().After(old.expires) {
			delete(a.scrapePlans, key)
		}
	}
	a.scrapePlans[plan.ID] = scrapePreview{plan: plan, ops: ops, version: s.Version, expires: time.Now().Add(15 * time.Minute), mappings: items}
	a.scrapeMu.Unlock()
	return plan, nil
}

func selectArtwork(candidates []model.Artwork, language string) map[string]model.Artwork {
	primary := strings.SplitN(language, "-", 2)[0]
	rank := func(value string) int {
		switch {
		case value == language || value == primary:
			return 0
		case value == "":
			return 1
		case value == "en":
			return 2
		default:
			return 3
		}
	}
	best := map[string]model.Artwork{}
	for _, image := range candidates {
		if image.Kind != "poster" && image.Kind != "fanart" && image.Kind != "clearlogo" || rank(image.Language) == 3 {
			continue
		}
		ext := strings.ToLower(filepath.Ext(image.Path))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
			continue
		}
		previous, exists := best[image.Kind]
		if !exists || rank(image.Language) < rank(previous.Language) || rank(image.Language) == rank(previous.Language) &&
			(image.Score > previous.Score || image.Score == previous.Score && (image.Votes > previous.Votes || image.Votes == previous.Votes && image.Path < previous.Path)) {
			best[image.Kind] = image
		}
	}
	return best
}

func (a *Service) ScrapeExecute(id string) (Run, error) {
	if runtime.GOOS == "windows" {
		return Run{}, errors.New("Windows 暂不支持执行刮削")
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.coordinator.closed || a.coordinator.running != "" || a.coordinator.batch != "" {
		return Run{}, ErrBusy
	}
	a.scrapeMu.Lock()
	p, ok := a.scrapePlans[id]
	a.scrapeMu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return Run{}, errors.New("刮削预览已过期")
	}
	if len(p.plan.Issues) > 0 || len(p.ops) == 0 {
		return Run{}, errors.New("刮削预览有冲突")
	}
	s, e := a.settings.Load()
	if e != nil {
		return Run{}, e
	}
	if s.Version != p.version {
		return Run{}, errors.New("设置已更新，请重新预览")
	}
	for _, item := range p.mappings {
		if !withinMedia(s.TargetRoot, item.Path) {
			return Run{}, errors.New("目标视频已变化，请重新预览")
		}
		info, e := a.access.Stat(item.Path)
		if e != nil || !info.Mode().IsRegular() || fsops.Identity(info) != item.Identity || info.Size() != item.Size || info.ModTime().UnixNano() != item.MTime {
			return Run{}, errors.New("目标视频已变化，请重新预览")
		}
	}
	currentMappings, e := a.mappings.Mappings()
	if e != nil {
		return Run{}, e
	}
	byPath := make(map[string]model.MediaMapping, len(currentMappings))
	for _, mapping := range currentMappings {
		byPath[mapping.Path] = mapping
	}
	for _, mapping := range p.mappings {
		current, ok := byPath[mapping.Path]
		if !ok || current.RunID != mapping.RunID || current.Identity != mapping.Identity || current.TMDBID != mapping.TMDBID || current.Type != mapping.Type || current.Season != mapping.Season || !slices.Equal(current.Episodes, mapping.Episodes) {
			return Run{}, errors.New("刮削映射已变化，请重新预览")
		}
	}
	for _, op := range p.ops {
		info, e := a.access.Stat(op.Target)
		if op.Replace {
			if e != nil || !info.Mode().IsRegular() || fsops.Identity(info) != op.ReplaceIdentity || info.Size() != op.ReplaceSize || info.ModTime().UnixNano() != op.ReplaceMTime {
				return Run{}, errors.New("刮削目标已变化，请重新预览")
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return Run{}, errors.New("刮削目标已变化，请重新预览")
		}
	}
	r := Run{ID: model.Key(), Title: p.plan.Title, Mode: "scrape", State: "running", Created: time.Now().Format(time.RFC3339), Ops: p.ops, Total: len(p.ops), Purpose: "scrape", Mappings: p.mappings, Version: p.version}
	result, err := a.launch(r)
	if err == nil {
		a.scrapeMu.Lock()
		delete(a.scrapePlans, id)
		a.scrapeMu.Unlock()
	}
	return result, err
}
