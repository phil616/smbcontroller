package handler

import (
	"net/http"
	"time"
)

func (h *Handler) setupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, err := h.services.Admin.IsInitialized(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"initialized": initialized})
}

func (h *Handler) setupInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.services.Admin.Init(r.Context(), req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Initialized successfully"})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	admin, err := h.services.Admin.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	sess, err := h.sessions.Create(admin.ID, admin.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// #nosec G124 -- Secure is enabled for HTTPS or trusted loopback reverse-proxy HTTPS; local HTTP development must still be usable.
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  sess.ExpiresAt,
		MaxAge:   int(h.sessions.TTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "username": admin.Username})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_id"); err == nil {
		h.sessions.Delete(cookie.Value)
	}
	// #nosec G124 -- Secure follows the same conditional HTTPS policy as the login cookie.
	http.SetCookie(w, &http.Cookie{Name: "session_id", Value: "", Path: "/", HttpOnly: true, Secure: isSecureRequest(r), SameSite: http.SameSiteStrictMode, Expires: time.Unix(0, 0), MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	sess, _ := CurrentSession(r)
	writeJSON(w, http.StatusOK, sess)
}
