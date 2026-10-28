package rules

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"joycurator/internal/fsops"
	"joycurator/internal/model"
)

var tokenRE = regexp.MustCompile(`\{([a-z_]+)(?::(02))?\}`)
var illegalRE = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
var allowed = map[string]bool{"category": true, "title": true, "original_title": true, "year": true, "tmdb_id": true, "season": true, "episode": true, "episode_range": true, "resolution": true, "codec": true, "release_tag": true, "original_stem": true, "ext": true}

func Clean(value string) string {
	value = illegalRE.ReplaceAllString(value, "_")
	value = strings.Trim(value, " .")
	if value == "" {
		return "_"
	}
	return value
}
func Validate(r model.Rules) error {
	for _, template := range []string{r.MovieFolder, r.MovieFile, r.TVFolder, r.TVFile} {
		if template == "" || len(template) > 2048 {
			return errors.New("命名模板不能为空且不得超过 2048 字节")
		}
		for _, m := range tokenRE.FindAllStringSubmatch(template, -1) {
			if !allowed[m[1]] {
				return fmt.Errorf("未知模板字段：%s", m[1])
			}
		}
		remaining := tokenRE.ReplaceAllString(template, "")
		if strings.ContainsAny(remaining, "{}") || filepath.IsAbs(template) || strings.Contains(template, "\\") {
			return errors.New("模板字段格式或路径无效")
		}
		for _, part := range strings.Split(template, "/") {
			if part == ".." || part == "." || part == "" {
				return errors.New("模板不允许包含空目录、. 或 ..")
			}
		}
	}
	if strings.Contains(r.MovieFile, "/") || strings.Contains(r.TVFile, "/") {
		return errors.New("文件名模板不能包含目录")
	}
	for _, c := range r.Categories {
		if err := categoryPath(c.Name); err != nil {
			return err
		}
		if c.Type != "" && c.Type != "movie" && c.Type != "tv" {
			return errors.New("分类媒体类型无效")
		}
		if c.YearMin < 0 || c.YearMax < 0 || (c.YearMax != 0 && c.YearMax < c.YearMin) {
			return errors.New("分类年份范围无效")
		}
	}
	return nil
}
func categoryPath(name string) error {
	if name == "" || len(name) > 1024 || !filepath.IsLocal(name) || strings.Contains(name, "\\") {
		return errors.New("分类子目录必须是相对路径")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || Clean(part) != part || len(part) > 240 {
			return errors.New("分类子目录包含无效目录名")
		}
	}
	return nil
}
func Category(item model.Entry, categories []model.Category) string {
	for _, c := range categories {
		if c.Type != "" && c.Type != item.Type {
			continue
		}
		if c.YearMin > 0 && item.Year < c.YearMin {
			continue
		}
		if c.YearMax > 0 && item.Year > c.YearMax {
			continue
		}
		if len(c.Countries) > 0 && !overlap(c.Countries, item.Countries) {
			continue
		}
		if len(c.Genres) > 0 && !overlap(c.Genres, item.Genres) {
			continue
		}
		return c.Name
	}
	return "未分类"
}
func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}
func EpisodeRange(season int, episodes []int) string {
	if len(episodes) == 0 {
		return ""
	}
	result := fmt.Sprintf("S%02dE%02d", season, episodes[0])
	for _, e := range episodes[1:] {
		result += fmt.Sprintf("E%02d", e)
	}
	return result
}
func values(file model.Source, rule model.Rules) map[string]string {
	item := file.Entry
	v := map[string]string{
		"category":       Category(*item, rule.Categories),
		"title":          item.Title,
		"original_title": item.OriginalTitle,
		"tmdb_id":        strconv.Itoa(item.TMDBID),
		"season":         strconv.Itoa(file.Season),
		"episode_range":  EpisodeRange(file.Season, file.Episodes),
		"resolution":     file.Probe.Resolution,
		"codec":          file.Probe.Codec,
		"release_tag":    file.Parsed.ReleaseTag,
		"original_stem":  strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path)),
		"ext":            strings.TrimPrefix(filepath.Ext(file.Path), "."),
	}
	if item.Year > 0 {
		v["year"] = strconv.Itoa(item.Year)
	}
	if len(file.Episodes) > 0 {
		v["episode"] = strconv.Itoa(file.Episodes[0])
	}
	return v
}
func Render(template string, values map[string]string, optionalQuality bool) (string, error) {
	var failure error
	for _, field := range []string{"resolution", "codec", "release_tag"} {
		if optionalQuality && values[field] == "" {
			template = strings.ReplaceAll(template, " - {"+field+"}", "")
		}
	}
	result := tokenRE.ReplaceAllStringFunc(template, func(token string) string {
		m := tokenRE.FindStringSubmatch(token)
		value := values[m[1]]
		if value == "" {
			failure = fmt.Errorf("模板字段缺失：%s", m[1])
			return ""
		}
		if m[2] == "02" {
			n, err := strconv.Atoi(value)
			if err != nil {
				failure = fmt.Errorf("字段 %s 不能格式化为数字", m[1])
			}
			value = fmt.Sprintf("%02d", n)
		}
		if m[1] == "category" {
			if err := categoryPath(value); err != nil {
				failure = err
				return ""
			}
			return value
		}
		return Clean(value)
	})
	if failure != nil {
		return "", failure
	}
	if !filepath.IsLocal(result) {
		return "", errors.New("模板生成的路径越界")
	}
	for _, part := range strings.Split(result, "/") {
		if len([]byte(part)) > 240 || part == ".." || part == "." || part == "" || Clean(part) != part {
			return "", errors.New("模板生成的目录或文件名无效或过长")
		}
	}
	return result, nil
}
func Target(file model.Source, settings model.Settings) (string, error) {
	if file.Entry == nil {
		return "", errors.New("尚未确认 TMDB 条目")
	}
	r := settings.Rules
	root, folder, filename := settings.TargetRoot, r.MovieFolder, r.MovieFile
	defaults := model.DefaultSettings().Rules
	optional := r.MovieFile == defaults.MovieFile
	if file.Entry.Type == "tv" {
		folder, filename = r.TVFolder, r.TVFile
		optional = r.TVFile == defaults.TVFile
		if len(file.Episodes) == 0 {
			return "", errors.New("剧集尚未指定季集")
		}
	}
	if root == "" {
		return "", errors.New("尚未配置目标目录")
	}
	v := values(file, r)
	dir, err := Render(folder, v, false)
	if err != nil {
		return "", err
	}
	base, err := Render(filename, v, optional)
	if err != nil {
		return "", err
	}
	if filepath.Base(base) != base {
		return "", errors.New("文件名模板生成了多级路径")
	}
	if !strings.EqualFold(filepath.Ext(base), filepath.Ext(file.Path)) {
		return "", errors.New("文件名模板必须保留原视频扩展名")
	}
	target := filepath.Join(root, dir, base)
	if !fsops.Within(root, target) {
		return "", errors.New("目标路径越界")
	}
	return target, nil
}

// ShowRoot is the directory before the first season-dependent component.
func ShowRoot(file model.Source, settings model.Settings) (string, error) {
	if file.Entry == nil {
		return "", errors.New("尚未确认 TMDB 条目")
	}
	parts := strings.Split(settings.Rules.TVFolder, "/")
	end := len(parts)
	for i, part := range parts {
		if strings.Contains(part, "{season") {
			end = i
			break
		}
	}
	if end == 0 {
		return "", errors.New("剧集模板需要在季目录之前提供剧集目录")
	}
	prefix := strings.Join(parts[:end], "/")
	if !strings.Contains(prefix, "{title}") && !strings.Contains(prefix, "{tmdb_id}") {
		return "", errors.New("剧集目录需要包含片名或 TMDB ID")
	}
	path, err := Render(prefix, values(file, settings.Rules), false)
	if err != nil {
		return "", err
	}
	return filepath.Join(settings.TargetRoot, path), nil
}
