// Package model defines the in-memory workspace, the TMDB value objects and
// the two persisted tables: organize records and their artifacts.
package model

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
	NFO        bool   `json:"nfo"`
	Images     bool   `json:"images"`
	Rules      Rules  `json:"rules"`
}

func DefaultSettings() Settings {
	return Settings{Version: 1, Language: "zh-CN", NFO: true, Images: true, Rules: Rules{
		MovieFolder: "{category}/{title} ({year}) [tmdb-{tmdb_id}]", MovieFile: "{title} ({year}) - {resolution}.{ext}",
		TVFolder: "{category}/{title} ({year}) [tmdb-{tmdb_id}]/Season {season:02}", TVFile: "{title} - {episode_range} - {resolution}.{ext}", Categories: DefaultCategories(),
	}}
}

// DefaultCategories is the built-in classification: documentaries and anime
// first, then by origin country, with a catch-all per media type so nothing
// ends up in 未分类. Genre names list both the Chinese and English label so the
// rules keep working when the metadata language changes.
func DefaultCategories() []Category {
	east := []string{"CN", "HK", "TW", "SG"}
	west := []string{"US", "GB", "FR", "DE", "IT", "ES", "CA", "AU", "NZ"}
	return []Category{
		{Name: "电影/纪录片", Type: "movie", Countries: []string{}, Genres: []string{"纪录片", "Documentary"}},
		{Name: "剧集/纪录片", Type: "tv", Countries: []string{}, Genres: []string{"纪录片", "Documentary"}},
		{Name: "电影/动画电影", Type: "movie", Countries: []string{}, Genres: []string{"动画", "Animation"}},
		{Name: "电影/华语电影", Type: "movie", Countries: east, Genres: []string{}},
		{Name: "电影/日韩电影", Type: "movie", Countries: []string{"JP", "KR"}, Genres: []string{}},
		{Name: "电影/欧美电影", Type: "movie", Countries: west, Genres: []string{}},
		{Name: "电影/其他电影", Type: "movie", Countries: []string{}, Genres: []string{}},
		{Name: "剧集/动漫", Type: "tv", Countries: []string{}, Genres: []string{"动画", "Animation"}},
		{Name: "剧集/国产剧", Type: "tv", Countries: east, Genres: []string{}},
		{Name: "剧集/日韩剧", Type: "tv", Countries: []string{"JP", "KR"}, Genres: []string{}},
		{Name: "剧集/欧美剧", Type: "tv", Countries: west, Genres: []string{}},
		{Name: "剧集/其他剧集", Type: "tv", Countries: []string{}, Genres: []string{}},
	}
}

// Parsed is what the file name and its folders yield.
type Parsed struct {
	Title      string `json:"title"`
	Year       int    `json:"year"`
	Type       string `json:"type"`
	Season     int    `json:"season"`
	Episodes   []int  `json:"episodes"`
	TMDBID     int    `json:"tmdbId"`
	ReleaseTag string `json:"releaseTag"`
}

// Probe is what ffprobe reported about the file.
type Probe struct {
	Resolution string  `json:"resolution"`
	Codec      string  `json:"codec"`
	Duration   float64 `json:"duration"`
	Warning    string  `json:"warning,omitempty"`
}

type Actor struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// Entry is a TMDB movie or show. It is a value object: only what is needed
// later is copied into a record.
type Entry struct {
	ID            int64    `json:"-"`
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

// Source states. The workspace lives in memory; organized files leave it.
const (
	StateUnpaired = "unpaired"
	StatePaired   = "paired"
)

// Source is one file in the pairing workspace. It is never persisted.
type Source struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	MTime      int64  `json:"mtime"`
	Parsed     Parsed `json:"parsed"`
	Probe      Probe  `json:"probe"`
	EntryID    int64  `json:"entryId"`
	Entry      *Entry `json:"entry,omitempty"`
	Season     int    `json:"season"`
	Episodes   []int  `json:"episodes"`
	Version    string `json:"version"`
	State      string `json:"state"`
	TargetPath string `json:"targetPath"`
	Revision   int    `json:"revision"`
}

// Operation is one file action the organizer knows how to run and reverse.
type Operation struct {
	ID             int64      `json:"id"`
	Kind           string     `json:"kind"`
	Source         string     `json:"source"`
	Target         string     `json:"target"`
	Mode           string     `json:"mode"`
	Size           int64      `json:"size"`
	MTime          int64      `json:"mtime"`
	Content        []byte     `json:"content,omitempty"`
	URL            string     `json:"url,omitempty"`
	Status         string     `json:"status"`
	Error          string     `json:"error"`
	Hash           string     `json:"hash"`
	Temp           string     `json:"temp"`
	Identity       string     `json:"identity"`
	SourceIdentity string     `json:"sourceIdentity"`
	Undo           *Operation `json:"undo,omitempty"`
}
