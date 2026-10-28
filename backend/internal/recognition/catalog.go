package recognition

import (
	"embed"
	"encoding/json"
)

//go:embed scripts/*.star
var scriptTemplates embed.FS

//go:embed scripts/clean-filename.star
var cleanFilenameScript string

func Catalog() []Descriptor {
	all := Sources()
	file := []Source{Filename, CleanFilename}
	dirs := []Source{ParentDirectory, Directories}
	definitions := []Descriptor{
		{"tmdb-id", "提取 TMDB ID", TMDBID, "提取显式 tmdb 标记中的 ID", all, Config{}},
		{"season-sxe", "SxxExx 季号", Season, "只提取季号", file, Config{}},
		{"episodes-sxe", "SxxExx 集数", Episodes, "支持 E01E03 与 E01-E03", file, Config{}},
		{"type-sxe", "SxxExx 剧集类型", MediaType, "明确季集标记输出 tv", file, Config{}},
		{"season-x", "1x03 季号", Season, "只提取季号", file, Config{}},
		{"episodes-x", "1x03 集数", Episodes, "只提取集数", file, Config{}},
		{"type-x", "1x03 剧集类型", MediaType, "明确季集标记输出 tv", file, Config{}},
		{"season-marker", "文件名 Sxx 季号", Season, "从独立季标记提取季号", file, Config{}},
		{"type-season-marker", "文件名季标记类型", MediaType, "独立季标记输出 tv", file, Config{}},
		{"season-folder", "目录季号", Season, "支持 Season N、第N季和特别篇", dirs, Config{}},
		{"type-folder", "季目录剧集类型", MediaType, "季目录或特别篇目录输出 tv", dirs, Config{}},
		{"episodes-number", "纯集数或 EP 标记", Episodes, "从数字、EP、第N集提取集数", file, Config{}},
		{"type-number", "纯集数剧集类型", MediaType, "父目录内的纯集数文件输出 tv", file, Config{}},
		{"episodes-trailing", "文件名尾部集数", Episodes, "年份与 TMDB ID 不当作尾部集数", file, Config{}},
		{"type-trailing", "尾部集数剧集类型", MediaType, "有效尾部集数输出 tv", file, Config{}},
		{"title-filename", "文件名片名", Title, "清理元数据、画质及季集后缀", file, Config{}},
		{"title-directory", "目录片名", Title, "跳过季目录和纯画质目录", dirs, Config{}},
		{"year", "提取年份", Year, "保留开头的数字片名；参数 min/max 限定年份", all, Config{Params: json.RawMessage(`{"min":1900,"max":2099}`)}},
		{"release-tag", "发布标签", ReleaseTag, "提取文件名末尾非数字发布组标签", file, Config{}},
	}
	for i := range definitions {
		code, err := scriptTemplates.ReadFile("scripts/" + definitions[i].ID + ".star")
		if err != nil {
			panic(err)
		}
		definitions[i].Config.Code = string(code)
	}
	return definitions
}
func descriptor(id string) (Descriptor, bool) {
	for _, entry := range Catalog() {
		if entry.ID == id {
			return entry, true
		}
	}
	return Descriptor{}, false
}
func SourceScripts() map[Source]string { return map[Source]string{CleanFilename: cleanFilenameScript} }
