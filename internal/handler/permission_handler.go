package handler

import "net/http"

func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.services.SMB.ListPermissions(r.Context(), queryInt64(r, "user_id"), queryInt64(r, "volume_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, perms)
}

func (h *Handler) setPermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   int64  `json:"user_id"`
		VolumeID int64  `json:"volume_id"`
		Access   string `json:"access"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	result, err := h.services.SMB.SetPermission(r.Context(), req.UserID, req.VolumeID, req.Access)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) bulkSetPermissions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserIDs   []int64 `json:"user_ids"`
		VolumeIDs []int64 `json:"volume_ids"`
		Access    string  `json:"access"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	result, err := h.services.SMB.BulkSetPermissions(r.Context(), req.UserIDs, req.VolumeIDs, req.Access)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) deletePermission(w http.ResponseWriter, r *http.Request) {
	userID := queryInt64(r, "user_id")
	volumeID := queryInt64(r, "volume_id")
	if userID <= 0 || volumeID <= 0 {
		writeError(w, http.StatusBadRequest, "user_id and volume_id are required")
		return
	}
	result, err := h.services.SMB.SetPermission(r.Context(), userID, volumeID, "none")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
