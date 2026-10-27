package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"joycurator/internal/fsops"
	"joycurator/internal/model"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true, ".ts": true, ".m2ts": true, ".wmv": true, ".webm": true, ".mpg": true, ".mpeg": true, ".flv": true}
var seasonRE = regexp.MustCompile(`(?i)(?:S(\d{1,3})E(\d{1,4})(?:(?:-?E?)(\d{1,4}))?|\b(\d{1,3})x(\d{1,4}))`)
var yearRE = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
var idRE = regexp.MustCompile(`(?i)tmdb[-= ]?(\d+)`)
var junkRE = regexp.MustCompile(`(?i)\b(?:2160p|1080p|1080i|720p|480p|4k|bluray|blu-ray|bdrip|webrip|web-dl|webdl|hdtv|remux|x264|x265|h264|h265|hevc|avc|aac|dts|ddp)\b`)
var seasonFolderRE = regexp.MustCompile(`(?i)^(?:season\s*\d+|s\d+|第.+季)$`)

func Parse(name string) model.Parsed {
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	clean := strings.NewReplacer(".", " ", "_", " ").Replace(stem)
	p := model.Parsed{Type: "movie", Episodes: []int{}}
	cut := len(clean)
	if loc := idRE.FindStringSubmatchIndex(clean); loc != nil {
		p.TMDBID, _ = strconv.Atoi(clean[loc[2]:loc[3]])
		if loc[0] < cut {
			cut = loc[0]
		}
	}
	if loc := seasonRE.FindStringSubmatchIndex(clean); loc != nil {
		p.Type = "tv"
		cut = min(cut, loc[0])
		m := seasonRE.FindStringSubmatch(clean)
		if m[1] != "" {
			p.Season, _ = strconv.Atoi(m[1])
			first, _ := strconv.Atoi(m[2])
			p.Episodes = []int{first}
			if m[3] != "" {
				last, _ := strconv.Atoi(m[3])
				if last > first && last-first <= 100 {
					for e := first + 1; e <= last; e++ {
						p.Episodes = append(p.Episodes, e)
					}
				}
			}
		} else {
			p.Season, _ = strconv.Atoi(m[4])
			e, _ := strconv.Atoi(m[5])
			p.Episodes = []int{e}
		}
	}
	if loc := yearRE.FindStringIndex(clean); loc != nil {
		p.Year, _ = strconv.Atoi(clean[loc[0]:loc[1]])
		cut = min(cut, loc[0])
	}
	if loc := junkRE.FindStringIndex(clean); loc != nil {
		cut = min(cut, loc[0])
	}
	p.Title = strings.Trim(clean[:cut], " -_[](){}")
	if p.Title == "" {
		parent := filepath.Base(filepath.Dir(name))
		if seasonFolderRE.MatchString(parent) {
			parent = filepath.Base(filepath.Dir(filepath.Dir(name)))
		}
		p.Title = strings.TrimSpace(strings.NewReplacer(".", " ", "_", " ").Replace(parent))
	}
	if i := strings.LastIndex(stem, "-"); i >= 0 && i+1 < len(stem) {
		p.ReleaseTag = stem[i+1:]
	}
	return p
}

var episodeOnlyRE = regexp.MustCompile(`(?i)^(?:\d{1,4}|e[p]?\.?\s*\d{1,4}|第\s*\d{1,4}\s*[集话話]|\d{1,4}\s*[集话話]|\d{1,4}v\d)$`)
var seasonNumberRE = regexp.MustCompile(`(?i)^(?:season\s*(\d{1,2})|s(\d{1,2})|第\s*([0-9一二三四五六七八九十]{1,3})\s*季)$`)
var bracketRE = regexp.MustCompile(`[\[【][^\]】]*[\]】]`)
var digitRunRE = regexp.MustCompile(`\d{1,4}`)
var trailingEpisodeRE = regexp.MustCompile(`(?i)^(.*?)[\s._-]+(?:ep?|e)?\.?\s*(\d{1,4})$`)

// ParsePath derives metadata from the full path.
func ParsePath(path string) model.Parsed {
	return ParsePathIn(path, nil)
}

// ParsePathIn is ParsePath with a set of roots: the folder walk for a title
// stops there, so a source directory is never mistaken for a show name.
//
//	凡人修仙传/1.mp4              → 凡人修仙传 S01E01
//	Show/Season 2/03.mkv         → Show S02E03
//	[Group] Show [1080p]/01.mkv  → Show S01E01
//	4k/1.mp4                     → unknown title, unknown season
func ParsePathIn(path string, roots []string) model.Parsed {
	stop := map[string]bool{}
	for _, root := range roots {
		stop[filepath.Clean(root)] = true
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	trimmed := strings.TrimSpace(stem)

	// A season/episode pattern in the name always wins.
	if seasonRE.MatchString(stem) {
		return Parse(path)
	}

	episode, title := 0, ""
	if episodeOnlyRE.MatchString(trimmed) {
		if run := digitRunRE.FindString(stem); run != "" {
			episode, _ = strconv.Atoi(run)
		}
	} else {
		// "[Group] Show - 01 [1080p]" keeps its number at the end.
		cleaned := bracketRE.ReplaceAllString(stem, " ")
		cleaned = junkRE.ReplaceAllString(cleaned, " ")
		if match := trailingEpisodeRE.FindStringSubmatch(strings.TrimSpace(cleaned)); match != nil {
			number, _ := strconv.Atoi(match[2])
			if number > 0 && !(number >= 1900 && number <= 2099) {
				episode = number
				title = cleanTitle(match[1])
			}
		}
	}
	if episode == 0 {
		parsed := Parse(path)
		if parsed.Title == "" {
			parsed.Title, _ = titleAndSeasonFromPath(path, stop)
		}
		return parsed
	}

	// The season is only known when a season folder says so; guessing 1 would
	// silently mislabel a season 2 folder.
	folderTitle, season := titleAndSeasonFromPath(path, stop)
	if title == "" {
		title = folderTitle
	}
	year := 0
	if match := yearRE.FindString(title); match != "" {
		year, _ = strconv.Atoi(match)
		title = strings.Trim(strings.TrimSpace(strings.Replace(title, match, "", 1)), " -_[](){}")
	}
	return model.Parsed{Title: title, Year: year, Type: "tv", Season: season, Episodes: []int{episode}}
}

// titleAndSeasonFromPath walks up from the file, skipping season folders and
// folders that are only a quality tag, and stops at a configured root.
func titleAndSeasonFromPath(path string, stop map[string]bool) (string, int) {
	dir := filepath.Dir(path)
	season := 0
	for depth := 0; depth < 4; depth++ {
		if stop[filepath.Clean(dir)] {
			break
		}
		part := filepath.Base(dir)
		if part == "." || part == string(filepath.Separator) || part == "" {
			break
		}
		if number, ok := seasonNumber(part); ok {
			if season == 0 {
				season = number
			}
			dir = filepath.Dir(dir)
			continue
		}
		cleaned := cleanTitle(part)
		if cleaned == "" {
			dir = filepath.Dir(dir)
			continue
		}
		return cleaned, season
	}
	return "", season
}

func seasonNumber(name string) (int, bool) {
	match := seasonNumberRE.FindStringSubmatch(strings.TrimSpace(name))
	if match == nil {
		return 0, false
	}
	value := ""
	for _, group := range match[1:] {
		if group != "" {
			value = group
		}
	}
	if value == "" {
		return 0, false
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number, true
	}
	return chineseNumber(value)
}

// chineseNumber converts the small numerals used in 第N季 folder names.
func chineseNumber(value string) (int, bool) {
	digits := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	runes := []rune(value)
	switch len(runes) {
	case 1:
		if runes[0] == '十' {
			return 10, true
		}
		if number, ok := digits[runes[0]]; ok {
			return number, true
		}
	case 2:
		if runes[1] == '十' {
			if number, ok := digits[runes[0]]; ok {
				return number * 10, true
			}
		}
		if runes[0] == '十' {
			if number, ok := digits[runes[1]]; ok {
				return 10 + number, true
			}
		}
	case 3:
		if runes[1] == '十' {
			first, ok1 := digits[runes[0]]
			last, ok2 := digits[runes[2]]
			if ok1 && ok2 {
				return first*10 + last, true
			}
		}
	}
	return 0, false
}

// cleanTitle strips release-group brackets, quality tags and separator noise
// from a folder name. It returns an empty string when nothing meaningful is
// left, so the caller can keep walking up.
func cleanTitle(name string) string {
	base := bracketRE.ReplaceAllString(name, " ")
	base = strings.NewReplacer(".", " ", "_", " ").Replace(base)
	base = strings.Join(strings.Fields(base), " ")
	if base == "" {
		// The name was nothing but brackets; keep what is inside them.
		inner := strings.NewReplacer("[", " ", "]", " ", "【", " ", "】", " ").Replace(name)
		base = strings.Join(strings.Fields(inner), " ")
	}
	base = junkRE.ReplaceAllString(base, " ")
	base = strings.Join(strings.Fields(base), " ")
	return strings.Trim(base, " -_[](){}")
}

func Probe(ctx context.Context, access *fsops.FS, path string) model.Probe {
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	file, err := access.Open(path)
	if err != nil {
		return model.Probe{Warning: "媒体文件不可读"}
	}
	defer file.Close()
	cmd := exec.CommandContext(deadline, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,width,height:format=duration", "-of", "json", "-i", "/dev/fd/3")
	cmd.ExtraFiles = []*os.File{file}
	bytes, err := cmd.Output()
	if err != nil {
		return model.Probe{Warning: "ffprobe 不可用或无法读取媒体；画质字段暂为空"}
	}
	var data struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(bytes, &data) != nil {
		return model.Probe{Warning: "媒体探测返回格式错误"}
	}
	p := model.Probe{}
	p.Duration, _ = strconv.ParseFloat(data.Format.Duration, 64)
	for _, stream := range data.Streams {
		if stream.Type == "video" {
			p.Codec = stream.Codec
			if stream.Width >= 3800 || stream.Height >= 2000 {
				p.Resolution = "2160p"
			} else if stream.Width >= 1900 || stream.Height >= 1000 {
				p.Resolution = "1080p"
			} else if stream.Width >= 1200 || stream.Height >= 700 {
				p.Resolution = "720p"
			} else if stream.Height > 0 {
				p.Resolution = fmt.Sprintf("%dp", stream.Height)
			}
			break
		}
	}
	return p
}
func IsVideo(name string) bool { return videoExt[strings.ToLower(filepath.Ext(name))] }

func Walk(ctx context.Context, access *fsops.FS, sources, excluded []string, visit func(string, fs.FileInfo) error) error {
	canonicalExcluded := make([]string, len(excluded))
	for i, path := range excluded {
		if path != "" {
			canonicalExcluded[i] = access.Canonical(path)
		}
	}
	excluded = canonicalExcluded
	seen := map[string]bool{}
	for _, source := range sources {
		source = access.Canonical(source)
		root, rel, err := access.Root(source)
		if err != nil {
			return err
		}
		err = fs.WalkDir(root.FS(), rel, func(name string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return walkErr
			}
			abs := filepath.Join(root.Name(), name)
			if d.Type()&fs.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				for _, e := range excluded {
					if e != "" && fsops.Within(e, abs) {
						return fs.SkipDir
					}
				}
				if strings.HasPrefix(d.Name(), ".") && abs != source {
					return fs.SkipDir
				}
				return nil
			}
			if seen[abs] || !videoExt[strings.ToLower(filepath.Ext(name))] || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			seen[abs] = true
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			return visit(abs, info)
		})
		root.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
