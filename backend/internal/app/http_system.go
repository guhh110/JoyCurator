package app

import (
	"errors"
	"net/http"
	"strings"

	"joycurator/internal/auth"
)

func systemRoutes(a Dependencies, add func(string, endpoint)) {
	add("GET /api/health", func(w http.ResponseWriter, r *http.Request) error {
		if err := a.Health(r.Context()); err != nil {
			return err
		}
		reply(w, map[string]bool{"ok": true})
		return nil
	})
	add("GET /api/session", func(w http.ResponseWriter, r *http.Request) error {
		initialized, err := a.Auth.Initialized(r.Context())
		if err != nil {
			return err
		}
		u, _ := a.Auth.Session(r.Context(), cookie(r))
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
				if err := a.Auth.Setup(r.Context(), input.Username, input.Password); err != nil {
					return errors.New("无法初始化：用户名 1–64 字节，密码 12–72 字节，且只可初始化一次")
				}
			}
			token, u, err := a.Auth.Login(r.Context(), input.Username, input.Password)
			if err != nil {
				errorReply(w, 401, "用户名或密码不正确")
				return nil
			}
			http.SetCookie(w, &http.Cookie{Name: "joycurator_session", Value: token, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionLifetime.Seconds())})
			reply(w, u)
			return nil
		})
	}
	add("POST /api/logout", func(w http.ResponseWriter, r *http.Request) error {
		if err := a.Auth.Logout(r.Context(), cookie(r)); err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{Name: "joycurator_session", Path: "/", HttpOnly: true, MaxAge: -1, SameSite: http.SameSiteStrictMode, Secure: a.Secure})
		reply(w, map[string]bool{"ok": true})
		return nil
	})

	add("GET /api/settings", func(w http.ResponseWriter, r *http.Request) error { v, e := a.Config.View(); return respond(w, v, e) })
	add("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) error {
		var in ConfigUpdate
		if e := read(r, &in); e != nil {
			return e
		}
		e := a.Config.Update(in)
		return respond(w, map[string]bool{"ok": true}, e)
	})
}
