package main

import (
	"net/http"
)

const adminCookie = "__Host-mind-offline-admin"

func (a *app) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer(r) == "" {
			cookie, err := r.Cookie(adminCookie)
			if err == nil {
				if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != "https://"+r.Host && r.Header.Get("Origin") != "http://"+r.Host {
					fail(w, 403, "请求来源不正确")
					return
				}
				r = r.Clone(r.Context())
				r.Header.Set("Authorization", "Bearer "+cookie.Value)
			}
		}
		a.customer(func(w http.ResponseWriter, r *http.Request) {
			if a.adminAccountID == "" || r.Context().Value(guestKey) != a.adminAccountID {
				fail(w, 403, "仅管理员可以访问")
				return
			}
			next(w, r)
		})(w, r)
	}
}
func (a *app) openAdminSession(w http.ResponseWriter, r *http.Request) {
	if a.adminAccountID == "" || r.Context().Value(guestKey) != a.adminAccountID {
		fail(w, 403, "仅管理员可以访问")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: bearer(r), Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *app) adminMe(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"id": a.adminAccountID, "username": "cole", "is_admin": true})
}
func (a *app) adminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	a.logoutCustomer(w, r)
}
func (a *app) adminPage(files http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(adminCookie)
		if err != nil || len(cookie.Value) != 64 || a.adminAccountID == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		var ok bool
		err = a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM customer_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT a.disabled AND a.id=$2)", hash(cookie.Value), a.adminAccountID).Scan(&ok)
		if err != nil {
			a.internal(w, err)
			return
		}
		if !ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		files.ServeHTTP(w, r)
	})
}
