package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"smb-controller/internal/service"
	"smb-controller/internal/session"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	services *service.Services
	sessions *session.Store
	webFS    fs.FS
}

func RegisterRoutes(r chi.Router, services *service.Services, sessions *session.Store, webFS fs.FS, trustedDomains []string) {
	h := &Handler{services: services, sessions: sessions, webFS: webFS}
	r.Use(TrustedDomainMiddleware(trustedDomains))

	r.Get("/", h.serveFile("index.html"))
	r.Get("/setup", h.serveFile("setup.html"))
	r.Get("/login", h.serveFile("login.html"))
	static := http.StripPrefix("/static/", http.FileServer(http.FS(mustSub(webFS, "static"))))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		static.ServeHTTP(w, r)
	}))

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/setup/status", h.setupStatus)
		api.Post("/setup/init", h.setupInit)
		api.Post("/auth/login", h.login)

		api.Group(func(protected chi.Router) {
			protected.Use(AuthMiddleware(sessions))
			protected.Post("/auth/logout", h.logout)
			protected.Get("/auth/me", h.me)

			protected.Get("/volumes", h.listVolumes)
			protected.Post("/volumes", h.createVolume)
			protected.Get("/volumes/{id}", h.getVolume)
			protected.Put("/volumes/{id}", h.updateVolume)
			protected.Delete("/volumes/{id}", h.deleteVolume)
			protected.Post("/volumes/{id}/repair", h.repairVolumePermissions)

			protected.Get("/users", h.listUsers)
			protected.Post("/users", h.createUser)
			protected.Post("/users/temporary", h.createTemporaryUser)
			protected.Get("/users/{id}", h.getUser)
			protected.Put("/users/{id}", h.updateUser)
			protected.Post("/users/{id}/password", h.changeUserPassword)
			protected.Put("/users/{id}/enabled", h.setUserEnabled)
			protected.Delete("/users/{id}", h.deleteUser)

			protected.Get("/permissions", h.listPermissions)
			protected.Put("/permissions", h.setPermission)
			protected.Put("/permissions/bulk", h.bulkSetPermissions)
			protected.Delete("/permissions", h.deletePermission)

			protected.Get("/system/status", h.systemStatus)
			protected.Post("/system/reload", h.systemReload)
			protected.Post("/system/restart", h.systemRestart)
			protected.Get("/system/conf", h.systemConf)
			protected.Get("/system/settings", h.systemSettings)
			protected.Put("/system/settings", h.updateSystemSettings)
		})
	})
}

func (h *Handler) serveFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, h.webFS, name)
	}
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
