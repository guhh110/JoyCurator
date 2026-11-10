// Package recognition interprets immutable path snapshots without filesystem or network access.
package recognition

import (
	"context"
	"encoding/json"
	"time"
)

type Target string

const (
	TMDBID     Target = "tmdbId"
	Title      Target = "title"
	MediaType  Target = "mediaType"
	Year       Target = "year"
	Season     Target = "season"
	Episodes   Target = "episodes"
	ReleaseTag Target = "releaseTag"
)

func Targets() []Target {
	return []Target{TMDBID, Title, MediaType, Year, Season, Episodes, ReleaseTag}
}
func Roles() []Target {
	return append(Targets(), "resolution", "videoCodec", "audioCodec", "bitDepth", "dynamicRange", "language", "subtitleLanguage", "checksum")
}

type Source string

const (
	Filename    Source = "filename"
	Directories Source = "directories"
	AllSources  Source = "all"
)

func Sources() []Source { return []Source{Filename, Directories, AllSources} }

type Config struct {
	Code    string          `json:"code,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Pattern string          `json:"pattern,omitempty"`
	Capture string          `json:"capture,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}
type Example struct {
	Paths    []string        `json:"paths"`
	Root     string          `json:"root"`
	Targets  []string        `json:"targets"`
	Expected json.RawMessage `json:"expected"`
}
type Rule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Stage       string    `json:"stage"`
	Target      Target    `json:"target,omitempty"`
	Source      Source    `json:"source,omitempty"`
	Order       int       `json:"order"`
	Enabled     bool      `json:"enabled"`
	Kind        string    `json:"kind"`
	Config      Config    `json:"config"`
	Examples    []Example `json:"examples"`
}
type Pipeline struct {
	Tokenizer  Rule   `json:"tokenizer"`
	Annotators []Rule `json:"annotators"`
	Resolver   Rule   `json:"resolver"`
}

func (p Pipeline) Rules() []Rule {
	out := []Rule{p.Tokenizer}
	out = append(out, p.Annotators...)
	return append(out, p.Resolver)
}

type Metadata struct {
	TMDBID     *int    `json:"tmdbId"`
	Title      *string `json:"title"`
	MediaType  *string `json:"mediaType"`
	Year       *int    `json:"year"`
	Season     *int    `json:"season"`
	Episodes   []int   `json:"episodes"`
	ReleaseTag *string `json:"releaseTag"`
}
type Input struct{ Path, Root string }
type BatchInput struct {
	ContextRoot    string   `json:"contextRoot"`
	SourceBoundary string   `json:"sourceBoundary"`
	Paths          []string `json:"paths"`
	Targets        []string `json:"targets"`
}
type TextSource struct {
	ID    string `json:"id"`
	Kind  Source `json:"kind"`
	Text  string `json:"text"`
	Depth int    `json:"depth"`
}
type Token struct {
	ID       string  `json:"id"`
	SourceID string  `json:"sourceId"`
	ParentID *string `json:"parentId"`
	Kind     string  `json:"kind"`
	Text     string  `json:"text"`
	Start    int     `json:"start"`
	End      int     `json:"end"`
}
type Span struct {
	SourceID string `json:"sourceId"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
}
type Candidate struct {
	ID       string `json:"id"`
	RuleID   string `json:"ruleId"`
	Role     Target `json:"role"`
	Value    any    `json:"value"`
	Spans    []Span `json:"spans"`
	Strength string `json:"strength"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}
type Document struct {
	FileID     string       `json:"fileId"`
	Path       string       `json:"path"`
	Directory  string       `json:"directory"`
	Sources    []TextSource `json:"sources"`
	Tokens     []Token      `json:"tokens"`
	Candidates []Candidate  `json:"candidates"`
}
type TreeNode struct {
	Path    string   `json:"path"`
	Parent  string   `json:"parent"`
	FileIDs []string `json:"fileIds"`
}
type Reference struct {
	FileID      string `json:"fileId"`
	CandidateID string `json:"candidateId"`
}
type Decision struct {
	Candidates []Reference `json:"candidates"`
	Reason     string      `json:"reason"`
}
type FileResult struct {
	FileID    string              `json:"fileId"`
	Path      string              `json:"path"`
	Metadata  Metadata            `json:"metadata"`
	Decisions map[Target]Decision `json:"decisions"`
}
type BatchResult struct {
	Error     string       `json:"error,omitempty"`
	Context   BatchInput   `json:"context"`
	Tree      []TreeNode   `json:"tree"`
	Documents []Document   `json:"documents"`
	Files     []FileResult `json:"files"`
	Steps     []Step       `json:"steps"`
}
type Result struct {
	Metadata Metadata `json:"metadata"`
	Steps    []Step   `json:"steps"`
}
type Attempt struct {
	Input       string            `json:"input"`
	Matched     bool              `json:"matched"`
	MatchedText string            `json:"matchedText,omitempty"`
	Captures    map[string]string `json:"captures,omitempty"`
	Error       string            `json:"error,omitempty"`
	Trace       []ScriptEvent     `json:"trace,omitempty"`
}
type ScriptEvent struct {
	Phase     string `json:"phase"`
	Line      int32  `json:"line"`
	Column    int32  `json:"column"`
	Function  string `json:"function,omitempty"`
	Operation string `json:"operation"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
}
type Step struct {
	RuleID         string            `json:"ruleId"`
	RuleName       string            `json:"ruleName"`
	Order          int               `json:"order"`
	Stage          string            `json:"stage"`
	FileID         string            `json:"fileId"`
	RuleConfig     Rule              `json:"ruleConfig"`
	Target         Target            `json:"target,omitempty"`
	Inputs         []string          `json:"inputs"`
	Attempts       []Attempt         `json:"attempts"`
	Status         string            `json:"status"`
	MatchedText    string            `json:"matchedText"`
	Captures       map[string]string `json:"captures"`
	ExtractedValue any               `json:"extractedValue"`
	Before         any               `json:"before"`
	After          any               `json:"after"`
	Applied        bool              `json:"applied"`
	Reason         string            `json:"reason"`
	StartedAt      time.Time         `json:"startedAt"`
	DurationMS     float64           `json:"durationMs"`
}
type Observer func(context.Context, Step) error
type Trace struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Root        string   `json:"root"`
	WorkspaceID string   `json:"workspaceId,omitempty"`
	Status      string   `json:"status"`
	StartedAt   string   `json:"startedAt"`
	FinishedAt  string   `json:"finishedAt,omitempty"`
	Metadata    Metadata `json:"metadata"`
	Error       string   `json:"error,omitempty"`
	StepCount   int      `json:"stepCount"`
}
type TestRequest struct {
	BatchInput
	Pipeline *Pipeline `json:"pipeline,omitempty"`
}
type ExampleResult struct {
	RuleID   string   `json:"ruleId"`
	Paths    []string `json:"paths"`
	Expected any      `json:"expected"`
	Actual   any      `json:"actual"`
	Passed   bool     `json:"passed"`
	Error    string   `json:"error"`
}
