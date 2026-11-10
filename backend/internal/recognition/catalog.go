package recognition

import (
	"embed"
	"encoding/json"
)

//go:embed scripts/*.js
var scriptTemplates embed.FS

func Catalog() []Rule {
	definitions := []struct {
		id, name, stage string
		role            Target
	}{
		{"tokenizer", "分词", "tokenize", ""}, {"tmdb-id", "TMDB ID", "annotate", TMDBID},
		{"resolution", "分辨率", "annotate", "resolution"}, {"videoCodec", "视频编码与来源", "annotate", "videoCodec"}, {"audioCodec", "音频编码", "annotate", "audioCodec"}, {"bitDepth", "位深", "annotate", "bitDepth"}, {"dynamicRange", "动态范围", "annotate", "dynamicRange"}, {"language", "语言", "annotate", "language"}, {"subtitleLanguage", "字幕语言", "annotate", "subtitleLanguage"}, {"checksum", "校验码", "annotate", "checksum"},
		{"release-tag", "发布组", "annotate", ReleaseTag}, {"season", "季号", "annotate", Season}, {"year", "年份", "annotate", Year}, {"episodes", "集数", "annotate", Episodes}, {"title", "片名", "annotate", Title}, {"resolver", "元数据裁决", "resolve", ""},
	}
	out := []Rule{}
	for _, d := range definitions {
		code, err := scriptTemplates.ReadFile("scripts/" + d.id + ".js")
		if err != nil {
			panic(err)
		}
		r := Rule{ID: d.id, Name: d.name, Stage: d.stage, Target: d.role, Enabled: true, Kind: "script", Config: Config{Code: string(code)}, Examples: []Example{}}
		if d.stage == "annotate" {
			r.Source = AllSources
		}
		if d.id == "year" {
			r.Config.Params = json.RawMessage(`{"min":1900,"max":2099}`)
		}
		out = append(out, r)
	}
	return out
}
