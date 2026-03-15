package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

type APIHandler struct {
	costing *service.CostingService
}

func NewAPIHandler(costing *service.CostingService) *APIHandler {
	return &APIHandler{costing: costing}
}

func (h *APIHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/users/", h.handleUsers)
}

func (h *APIHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	parts := splitPath(path)

	if len(parts) < 2 {
		writeAPIError(w, http.StatusNotFound, "route not found")
		return
	}

	userID := parts[0]
	resource := parts[1]

	switch {
	case resource == "settings" && len(parts) == 2 && r.Method == http.MethodPost:
		h.handleUpsertSettings(w, r, userID)
		return
	case resource == "settings" && len(parts) == 2 && r.Method == http.MethodGet:
		h.handleGetSettings(w, r, userID)
		return
	case resource == "chats" && len(parts) == 4 && parts[3] == "calculate" && r.Method == http.MethodPost:
		h.handleCalculate(w, r, userID, parts[2])
		return
	case resource == "chats" && len(parts) == 4 && parts[3] == "calculations" && r.Method == http.MethodGet:
		h.handleListChatCalculations(w, r, userID, parts[2])
		return
	default:
		writeAPIError(w, http.StatusNotFound, "route not found")
		return
	}
}

func (h *APIHandler) handleUpsertSettings(w http.ResponseWriter, r *http.Request, userID string) {
	var req service.UserSettings
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.costing.SaveUserSettings(r.Context(), userID, req); err != nil {
		writeAPIDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) handleGetSettings(w http.ResponseWriter, r *http.Request, userID string) {
	settings, err := h.costing.GetUserSettings(r.Context(), userID)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, settings)
}

func (h *APIHandler) handleCalculate(w http.ResponseWriter, r *http.Request, userID, chatID string) {
	var req service.OrderInput
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.costing.CalculateInChat(r.Context(), userID, chatID, req)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, result)
}

func (h *APIHandler) handleListChatCalculations(w http.ResponseWriter, r *http.Request, userID, chatID string) {
	items, err := h.costing.ListChatCalculations(r.Context(), userID, chatID)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, map[string]any{"items": items})
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	if decoder.More() {
		return errors.New("invalid json: multiple objects in body")
	}
	return nil
}

func writeAPIJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeAPIJSON(w, status, map[string]string{"error": message})
}

func writeAPIDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidArgument):
		writeAPIError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, err.Error())
	}
}
