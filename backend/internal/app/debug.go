package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"joycurator/internal/infra/fsops"
	"joycurator/internal/infra/scanner"
	"joycurator/internal/matching"
	"joycurator/internal/model"
	"joycurator/internal/recognition"
	"joycurator/internal/trace"
)

type RecognitionDebugRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Root string `json:"root,omitempty"`
}
type RecognitionDebugInput struct {
	Path               string `json:"path"`
	NormalizedPath     string `json:"normalizedPath"`
	Root               string `json:"root"`
	BoundarySource     string `json:"boundarySource"`
	ExtensionSupported bool   `json:"extensionSupported"`
	WithinSource       bool   `json:"withinSource"`
	WithinTarget       bool   `json:"withinTarget"`
	FileExistence      string `json:"fileExistence"`
	FileAccess         string `json:"fileAccess"`
}
type RecognitionDebugResult struct {
	Mode       string                `json:"mode"`
	Input      RecognitionDebugInput `json:"input"`
	Parsed     model.Parsed          `json:"parsed"`
	Entry      *model.Entry          `json:"entry"`
	Candidates []model.Entry         `json:"candidates"`
	State      string                `json:"state"`
	Reason     string                `json:"reason"`
	Season     int                   `json:"season"`
	Episodes   []int                 `json:"episodes"`
	Steps      []trace.Event         `json:"steps"`
	DurationMS float64               `json:"durationMs"`
}

// Debug only loads configuration; it never stats, opens or walks the
// supplied media path, and never creates a workspace, run or execution plan.
func (a *WorkspaceService) Debug(ctx context.Context, in RecognitionDebugRequest) (*RecognitionDebugResult, error) {
	s, err := a.settings.Load()
	if err != nil {
		return nil, err
	}
	input, err := debugPathInput(in, s)
	if err != nil {
		return nil, err
	}
	return recognizeDebug(ctx, in.Mode, input, s, a.factory, a.extract.Current()), nil
}
func debugPathInput(in RecognitionDebugRequest, s model.Settings) (RecognitionDebugInput, error) {
	if in.Mode != "full" && in.Mode != "parse" {
		return RecognitionDebugInput{}, errors.New("调试模式必须为 full 或 parse")
	}
	if !validDebugPath(in.Path) {
		return RecognitionDebugInput{}, errors.New("请输入以 / 开头的完整文件路径，不支持 Windows 路径或控制字符")
	}
	path := filepath.Clean(in.Path)
	if path == "/" || strings.HasSuffix(in.Path, "/") {
		return RecognitionDebugInput{}, errors.New("请输入完整文件路径，不能只提供目录")
	}
	input := RecognitionDebugInput{Path: in.Path, NormalizedPath: path, FileExistence: "unverified", FileAccess: "unverified", ExtensionSupported: scanner.IsVideo(path), WithinSource: s.SourceRoot != "" && fsops.Within(s.SourceRoot, path), WithinTarget: s.TargetRoot != "" && fsops.Within(s.TargetRoot, path)}
	switch {
	case in.Root != "":
		if !validDebugPath(in.Root) {
			return input, errors.New("解析边界必须是以 / 开头的目录路径")
		}
		input.Root, input.BoundarySource = filepath.Clean(in.Root), "custom"
	case input.WithinSource:
		input.Root, input.BoundarySource = s.SourceRoot, "source"
	default:
		input.Root, input.BoundarySource = "/", "filesystem"
	}
	if input.Root == path || !fsops.Within(input.Root, path) {
		return input, errors.New("解析边界必须是输入文件路径的父级目录")
	}
	return input, nil
}
func validDebugPath(path string) bool {
	return len(path) > 0 && len(path) <= 16384 && strings.HasPrefix(path, "/") && strings.IndexFunc(path, unicode.IsControl) < 0
}

// A factory keeps the orchestration testable with a local transport while the
// production endpoint always obtains its client from the existing settings.
func recognizeDebug(ctx context.Context, mode string, input RecognitionDebugInput, s model.Settings, newClient matching.Factory, configured ...*recognition.Engine) *RecognitionDebugResult {
	started := time.Now()
	log := trace.New()
	log.Add("input", "path-input", "info", "只处理路径文本，文件存在性和权限未验证", input)
	if !input.ExtensionSupported {
		log.Add("input", "extension", "skipped", "此扩展名会被正式扫描跳过；下方仍模拟路径解析和识别", map[string]any{"extension": filepath.Ext(input.NormalizedPath)})
	}
	if !input.WithinSource || input.WithinTarget {
		log.Add("input", "source-scope", "info", "路径文本不符合当前正式导入范围；仍允许模拟识别", map[string]any{"withinSource": input.WithinSource, "withinTarget": input.WithinTarget})
	}
	engine := recognition.DefaultEngine()
	if len(configured) > 0 {
		engine = configured[0]
	}
	extracted, extractionErr := engine.Extract(ctx, recognition.Input{Path: input.NormalizedPath, Root: input.Root}, nil)
	for _, step := range extracted.Steps {
		log.Add("parse", step.RuleID, step.Status, step.Reason, step)
	}
	parsed := recognition.ParsedMetadata(extracted.Metadata)
	log.Add("parse", "parsed-result", "info", "路径规则链执行完成", parsed)
	item := matching.Item{File: model.File{Path: input.NormalizedPath, Season: parsed.Season, Episodes: parsed.Episodes}, Metadata: extracted.Metadata, Candidates: []model.Entry{}, State: "parsed", Reason: "已完成路径解析，未访问 TMDB"}
	if extractionErr != nil {
		item.State, item.Reason = "failed", "路径提取已停止，保留已完成步骤"
	} else if mode == "full" {
		if s.Token == "" {
			item.State, item.Reason = "failed", "未配置 TMDB Token，路径解析已完成，联网识别未执行"
			log.Add("tmdb", "credentials", "failed", item.Reason, nil)
		} else if client, err := newClient(s, log); err != nil {
			item.State, item.Reason = "failed", err.Error()
			log.Add("tmdb", "configuration", "failed", item.Reason, nil)
		} else {

			memo := matching.NewResolver(client, log)

			memo.Match(ctx, &item)
			if item.Entry != nil {
				memo.Validate(ctx, &item)
			} else {
				log.Add("validate", "no-entry", "skipped", "尚未确定作品，不进行季集校验", nil)
			}
		}
	}
	log.Add("result", "final-result", item.State, item.Reason, map[string]any{"entry": item.Entry, "season": item.Season, "episodes": item.Episodes, "simulationOnly": true})
	return &RecognitionDebugResult{Mode: mode, Input: input, Parsed: parsed, Entry: item.Entry, Candidates: item.Candidates, State: item.State, Reason: item.Reason, Season: item.Season, Episodes: item.Episodes, Steps: log.Events(), DurationMS: float64(time.Since(started).Microseconds()) / 1000}
}
