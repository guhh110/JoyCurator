package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"joycurator/internal/recognition"
)

type Category struct {
	Name      string   `json:"name"`
	Type      string   `json:"type,omitempty"`
	Countries []string `json:"countries"`
	Genres    []string `json:"genres"`
	YearMin   int      `json:"yearMin,omitempty"`
	YearMax   int      `json:"yearMax,omitempty"`
}
type Rules struct {
	MovieFolder string     `json:"movieFolder"`
	MovieFile   string     `json:"movieFile"`
	TVFolder    string     `json:"tvFolder"`
	TVFile      string     `json:"tvFile"`
	Categories  []Category `json:"categories"`
}
type Settings struct {
	Version    int    `json:"version"`
	SourceRoot string `json:"sourceRoot"`
	TargetRoot string `json:"targetRoot"`
	Token      string `json:"-"`
	ProxyURL   string `json:"proxyURL"`
	Language   string `json:"language"`
	Rules      Rules  `json:"rules"`
}

func DefaultSettings() Settings {
	return Settings{Version: 1, Language: "zh-CN", Rules: Rules{
		MovieFolder: "{category}/{title} ({year})", MovieFile: "{title} ({year}) - {resolution}.{ext}",
		TVFolder: "{category}/{title} ({year})/Season {season:02}", TVFile: "{title} - {episode_range} - {resolution}.{ext}", Categories: DefaultCategories(),
	}}
}

// DefaultCategories matches special genres before broad country rules.
// Each media type ends with an explicit catch-all category.
func DefaultCategories() []Category {
	east := []string{"CN", "HK", "TW", "SG"}
	west := []string{"US", "GB", "FR", "DE", "IT", "ES", "CA", "AU", "NZ"}
	return []Category{
		{Name: "电影/纪录片", Type: "movie", Countries: []string{}, Genres: []string{"纪录片", "Documentary"}},
		{Name: "电影/动画电影", Type: "movie", Countries: []string{}, Genres: []string{"动画", "Animation"}},
		{Name: "电影/华语电影", Type: "movie", Countries: east, Genres: []string{}},
		{Name: "电影/日韩电影", Type: "movie", Countries: []string{"JP", "KR"}, Genres: []string{}},
		{Name: "电影/欧美电影", Type: "movie", Countries: west, Genres: []string{}},
		{Name: "电影/其他电影", Type: "movie", Countries: []string{}, Genres: []string{}},
		{Name: "电视剧/儿童", Type: "tv", Countries: []string{}, Genres: []string{"儿童", "Kids"}},
		{Name: "电视剧/国漫", Type: "tv", Countries: east, Genres: []string{"动画", "Animation"}},
		{Name: "电视剧/日番", Type: "tv", Countries: []string{"JP"}, Genres: []string{"动画", "Animation"}},
		{Name: "电视剧/国产剧", Type: "tv", Countries: east, Genres: []string{}},
		{Name: "电视剧/欧美剧", Type: "tv", Countries: west, Genres: []string{}},
		{Name: "电视剧/日韩剧", Type: "tv", Countries: []string{"JP", "KR"}, Genres: []string{}},
		{Name: "电视剧/其他剧集", Type: "tv", Countries: []string{}, Genres: []string{}},
	}
}

type Actor struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// Entry is a TMDB movie or show. It is a value object: only what is needed
// later is copied into a record.
type Entry struct {
	TMDBID        int      `json:"tmdbId"`
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle"`
	Year          int      `json:"year"`
	Overview      string   `json:"overview"`
	Date          string   `json:"date"`
	Genres        []string `json:"genres"`
	Countries     []string `json:"countries"`
	Cast          []Actor  `json:"cast"`
	PosterPath    string   `json:"posterPath"`
	BackdropPath  string   `json:"backdropPath"`
}

// Source contains the inputs used to render naming rules.
type Source struct {
	Path     string               `json:"path"`
	Metadata recognition.Metadata `json:"metadata"`
	Entry    *Entry               `json:"entry,omitempty"`
	Season   int                  `json:"season"`
	Episodes []int                `json:"episodes"`
}

type File struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	MTime    int64  `json:"mtime,string"`
	Season   int    `json:"season"`
	Episodes []int  `json:"episodes"`
}

func Key() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Season is one season entry of a show.
type Season struct {
	TMDBID     int    `json:"tmdbId"`
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Overview   string `json:"overview"`
	Date       string `json:"date"`
	PosterPath string `json:"posterPath"`
}

// Episode is one episode of a season.
type Episode struct {
	Season   int    `json:"season"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	AirDate  string `json:"airDate"`
	Overview string `json:"overview"`
	Still    string `json:"stillPath"`
	TMDBID   int    `json:"tmdbId"`
}

// MediaLayout records confirmed output directories. An empty WorkDir needs confirmation.
type MediaLayout struct {
	WorkDir     string `json:"workDir"`
	SeasonDir   string `json:"seasonDir"`
	MoviePrefix bool   `json:"moviePrefix"`
	Manual      bool   `json:"manual"`
}

// MediaMapping is the durable bridge between organization and scraping.
type MediaMapping struct {
	Entry    *Entry        `json:"entry"`
	Layout   MediaLayout   `json:"layout"`
	Original *MediaMapping `json:"original,omitempty"`
	Path     string        `json:"path"`
	RunID    string        `json:"runId"`
	Source   string        `json:"source"`
	Type     string        `json:"type"`
	TMDBID   int           `json:"tmdbId"`
	Season   int           `json:"season"`
	Episodes []int         `json:"episodes"`
	State    string        `json:"state"`
	Identity string        `json:"identity"`
	Size     int64         `json:"size"`
	MTime    int64         `json:"mtime"`
}

type Artwork struct {
	Path     string  `json:"path"`
	Language string  `json:"language"`
	Score    float64 `json:"score"`
	Votes    int     `json:"votes"`
	Kind     string  `json:"kind"`
}

var ErrNotFound = errors.New("TMDB 条目或季集不存在")

type CleanupRequest struct {
	Before      string `json:"before"`
	Traces      bool   `json:"traces"`
	Cache       bool   `json:"cache"`
	CacheBefore int64  `json:"cacheBefore"`
}
type CleanupCounts struct {
	Traces int64 `json:"traces"`
	Cache  int64 `json:"cache"`
}
type CleanupPreview struct {
	Request CleanupRequest `json:"request"`
	CleanupCounts
}
