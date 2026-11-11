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
	"joycurator/internal/matching"
	"joycurator/internal/metadata"
	"joycurator/internal/model"
)

func withinMedia(root, path string) bool { return fsops.Within(root, path) }

func (a *Service) validateSkipped(mappings []model.MediaMapping, ops []Work) error {
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

func (a *Service) checkScrapeAvailability(items []model.MediaMapping) ([]model.MediaMapping, error) {
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

func (a *Service) ScrapePreview(ctx context.Context, input ScrapePreviewRequest) (ScrapePlan, error) {
	selected := input.Paths
	if input.Content != "all" && input.Content != "nfo" && input.Content != "images" {
		return ScrapePlan{}, errors.New("请选择刮削内容：all、nfo 或 images")
	}
	if len(selected) == 0 || len(selected) > 1000 {
		return ScrapePlan{}, errors.New("请选择 1–1000 个目标视频")
	}
	items := make([]model.MediaMapping, 0, len(selected))
	seenPaths := map[string]bool{}
	for _, path := range selected {
		if err := ctx.Err(); err != nil {
			return ScrapePlan{}, err
		}
		item, err := a.mappings.Mapping(path)
		if err != nil {
			return ScrapePlan{}, err
		}
		if item == nil || seenPaths[path] {
			return ScrapePlan{}, errors.New("目标视频不可用或未登记：" + path)
		}
		seenPaths[path] = true
		items = append(items, *item)
	}
	for i := range items {
		original := items[i]
		items[i].Original = &original
	}
	return a.scrapePreviewItems(ctx, input, items)
}

func (a *Service) scrapePreviewItems(ctx context.Context, input ScrapePreviewRequest, items []model.MediaMapping) (ScrapePlan, error) {
	s, err := a.settings.Load()
	if err != nil {
		return ScrapePlan{}, err
	}
	items, err = a.checkScrapeAvailability(items)
	if err != nil {
		return ScrapePlan{}, err
	}
	for _, item := range items {
		if item.State == "unavailable" {
			return ScrapePlan{}, errors.New("目标视频已变化：" + item.Path)
		}
		if err = matching.ValidateLayout(item.Layout, item.Path, item.Type, s.TargetRoot); err != nil {
			return ScrapePlan{}, err
		}
	}
	selected := input.Paths
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
	plan := ScrapePlan{Content: input.Content, OnlyMissing: input.OnlyMissing, Skipped: []string{}, ID: model.Key(), Title: entry.Title, Type: kind, TMDBID: id, Targets: selected, Artifacts: []string{}, Issues: []string{}}
	ops := []Work{}
	seen := map[string]Operation{}
	remaining := map[uint64]int64{}
	add := func(target, kind, url string, content []byte, owner string) {
		if input.Content == "nfo" && kind != "nfo" || input.Content == "images" && kind != "image" {
			return
		}
		key := strings.ToLower(target)
		op := Operation{Target: target, Kind: kind, Mode: "copy", URL: url, Content: content, Status: "pending"}
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
			if input.OnlyMissing {
				plan.Skipped = append(plan.Skipped, target)
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
		ops = append(ops, Work{Operation: op, Owner: owner})
		plan.Artifacts = append(plan.Artifacts, target)
	}
	showDirs := map[string]string{}
	for _, item := range items {
		dir := item.Layout.WorkDir
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
		if input.Content != "images" {
			body, e := metadata.MovieNFO(entry)
			if e != nil {
				return plan, e
			}
			for _, item := range items {
				add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".nfo", "nfo", "", body, item.Path)
			}
		}
	} else {
		if input.Content != "images" {
			body, e := metadata.ShowNFO(entry)
			if e != nil {
				return plan, e
			}
			for _, dir := range dirs {
				add(filepath.Join(dir, "tvshow.nfo"), "nfo", "", body, showDirs[dir])
			}
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
				if input.Content != "images" {
					body, e := metadata.EpisodeNFO(entry, matched)
					if e != nil {
						return plan, e
					}
					add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".nfo", "nfo", "", body, item.Path)
				}
				for _, episode := range matched {
					ext := strings.ToLower(filepath.Ext(episode.Still))
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
						add(strings.TrimSuffix(item.Path, filepath.Ext(item.Path))+".jpg", "image", episode.Still, nil, item.Path)
						break
					}
				}
			}
			seasonDirs := map[string]string{}
			for _, item := range items {
				if item.Season == season {
					dir := item.Layout.SeasonDir
					if dir == "" {
						dir = item.Layout.WorkDir
					}
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
				flat := true
				for _, item := range items {
					if item.Season == season && item.Layout.SeasonDir == dir {
						flat = false
					}
				}
				if input.Content != "images" && !flat {
					body, e := metadata.SeasonNFO(entry, seasonInfo)
					if e != nil {
						return plan, e
					}
					add(filepath.Join(dir, "season.nfo"), "nfo", "", body, seasonDirs[dir])
				}
				if seasonInfo.PosterPath != "" {
					ext := strings.ToLower(filepath.Ext(seasonInfo.PosterPath))
					if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
						name := "poster.jpg"
						if flat {
							name = fmt.Sprintf("season%02d-poster.jpg", season)
						}
						add(filepath.Join(dir, name), "image", seasonInfo.PosterPath, nil, seasonDirs[dir])
					}
				}
			}
		}
	}
	if input.Content != "nfo" {
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
				if entry.Type == "movie" {
					for _, item := range items {
						if item.Layout.WorkDir == dir {
							add(matching.MovieArtworkPath(item, name), "image", image.Path, nil, item.Path)
						}
					}
				} else {
					add(filepath.Join(dir, name), "image", image.Path, nil, showDirs[dir])
				}
			}
		}
	}
	for i := range ops {
		op := &ops[i]
		for _, item := range items {
			shared := !item.Layout.MoviePrefix && filepath.Dir(op.Target) == item.Layout.WorkDir && (baseSharedArtifact(op.Target))
			base := filepath.Base(op.Target)
			if kind == "tv" && (strings.HasPrefix(base, "season") || base == "tvshow.nfo" || base == "poster.jpg" || base == "fanart.jpg" || base == "clearlogo.png" || base == "clearlogo.jpg") {
				shared = filepath.Dir(op.Target) == item.Layout.WorkDir && (base == "tvshow.nfo" || base == "poster.jpg" || base == "fanart.jpg" || strings.HasPrefix(base, "clearlogo.") || base == fmt.Sprintf("season%02d-poster.jpg", item.Season)) || item.Layout.SeasonDir != "" && filepath.Dir(op.Target) == item.Layout.SeasonDir && (base == "season.nfo" || base == "poster.jpg")
			}

			if op.Owner == item.Path || shared {
				op.Owners = append(op.Owners, item.Path)
			}
		}
	}
	// Videos with no scheduled artifact must keep their previous scrape state.
	affected := []model.MediaMapping{}
	for _, item := range items {
		for _, op := range ops {
			if op.Owner == item.Path || slices.Contains(op.Owners, item.Path) {
				affected = append(affected, item)
				break
			}
		}
	}
	sort.Strings(plan.Skipped)
	sort.Strings(plan.Artifacts)
	a.previews.put(&Plan{ID: plan.ID, Purpose: "scrape", Ops: ops, Inputs: items, Mappings: affected, Version: s.Version,
		Expires: time.Now().Add(previewTTL), Issues: plan.Issues, Title: plan.Title})
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
	p := a.previews.get(id)
	if p == nil || p.Purpose != "scrape" || time.Now().After(p.Expires) {
		return Run{}, errors.New("刮削预览已过期")
	}
	if len(p.Issues) > 0 || len(p.Ops) == 0 {
		return Run{}, errors.New("刮削预览有冲突")
	}
	s, e := a.settings.Load()
	if e != nil {
		return Run{}, e
	}
	if s.Version != p.Version {
		return Run{}, errors.New("设置已更新，请重新预览")
	}
	for _, item := range p.Mappings {
		if !withinMedia(s.TargetRoot, item.Path) {
			return Run{}, errors.New("目标视频已变化，请重新预览")
		}
		info, e := a.access.Stat(item.Path)
		if e != nil || !info.Mode().IsRegular() || fsops.Identity(info) != item.Identity || info.Size() != item.Size || info.ModTime().UnixNano() != item.MTime {
			return Run{}, errors.New("目标视频已变化，请重新预览")
		}
	}
	for _, m := range p.Mappings {
		if err := a.validateScrapeMapping(m, ""); err != nil {
			return Run{}, err
		}
	}
	r := Run{ID: model.Key(), Title: p.Title, Mode: "scrape", State: "queued", Created: time.Now().Format(time.RFC3339), Ops: append([]Work(nil), p.Ops...), Total: len(p.Ops), Purpose: "scrape", Mappings: append([]model.MediaMapping(nil), p.Mappings...), Version: p.Version}
	if e = a.validateScrapeRun(r, s); e != nil {
		return Run{}, e
	}
	a.coordinator.mu.Lock()
	defer a.coordinator.mu.Unlock()
	if a.previews.get(id) != p {
		return Run{}, errors.New("预览已过期或已提交")
	}
	current, e := a.settings.Load()
	if e != nil {
		return Run{}, e
	}
	if current.Version != p.Version {
		return Run{}, errors.New("设置已变化，请重新预览")
	}
	if e = a.validateScrapeRun(Run{Mappings: p.Inputs}, current); e != nil {
		return Run{}, e
	}
	if e = a.validateScrapeRun(r, current); e != nil {
		return Run{}, e
	}
	err := a.enqueueLocked([]Run{r})
	if err == nil {
		a.previews.remove(id)
	}
	return summary(r), err
}

func (a *Service) validateScrapeRun(r Run, s model.Settings) error {
	for _, op := range r.Ops {
		if err := a.access.NoSymlinks(filepath.Dir(op.Target)); err != nil {
			return err
		}
		if !withinMedia(s.TargetRoot, op.Target) {
			return errors.New("刮削目标超出当前媒体目录")
		}
	}
	if r.Undo {
		return nil
	}
	for _, m := range r.Mappings {
		if err := a.access.NoSymlinks(m.Path); err != nil {
			return err
		}
		if err := a.validateScrapeMapping(m, r.ID); err != nil {
			return err
		}
		info, err := a.access.Stat(m.Path)
		if !withinMedia(s.TargetRoot, m.Path) || err != nil || !info.Mode().IsRegular() || fsops.Identity(info) != m.Identity || info.Size() != m.Size || info.ModTime().UnixNano() != m.MTime {
			return errors.New("目标视频已变化，请重新预览：" + m.Path)
		}
	}
	pending := []Work{}
	for i, op := range r.Ops {
		if op.Status == "completed" {
			if err := (&Engine{FS: a.access}).Check(context.Background(), op.Target, op.Identity, op.Hash); err != nil {
				return err
			}
			continue
		}
		if op.Status != "pending" {
			continue
		}
		info, statErr := a.access.Stat(op.Target)
		valid := false
		if op.Replace {
			valid = statErr == nil && info.Mode().IsRegular() && fsops.Identity(info) == op.ReplaceIdentity && info.Size() == op.ReplaceSize && info.ModTime().UnixNano() == op.ReplaceMTime
		} else {
			valid = errors.Is(statErr, os.ErrNotExist)
		}
		if !valid {
			reused, reuseErr := a.reuseScrapeArtifact(&op)
			if reuseErr != nil {
				return reuseErr
			}
			if !reused {
				return errors.New("刮削目标已变化，请重新预览：" + op.Target)
			}
			r.Ops[i] = op
		} else {
			pending = append(pending, op)
		}
	}
	return a.validateOps(pending)
}

// Reuse is limited to another recorded, unchanged artifact with the exact same
// intended content. External file changes still invalidate the queued preview.
func (a *Service) reuseScrapeArtifact(op *Work) (bool, error) {
	previous, err := a.store.ScrapeArtifact(op.Target)
	if err != nil {
		return false, err
	}
	if previous == nil || previous.Kind != op.Kind || previous.URL != op.URL || previous.ConvertJPEG != op.ConvertJPEG || !bytes.Equal(previous.Content, op.Content) {
		return false, nil
	}
	if err := (&Engine{FS: a.access}).Check(context.Background(), op.Target, previous.Identity, previous.Hash); err != nil {
		return false, err
	}
	op.Operation = Operation{Kind: op.Kind, Mode: op.Mode, Target: op.Target, URL: op.URL, Content: op.Content, ConvertJPEG: op.ConvertJPEG, Status: "completed", Identity: previous.Identity, Hash: previous.Hash}
	op.Reused = true
	return true, nil
}

func baseSharedArtifact(path string) bool {
	base := filepath.Base(path)
	return base == "poster.jpg" || base == "fanart.jpg" || strings.HasPrefix(base, "clearlogo.")
}
func (a *Service) validateScrapeMapping(m model.MediaMapping, run string) error {
	current, err := a.mappings.Mapping(m.Path)
	if err != nil {
		return err
	}
	expected := m.Original
	if run != "" && m.RunID == run {
		copy := m
		copy.Original = nil
		expected = &copy
	}
	if !matching.SameMapping(current, expected) {
		return errors.New("刮削登记已变化，请重新预览：" + m.Path)
	}
	return nil
}
