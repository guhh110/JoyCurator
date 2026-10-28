package app

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"joycurator/internal/auth"
	"joycurator/internal/fsops"
	"joycurator/internal/model"
	"joycurator/internal/rules"
	"joycurator/internal/scanner"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type endpoint func(http.ResponseWriter, *http.Request) error

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func read(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("请求内容无效")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("请求包含多余内容")
	}
	return nil
}
func errorReply(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
func (a *App) Handler(frontend fs.FS) http.Handler {
	mux := http.NewServeMux()
	add := func(pattern string, f endpoint) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := f(w, r); err != nil {
				errorReply(w, 400, err.Error())
			}
		})
	}
	add("GET /api/health", func(w http.ResponseWriter, r *http.Request) error {
		if err := a.db.PingContext(r.Context()); err != nil {
			return err
		}
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	add("GET /api/session", func(w http.ResponseWriter, r *http.Request) error {
		initialized, err := a.auth.Initialized(r.Context())
		if err != nil {
			return err
		}
		u, _ := a.auth.Session(r.Context(), cookie(r))
		reply(w, map[string]any{"initialized": initialized, "username": u.Username})
		return nil
	})
	for _, path := range []string{"setup", "login"} {
		add("POST /api/"+path, func(w http.ResponseWriter, r *http.Request) error {
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := read(r, &input); err != nil {
				return err
			}
			if strings.HasSuffix(r.URL.Path, "setup") {
				if err := a.auth.Setup(r.Context(), input.Username, input.Password); err != nil {
					return errors.New("无法初始化：用户名 1–64 字节，密码 12–72 字节，且只可初始化一次")
				}
			}
			token, u, err := a.auth.Login(r.Context(), input.Username, input.Password)
			if err != nil {
				errorReply(w, 401, "用户名或密码不正确")
				return nil
			}
			http.SetCookie(w, &http.Cookie{Name: "joycurator_session", Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionLifetime.Seconds())})
			reply(w, u)
			return nil
		})
	}
	add("POST /api/logout", func(w http.ResponseWriter, r *http.Request) error {
		if err := a.auth.Logout(r.Context(), cookie(r)); err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{Name: "joycurator_session", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode, Secure: a.secure})
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	add("GET /api/settings", func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.settings()
		if err != nil {
			return err
		}
		reply(w, struct {
			model.Settings
			HasToken bool     `json:"hasToken"`
			Roots    []string `json:"roots"`
		}{s, s.Token != "", a.access.Roots})
		return nil
	})
	add("PUT /api/settings", a.updateSettings)
	add("GET /api/browse", func(w http.ResponseWriter, r *http.Request) error {
		type item struct {
			Path string `json:"path"`
			Dir  bool   `json:"dir"`
		}
		items := []item{}
		path := r.URL.Query().Get("path")
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "roots" {
			return errors.New("目录浏览范围无效")
		}
		settings, err := a.settings()
		if err != nil {
			return err
		}
		roots := []string{settings.SourceRoot}
		if scope == "roots" {
			roots = a.access.Roots
		}
		if path != "" && scope != "roots" && !a.sourceAllowed(path, settings) {
			return errors.New("只能浏览源目录")
		}
		if path == "" {
			for _, p := range roots {
				items = append(items, item{p, true})
			}
		} else {
			root, rel, err := a.access.Root(path)
			if err != nil {
				return err
			}
			defer root.Close()
			stat, err := root.Lstat(rel)
			if err != nil || !stat.IsDir() {
				return errors.New("不是可访问的目录")
			}
			f, err := root.Open(rel)
			if err != nil {
				return err
			}
			defer f.Close()
			entries, err := f.ReadDir(-1)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".") || e.Type()&os.ModeSymlink != 0 || scope != "roots" && !a.sourceAllowed(filepath.Join(path, e.Name()), settings) {
					continue
				}
				if e.IsDir() || scanner.IsVideo(e.Name()) {
					items = append(items, item{filepath.Join(path, e.Name()), e.IsDir()})
				}
			}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Dir != items[j].Dir {
				return items[i].Dir
			}
			return items[i].Path < items[j].Path
		})
		reply(w, items)
		return nil
	})
	add("POST /api/scan", func(w http.ResponseWriter, r *http.Request) error {
		var input struct {
			Paths []string `json:"paths"`
		}
		if err := read(r, &input); err != nil {
			return err
		}
		items, err := a.scan(r.Context(), input.Paths)
		if err == nil {
			reply(w, items)
		}
		return err
	})
	add("GET /api/search", func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.settings()
		if err != nil {
			return err
		}
		client, err := a.client(s)
		if err != nil {
			return err
		}
		q := r.URL.Query()
		year, _ := strconv.Atoi(q.Get("year"))
		if q.Get("type") != "movie" && q.Get("type") != "tv" || strings.TrimSpace(q.Get("q")) == "" {
			return errors.New("请输入片名并选择类型")
		}
		items, err := client.Search(r.Context(), q.Get("type"), q.Get("q"), year)
		if err == nil {
			reply(w, items)
		}
		return err
	})
	add("GET /api/seasons", func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.settings()
		if err != nil {
			return err
		}
		client, err := a.client(s)
		if err != nil {
			return err
		}
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		if id <= 0 {
			return errors.New("作品 ID 无效")
		}
		items, err := client.Seasons(r.Context(), id)
		if err == nil {
			reply(w, items)
		}
		return err
	})
	add("GET /api/episodes", func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.settings()
		if err != nil {
			return err
		}
		client, err := a.client(s)
		if err != nil {
			return err
		}
		id, _ := strconv.Atoi(r.URL.Query().Get("id"))
		season, _ := strconv.Atoi(r.URL.Query().Get("season"))
		if id <= 0 || season < 0 || season > 999 {
			return errors.New("作品或季号无效")
		}
		items, err := client.SeasonEpisodes(r.Context(), id, season)
		if err == nil {
			reply(w, items)
		}
		return err
	})
	add("POST /api/preview", func(w http.ResponseWriter, r *http.Request) error {
		var input PreviewRequest
		if err := read(r, &input); err != nil {
			return err
		}
		p, err := a.preview(r.Context(), input)
		if err == nil {
			reply(w, p)
		}
		return err
	})
	add("POST /api/execute", func(w http.ResponseWriter, r *http.Request) error {
		var input struct {
			ID string `json:"id"`
		}
		if err := read(r, &input); err != nil {
			return err
		}
		run, err := a.execute(input.ID)
		if err == nil {
			reply(w, run)
		}
		return err
	})
	add("GET /api/runs", func(w http.ResponseWriter, r *http.Request) error {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		if page > 1000000 {
			page = 1000000
		}
		items, err := a.records(page)
		if err == nil {
			reply(w, items)
		}
		return err
	})
	add("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) error {
		run, err := a.record(r.PathValue("id"))
		if err == nil {
			reply(w, run)
		}
		return err
	})
	for _, action := range []string{"retry", "undo"} {
		add("POST /api/runs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) error {
			run, err := a.resume(r.PathValue("id"), strings.HasSuffix(r.URL.Path, "/undo"))
			if err == nil {
				reply(w, run)
			}
			return err
		})
	}
	add("POST /api/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.running != r.PathValue("id") || a.cancel == nil {
			return errors.New("任务未在运行")
		}
		a.cancel()
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			errorReply(w, 404, "接口不存在")
			return
		}
		if frontend == nil {
			http.Error(w, "前端请访问 http://localhost:5175；单文件部署请使用 make build", 503)
			return
		}
		http.FileServer(http.FS(frontend)).ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method != "GET" && r.Method != "HEAD" {
				if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
					errorReply(w, 403, "拒绝跨站请求")
					return
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					u, err := url.Parse(origin)
					if err != nil || u.Host != r.Host {
						errorReply(w, 403, "拒绝跨站请求")
						return
					}
				}
			}
			public := r.URL.Path == "/api/session" || r.URL.Path == "/api/health" || r.URL.Path == "/api/setup" || r.URL.Path == "/api/login"
			if !public {
				if _, err := a.auth.Session(r.Context(), cookie(r)); err != nil {
					errorReply(w, 401, "请先登录")
					return
				}
			}
		}
		defer func() {
			if err := recover(); err != nil {
				slog.Error("请求失败", "path", r.URL.Path)
				errorReply(w, 500, "服务暂时不可用")
			}
		}()
		mux.ServeHTTP(w, r)
	})
}
func cookie(r *http.Request) string {
	c, err := r.Cookie("joycurator_session")
	if err != nil {
		return ""
	}
	return c.Value
}
func (a *App) updateSettings(w http.ResponseWriter, r *http.Request) error {
	var input struct {
		model.Settings
		TMDBToken *string `json:"tmdbToken"`
	}
	if err := read(r, &input); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running != "" {
		return errBusy
	}
	old, err := a.settings()
	if err != nil {
		return err
	}
	if input.Version != old.Version {
		return errors.New("设置已更新，请重新加载")
	}
	if err = rules.Validate(input.Rules); err != nil {
		return err
	}
	if input.Language == "" {
		input.Language = "zh-CN"
	}
	if _, err = a.client(input.Settings); err != nil {
		return err
	}
	for _, p := range []string{input.SourceRoot, input.TargetRoot} {
		if p == "" || !filepath.IsAbs(p) {
			return errors.New("源目录和目标目录必须填写绝对路径")
		}
		info, e := a.access.Stat(p)
		if e != nil || !info.IsDir() {
			return errors.New("目录不存在或未授权：" + p)
		}
	}
	input.SourceRoot, err = filepath.EvalSymlinks(a.access.Canonical(input.SourceRoot))
	if err != nil {
		return errors.New("无法解析源目录")
	}
	input.TargetRoot, err = filepath.EvalSymlinks(a.access.Canonical(input.TargetRoot))
	if err != nil {
		return errors.New("无法解析目标目录")
	}
	for _, path := range []string{input.SourceRoot, input.TargetRoot} {
		if _, err := a.access.Stat(path); err != nil {
			return errors.New("目录解析后超出授权范围")
		}
	}
	if fsops.Within(input.TargetRoot, input.SourceRoot) {
		return errors.New("源目录不能位于目标目录内或与其相同")
	}

	input.Token = old.Token
	if input.TMDBToken != nil {
		input.Token = strings.TrimSpace(*input.TMDBToken)
	}
	input.Version++
	if len(input.Rules.Categories) == 0 {
		input.Rules.Categories = model.DefaultCategories()
	}
	if _, err = a.db.Exec("UPDATE config SET value=?,token=? WHERE id=1", jsonText(input.Settings), input.Token); err != nil {
		return err
	}
	a.plans = map[string]*Plan{}
	reply(w, map[string]bool{"ok": true})
	return nil
}
