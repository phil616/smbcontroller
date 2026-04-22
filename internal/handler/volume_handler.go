package handler

import (
	"net/http"

	"smb-controller/internal/service"
)

func (h *Handler) listVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := h.services.SMB.ListVolumes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, volumes)
}

func (h *Handler) createVolume(w http.ResponseWriter, r *http.Request) {
	var req service.CreateVolumeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	volume, err := h.services.SMB.CreateVolume(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, volume)
}

func (h *Handler) getVolume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	volume, err := h.services.SMB.GetVolume(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if volume == nil {
		writeError(w, http.StatusNotFound, "volume not found")
		return
	}
	writeJSON(w, http.StatusOK, volume)
}

func (h *Handler) updateVolume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req service.UpdateVolumeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	volume, err := h.services.SMB.UpdateVolume(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, volume)
}

func (h *Handler) deleteVolume(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.services.SMB.DeleteVolume(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
