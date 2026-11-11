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
		{"season", "季号", "annotate", Season}, {"year", "年份", "annotate", Year}, {"episodes", "集数", "annotate", Episodes}, {"title", "片名", "annotate", Title}, {"resolver", "建议脚本", "resolve", ""},
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
