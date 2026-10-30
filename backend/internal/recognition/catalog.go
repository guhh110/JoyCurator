package recognition

import (
	"embed"
	"encoding/json"
)

//go:embed scripts/*.js
var scriptTemplates embed.FS

//go:embed scripts/clean-filename.js
var cleanFilenameScript string

// Catalog lists the default field rules as editable templates. Each template is
// one complete script for one field, trying several notations in priority order.
func Catalog() []Descriptor {
	all := []Source{RelativePath, Filename, CleanFilename, ParentDirectory, Directories, AbsolutePath}
	definitions := []Descriptor{
		{"tmdb-id", "TMDB ID", TMDBID, "文件名或目录中的 tmdb-603 标记", all, Config{}},
		{"season", "季号", Season, "依次尝试 SxxExx、1x03、独立 Sxx 标记、季目录", all, Config{}},
		{"episodes", "集数", Episodes, "依次尝试 SxxExx、1x03、阿拉伯或中文纯集号文件名、尾部集号", all, Config{}},
		{"media-type", "媒体类型", MediaType, "已识别季号或集数即为剧集", all, Config{}},
		{"title", "片名", Title, "先取文件名，只剩集号时改用目录名", all, Config{}},
		{"year", "年份", Year, "片名之后的四位年份；参数 min/max 限定范围", all, Config{Params: json.RawMessage(`{"min":1900,"max":2099}`)}},
		{"release-tag", "发布标签", ReleaseTag, "文件名末尾的发布组", all, Config{}},
	}
	for i := range definitions {
		code, err := scriptTemplates.ReadFile("scripts/" + definitions[i].ID + ".js")
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
