package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"joycurator/internal/auth"
	"joycurator/internal/matching"
	"joycurator/internal/organizer"
	"joycurator/internal/planning"
	"joycurator/internal/recognition"
)

type Dependencies struct {
	Auth        *auth.Service
	Secure      bool
	Health      func(context.Context) error
	Config      *ConfigService
	Matching    *matching.Service
	Recognition *recognition.Service
	Workspace   *WorkspaceService
	Files       *FileService
	Planning    *planning.Service
	Organizer   *organizer.Service
}
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

func respond(w http.ResponseWriter, value any, err error) error {
	if err == nil {
		reply(w, value)
	}
	return err
}
func NewHTTP(a Dependencies, frontend fs.FS) http.Handler {
	mux := http.NewServeMux()
	add := func(pattern string, f endpoint) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := f(w, r); err != nil {
				errorReply(w, 400, err.Error())
			}
		})
	}
	systemRoutes(a, add)
	libraryRoutes(a, add)
	catalogRoutes(a, add)
	organizationRoutes(a, add)
	identificationRoutes(a, add)
	batchRoutes(a, add)
	extractionRoutes(a, add)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			errorReply(w, 404, "接口不存在")
			return
		}
		if frontend == nil {
			http.Error(w, "前端请访问 http://localhost:5175；单文件部署请使用 make build", 503)
			return
		}
		// Client pages have real URLs. Only extensionless GET/HEAD navigation
		// falls back to the SPA entry; missing assets and APIs retain their 404.
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && filepath.Ext(r.URL.Path) == "" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			name := strings.TrimPrefix(r.URL.Path, "/")
			if _, err := fs.Stat(frontend, name); err != nil {
				request := r.Clone(r.Context())
				request.URL.Path = "/"
				http.FileServer(http.FS(frontend)).ServeHTTP(w, request)
				return
			}
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
				if _, err := a.Auth.Session(r.Context(), cookie(r)); err != nil {
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
