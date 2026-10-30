// Package recognition extracts metadata from path text. It has no filesystem,
// database, network or media-service dependencies.
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

type Source string

const (
	Filename        Source = "filename"
	CleanFilename   Source = "cleanFilename"
	ParentDirectory Source = "parentDirectory"
	Directories     Source = "directories"
	RelativePath    Source = "relativePath"
	AbsolutePath    Source = "absolutePath"
)

type Config struct {
	Code       string          `json:"code,omitempty"`
	SourceCode string          `json:"sourceCode,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Pattern    string          `json:"pattern,omitempty"`
	Capture    string          `json:"capture,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}
type Example struct {
	Path     string          `json:"path"`
	Root     string          `json:"root,omitempty"`
	Expected json.RawMessage `json:"expected"`
}
type ExampleResult struct {
	RuleID   string `json:"ruleId"`
	Path     string `json:"path"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Passed   bool   `json:"passed"`
	Error    string `json:"error"`
}
type Rule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Target      Target    `json:"target"`
	Source      Source    `json:"source"`
	Order       int       `json:"order"`
	Enabled     bool      `json:"enabled"`
	Kind        string    `json:"kind"`
	Config      Config    `json:"config"`
	Overwrite   bool      `json:"overwrite"`
	Examples    []Example `json:"examples"`
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
	RuleConfig     Rule              `json:"ruleConfig"`
	Target         Target            `json:"target"`
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
type Result struct {
	Metadata Metadata `json:"metadata"`
	Steps    []Step   `json:"steps"`
}
type Input struct {
	Path string
	Root string
}
type Descriptor struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Target      Target   `json:"target"`
	Description string   `json:"description"`
	Sources     []Source `json:"sources"`
	Config      Config   `json:"config"`
}

// Observer receives a completed immutable step, allowing an application to
// persist progress without coupling extraction to storage.
type Observer func(context.Context, Step) error

func Targets() []Target {
	return []Target{TMDBID, Title, MediaType, Year, Season, Episodes, ReleaseTag}
}
func Sources() []Source {
	return []Source{Filename, CleanFilename, ParentDirectory, Directories, RelativePath, AbsolutePath}
}

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
	Path  string  `json:"path"`
	Root  string  `json:"root,omitempty"`
	Rules *[]Rule `json:"rules,omitempty"`
}
