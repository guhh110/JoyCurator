package tmdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"joycurator/internal/model"
)

// Client never returns upstream response bodies or credentials in errors.
type Client struct {
	DB       *sql.DB
	HTTP     *http.Client
	BaseURL  string
	Token    string
	Language string
	// RetryWait is the base backoff between attempts. Tests shorten it.
	RetryWait time.Duration
	mu        sync.Mutex
	next      time.Time
}

func New(db *sql.DB, token, proxy, language string) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
			return nil, errors.New("代理地址必须是 http、https 或 socks5 URL")
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &Client{DB: db, HTTP: &http.Client{Transport: transport, Timeout: 20 * time.Second}, BaseURL: "https://api.themoviedb.org/3", Token: token, Language: language, RetryWait: time.Second}, nil
}

func (c *Client) retryWait() time.Duration {
	if c.RetryWait > 0 {
		return c.RetryWait
	}
	return time.Second
}

// minInterval spaces request starts. TMDB allows roughly 50 requests per
// second; 10/s keeps batch candidate lookups responsive without risking 429s.
const minInterval = 100 * time.Millisecond

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	delay := time.Until(c.next)
	if delay < 0 {
		delay = 0
	}
	c.next = time.Now().Add(delay + minInterval)
	c.mu.Unlock()
	return wait(ctx, delay)
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (c *Client) Get(ctx context.Context, path string, q url.Values, dst any) error {
	if c.Token == "" {
		return errors.New("请先配置 TMDB API Read Access Token")
	}
	if q == nil {
		q = url.Values{}
	}
	if q.Get("language") == "" {
		q.Set("language", c.Language)
	}
	key := path + "?" + q.Encode()
	var cached []byte
	if c.DB != nil {
		err := c.DB.QueryRowContext(ctx, "SELECT value FROM tmdb_cache WHERE key=? AND expires_at>?", key, time.Now().Unix()).Scan(&cached)
		if err == nil && json.Unmarshal(cached, dst) == nil {
			return nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.New("无法读取 TMDB 缓存")
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.throttle(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+path+"?"+q.Encode(), nil)
		if err != nil {
			return errors.New("TMDB 请求配置无效")
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < 2 {
				if err = wait(ctx, time.Duration(attempt+1)*c.retryWait()); err != nil {
					return err
				}
				continue
			}
			return errors.New("TMDB 网络连接失败，请检查代理或网络")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			if attempt < 2 {
				delay := time.Duration(attempt+1) * c.retryWait()
				if seconds, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && seconds > 0 {
					delay = min(time.Duration(seconds)*time.Second, 30*time.Second)
				}
				if err = wait(ctx, delay); err != nil {
					return err
				}
				continue
			}
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return errors.New("TMDB 凭据无效或没有访问权限")
		}
		if resp.StatusCode == 404 {
			return errors.New("TMDB 条目或季集不存在")
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("TMDB 请求失败（HTTP %d）", resp.StatusCode)
		}
		if readErr != nil || json.Unmarshal(data, dst) != nil {
			return errors.New("TMDB 返回的数据格式错误")
		}
		if c.DB != nil {
			_, err = c.DB.ExecContext(ctx, "INSERT INTO tmdb_cache(key,value,expires_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,expires_at=excluded.expires_at", key, data, time.Now().Add(24*time.Hour).Unix())
			if err != nil {
				return errors.New("无法保存 TMDB 缓存")
			}
		}
		return nil
	}
	return errors.New("TMDB 服务暂时不可用")
}

type rawItem struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	Name          string   `json:"name"`
	OriginalTitle string   `json:"original_title"`
	OriginalName  string   `json:"original_name"`
	Overview      string   `json:"overview"`
	ReleaseDate   string   `json:"release_date"`
	FirstAirDate  string   `json:"first_air_date"`
	Poster        string   `json:"poster_path"`
	Backdrop      string   `json:"backdrop_path"`
	OriginCountry []string `json:"origin_country"`
	Genres        []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Countries []struct {
		Code string `json:"iso_3166_1"`
	} `json:"production_countries"`
	Credits struct {
		Cast []struct {
			Name      string `json:"name"`
			Character string `json:"character"`
		} `json:"cast"`
	} `json:"credits"`
}

func normalize(raw rawItem, kind string) model.Entry {
	item := model.Entry{TMDBID: raw.ID, Type: kind, Title: raw.Title, OriginalTitle: raw.OriginalTitle, Overview: raw.Overview, Date: raw.ReleaseDate, PosterPath: raw.Poster, BackdropPath: raw.Backdrop, Genres: []string{}, Countries: []string{}, Cast: []model.Actor{}}
	if kind == "tv" {
		item.Title = raw.Name
		item.OriginalTitle = raw.OriginalName
		item.Date = raw.FirstAirDate
		item.Countries = raw.OriginCountry
	}
	if len(item.Date) >= 4 {
		item.Year, _ = strconv.Atoi(item.Date[:4])
	}
	if item.Title == "" {
		item.Title = item.OriginalTitle
	}
	for _, g := range raw.Genres {
		item.Genres = append(item.Genres, g.Name)
	}
	for _, co := range raw.Countries {
		if !contains(item.Countries, co.Code) {
			item.Countries = append(item.Countries, co.Code)
		}
	}
	for i, actor := range raw.Credits.Cast {
		if i >= 20 {
			break
		}
		item.Cast = append(item.Cast, model.Actor{Name: actor.Name, Role: actor.Character})
	}
	return item
}
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func (c *Client) Search(ctx context.Context, kind, query string, year int) ([]model.Entry, error) {
	if kind != "movie" && kind != "tv" {
		return nil, errors.New("媒体类型必须是 movie 或 tv")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("搜索词不能为空")
	}
	q := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		key := "year"
		if kind == "tv" {
			key = "first_air_date_year"
		}
		q.Set(key, strconv.Itoa(year))
	}
	var response struct {
		Results []rawItem `json:"results"`
	}
	if err := c.Get(ctx, "/search/"+kind, q, &response); err != nil {
		return nil, err
	}
	items := []model.Entry{}
	for _, raw := range response.Results {
		items = append(items, normalize(raw, kind))
	}
	return items, nil
}
func (c *Client) Detail(ctx context.Context, kind string, id int) (model.Entry, error) {
	if (kind != "movie" && kind != "tv") || id < 1 {
		return model.Entry{}, errors.New("TMDB ID 或媒体类型无效")
	}
	path := fmt.Sprintf("/%s/%d", kind, id)
	var raw rawItem
	if err := c.Get(ctx, path, url.Values{"append_to_response": {"credits"}}, &raw); err != nil {
		return model.Entry{}, err
	}
	item := normalize(raw, kind)
	missingTitle := raw.Title == "" && raw.Name == ""
	if missingTitle || item.Overview == "" {
		var english rawItem
		if err := c.Get(ctx, path, url.Values{"language": {"en-US"}}, &english); err != nil {
			return item, err
		}
		fallback := normalize(english, kind)
		if missingTitle {
			item.Title = fallback.Title
		}
		if item.Overview == "" {
			item.Overview = fallback.Overview
		}
	}
	return item, nil
}
func (c *Client) Image(ctx context.Context, path string) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.ContainsAny(path, "?#\\") {
		return nil, errors.New("TMDB 图片路径无效")
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.throttle(ctx); err != nil {
			return nil, err
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://image.tmdb.org/t/p/original"+path, nil)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < 2 {
				_ = wait(ctx, c.retryWait())
				continue
			}
			return nil, errors.New("图片下载失败")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 30<<20))
		resp.Body.Close()
		if resp.StatusCode != 200 || readErr != nil {
			if attempt < 2 {
				_ = wait(ctx, c.retryWait())
				continue
			}
			return nil, errors.New("图片下载失败")
		}
		if len(data) == 0 || len(data) >= 30<<20 || !strings.HasPrefix(http.DetectContentType(data), "image/") {
			return nil, errors.New("返回内容不是有效图片")
		}
		return data, nil
	}
	return nil, errors.New("图片下载失败")
}

// Season is one season entry of a show.
type Season struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

// Episode is one episode of a season.
type Episode struct {
	Season   int    `json:"season"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	AirDate  string `json:"airDate"`
	Overview string `json:"overview"`
	Still    string `json:"stillPath"`
}

// Seasons lists the seasons of a show.
func (c *Client) Seasons(ctx context.Context, id int) ([]Season, error) {
	var raw struct {
		Seasons []struct {
			SeasonNumber int    `json:"season_number"`
			Name         string `json:"name"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"seasons"`
	}
	if err := c.Get(ctx, fmt.Sprintf("/tv/%d", id), nil, &raw); err != nil {
		return nil, err
	}
	seasons := make([]Season, 0, len(raw.Seasons))
	for _, item := range raw.Seasons {
		seasons = append(seasons, Season{Number: item.SeasonNumber, Name: item.Name, Count: item.EpisodeCount})
	}
	sort.Slice(seasons, func(i, j int) bool { return seasons[i].Number < seasons[j].Number })
	return seasons, nil
}

// SeasonEpisodes lists the episodes of one season.
func (c *Client) SeasonEpisodes(ctx context.Context, id, season int) ([]Episode, error) {
	var raw struct {
		Episodes []struct {
			EpisodeNumber int    `json:"episode_number"`
			Name          string `json:"name"`
			AirDate       string `json:"air_date"`
			Overview      string `json:"overview"`
			StillPath     string `json:"still_path"`
		} `json:"episodes"`
	}
	if err := c.Get(ctx, fmt.Sprintf("/tv/%d/season/%d", id, season), nil, &raw); err != nil {
		return nil, err
	}
	episodes := make([]Episode, 0, len(raw.Episodes))
	for _, item := range raw.Episodes {
		episodes = append(episodes, Episode{
			Season:   season,
			Number:   item.EpisodeNumber,
			Name:     item.Name,
			AirDate:  item.AirDate,
			Overview: item.Overview,
			Still:    item.StillPath,
		})
	}
	return episodes, nil
}
