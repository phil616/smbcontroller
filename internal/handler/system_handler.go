package handler

import (
	"net/http"

	"smb-controller/internal/models"
)

func (h *Handler) systemStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.services.System.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) systemReload(w http.ResponseWriter, r *http.Request) {
	if err := h.services.System.Reload(r.Context()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) systemRestart(w http.ResponseWriter, r *http.Request) {
	if err := h.services.System.Restart(r.Context()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) systemConf(w http.ResponseWriter, r *http.Request) {
	conf, err := h.services.System.Conf()
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"content": conf})
}

func (h *Handler) systemSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.services.System.Settings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) updateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var settings models.SystemSettings
	if err := decodeJSON(r, &settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.services.System.UpdateSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
