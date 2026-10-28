package recognition

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var sxeRE = regexp.MustCompile(`(?i)s(\d{1,3})e(\d{1,4})((?:\s*(?:-\s*e?|e)\d{1,4})*)`)
var ePartRE = regexp.MustCompile(`(?i)(-?)\s*e?(\d{1,4})`)
var xRE = regexp.MustCompile(`(?i)\b(\d{1,3})x(\d{1,4})`)
var markerRE = regexp.MustCompile(`(?i)(?:^|[ ._-])s(\d{1,3})(?:$|[ ._-])`)
var folderRE = regexp.MustCompile(`(?i)^(?:season\s*(\d{1,3})|s(\d{1,3})|第\s*([0-9零一二两三四五六七八九十]{1,3})\s*季)$`)
var specialRE = regexp.MustCompile(`(?i)^(?:specials?|特别篇|特別篇)$`)
var episodeRE = regexp.MustCompile(`(?i)^(?:\d{1,4}|ep?\.?\s*\d{1,4}|第\s*\d{1,4}\s*[集话話]|\d{1,4}\s*[集话話]|\d{1,4}v\d)$`)
var trailingRE = regexp.MustCompile(`(?i)^(.*?)[\s._-]+(?:ep?|e)?\.?\s*(\d{1,4})$`)
var yearRE = regexp.MustCompile(`\b\d{4}\b`)
var episodeTagRE = regexp.MustCompile(`(?i)^e\d+$`)

type matchValue struct {
	value    any
	text     string
	captures map[string]string
}
type yearParams struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func Catalog() []Descriptor {
	all := Sources()
	file := []Source{Filename, CleanFilename}
	dirs := []Source{ParentDirectory, Directories}
	definitions := []Descriptor{
		{"tmdb-id", "提取 TMDB ID", TMDBID, "提取显式 tmdb 标记中的 ID", all, nil},
		{"season-sxe", "SxxExx 季号", Season, "只提取季号", file, nil},
		{"episodes-sxe", "SxxExx 集数", Episodes, "支持 E01E03 与 E01-E03", file, nil},
		{"type-sxe", "SxxExx 剧集类型", MediaType, "明确季集标记输出 tv", file, nil},
		{"season-x", "1x03 季号", Season, "只提取季号", file, nil},
		{"episodes-x", "1x03 集数", Episodes, "只提取集数", file, nil},
		{"type-x", "1x03 剧集类型", MediaType, "明确季集标记输出 tv", file, nil},
		{"season-marker", "文件名 Sxx 季号", Season, "从独立季标记提取季号", file, nil},
		{"type-season-marker", "文件名季标记类型", MediaType, "独立季标记输出 tv", file, nil},
		{"season-folder", "目录季号", Season, "支持 Season N、第N季和特别篇", dirs, nil},
		{"type-folder", "季目录剧集类型", MediaType, "季目录或特别篇目录输出 tv", dirs, nil},
		{"episodes-number", "纯集数或 EP 标记", Episodes, "从数字、EP、第N集提取集数", file, nil},
		{"type-number", "纯集数剧集类型", MediaType, "父目录内的纯集数文件输出 tv", file, nil},
		{"episodes-trailing", "文件名尾部集数", Episodes, "年份与 TMDB ID 不当作尾部集数", file, nil},
		{"type-trailing", "尾部集数剧集类型", MediaType, "有效尾部集数输出 tv", file, nil},
		{"title-filename", "文件名片名", Title, "清理元数据、画质及季集后缀", file, nil},
		{"title-directory", "目录片名", Title, "跳过季目录和纯画质目录", dirs, nil},
		{"year", "提取年份", Year, "保留开头的数字片名；参数 min/max 限定年份", all, json.RawMessage(`{"min":1900,"max":2099}`)},
		{"release-tag", "发布标签", ReleaseTag, "提取文件名末尾非数字发布组标签", file, nil},
	}
	for i := range definitions {
		if definitions[i].Params == nil {
			definitions[i].Params = json.RawMessage(`{}`)
		}
	}
	return definitions
}
func descriptor(handler string) (Descriptor, bool) {
	for _, d := range Catalog() {
		if d.Handler == handler {
			return d, true
		}
	}
	return Descriptor{}, false
}
func builtin(handler, text string, p pathInput, params yearParams) (matchValue, error) {
	raw := text
	// Only explicit ID extraction can use the ID token; other handlers see it
	// masked before interpreting any numeric episode or season syntax.
	if handler != "tmdb-id" {
		text = idRE.ReplaceAllString(text, " ")
	}
	single := func(value any, matched string) matchValue {
		return matchValue{value: value, text: matched, captures: map[string]string{"value": fmt.Sprint(value)}}
	}
	switch handler {
	case "tmdb-id":
		if m := idRE.FindStringSubmatch(raw); m != nil {
			return single(m[1], m[0]), nil
		}
	case "season-sxe", "episodes-sxe", "type-sxe":
		if m := sxeRE.FindStringSubmatch(text); m != nil {
			if loc := sxeRE.FindStringIndex(text); loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
				return matchValue{}, fmt.Errorf("季集标记中的数字超出范围")
			}
			if handler == "season-sxe" {
				return single(m[1], m[0]), nil
			}
			if handler == "type-sxe" {
				return single("tv", m[0]), nil
			}
			first, _ := strconv.Atoi(m[2])
			eps := []int{first}
			for _, part := range ePartRE.FindAllStringSubmatch(m[3], -1) {
				n, _ := strconv.Atoi(part[2])
				if part[1] == "-" {
					if n <= eps[len(eps)-1] || n-first >= 1000 {
						return matchValue{}, fmt.Errorf("合并集范围无效")
					}
					for e := eps[len(eps)-1] + 1; e <= n; e++ {
						eps = append(eps, e)
					}
				} else {
					eps = append(eps, n)
				}
			}
			return single(eps, m[0]), nil
		}
	case "season-x", "episodes-x", "type-x":
		if m := xRE.FindStringSubmatch(text); m != nil {
			if loc := xRE.FindStringIndex(text); loc[1] < len(text) && text[loc[1]] >= '0' && text[loc[1]] <= '9' {
				return matchValue{}, fmt.Errorf("季集标记中的数字超出范围")
			}
			if handler == "season-x" {
				return single(m[1], m[0]), nil
			}
			if handler == "episodes-x" {
				return single(m[2], m[0]), nil
			}
			return single("tv", m[0]), nil
		}
	case "season-marker", "type-season-marker":
		if m := markerRE.FindStringSubmatch(text); m != nil {
			if handler == "season-marker" {
				return single(m[1], m[0]), nil
			}
			return single("tv", m[0]), nil
		}
	case "season-folder", "type-folder":
		if specialRE.MatchString(strings.TrimSpace(text)) {
			if handler == "type-folder" {
				return single("tv", raw), nil
			}
			return single(0, raw), nil
		}
		if m := folderRE.FindStringSubmatch(strings.TrimSpace(text)); m != nil {
			if handler == "type-folder" {
				return single("tv", m[0]), nil
			}
			for _, value := range m[1:] {
				if value != "" {
					return single(value, m[0]), nil
				}
			}
		}
	case "episodes-number", "type-number":
		cleaned := cleanInput(text)
		numeric, _ := strconv.Atoi(cleaned)
		if episodeRE.MatchString(cleaned) && len(p.directories) > 0 && !(numeric >= 1900 && numeric <= 2099) {
			if handler == "type-number" {
				return single("tv", cleaned), nil
			}
			return single(numberRE.FindString(cleaned), cleaned), nil
		}
	case "episodes-trailing", "type-trailing":
		if m := trailingRE.FindStringSubmatch(cleanInput(text)); m != nil {
			n, _ := strconv.Atoi(m[2])
			if n > 0 && !(n >= 1900 && n <= 2099) {
				if handler == "type-trailing" {
					return single("tv", m[0]), nil
				}
				return single(m[2], m[0]), nil
			}
		}
	case "title-filename":
		cleaned := cleanInput(text)
		numeric, _ := strconv.Atoi(cleaned)
		if episodeRE.MatchString(cleaned) && len(p.directories) > 0 && !(numeric >= 1900 && numeric <= 2099) {
			return matchValue{}, nil
		}
		cut := len(text)
		for _, re := range []*regexp.Regexp{sxeRE, xRE, markerRE} {
			if loc := re.FindStringIndex(text); loc != nil {
				cut = min(cut, loc[0])
			}
		}
		text = text[:cut]
		if m := trailingRE.FindStringSubmatch(cleanInput(text)); m != nil {
			n, _ := strconv.Atoi(m[2])
			if n > 0 && !(n >= 1900 && n <= 2099) {
				text = m[1]
			}
		}
		if title := titleText(text); title != "" {
			return single(title, raw), nil
		}
	case "title-directory":
		if specialRE.MatchString(strings.TrimSpace(text)) || folderRE.MatchString(strings.TrimSpace(text)) {
			return matchValue{}, nil
		}
		if title := titleText(text); title != "" {
			return single(title, raw), nil
		}
	case "year":
		if year, _ := findYear(text, params); year > 0 {
			return single(year, strconv.Itoa(year)), nil
		}
	case "release-tag":
		if index := strings.LastIndex(raw, "-"); index >= 0 {
			tag := strings.TrimSpace(raw[index+1:])
			if tag != "" && numberRE.FindString(tag) != tag && !episodeTagRE.MatchString(tag) && !strings.Contains(strings.ToLower(raw[max(0, index-5):]), "tmdb") && !qualityRE.MatchString(tag) && !strings.ContainsAny(tag, ". ") {
				return single(tag, tag), nil
			}
		}
	}
	return matchValue{}, nil
}
func findYear(text string, params yearParams) (int, int) {
	text = idRE.ReplaceAllString(text, " ")
	for _, loc := range yearRE.FindAllStringIndex(text, -1) {
		year, _ := strconv.Atoi(text[loc[0]:loc[1]])
		if year >= params.Min && year <= params.Max && NormalizeTitle(bracketsRE.ReplaceAllString(text[:loc[0]], " ")) != "" {
			return year, loc[0]
		}
	}
	return 0, -1
}
func titleText(text string) string {
	text = idRE.ReplaceAllString(text, " ")
	if _, index := findYear(text, yearParams{1900, 2099}); index >= 0 {
		text = text[:index]
	}
	if loc := qualityRE.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}
	text = bracketsRE.ReplaceAllString(text, " ")
	text = strings.NewReplacer(".", " ", "_", " ").Replace(text)
	return strings.Trim(strings.Join(strings.Fields(text), " "), " -_[](){}【】")
}
