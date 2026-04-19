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
	costing  *service.CostingService
	deepseek *service.DeepSeekClient
}

func NewAPIHandler(costing *service.CostingService, deepseek *service.DeepSeekClient) *APIHandler {
	return &APIHandler{costing: costing, deepseek: deepseek}
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
	case resource == "chats" && len(parts) == 2 && r.Method == http.MethodPost:
		h.handleCreateChat(w, r, userID)
		return
	case resource == "chats" && len(parts) == 2 && r.Method == http.MethodGet:
		h.handleListChats(w, r, userID)
		return
	case resource == "chats" && len(parts) == 3 && r.Method == http.MethodDelete:
		h.handleDeleteChat(w, r, userID, parts[2])
		return
	case resource == "chats" && len(parts) == 4 && parts[3] == "restore" && r.Method == http.MethodPost:
		h.handleRestoreChat(w, r, userID, parts[2])
		return
	case resource == "chats" && len(parts) == 4 && parts[3] == "calculate" && r.Method == http.MethodPost:
		h.handleCalculate(w, r, userID, parts[2])
		return
	case resource == "chats" && len(parts) == 4 && parts[3] == "calculations" && r.Method == http.MethodGet:
		h.handleListChatCalculations(w, r, userID, parts[2])
		return
	case resource == "market-feedback" && len(parts) == 2 && r.Method == http.MethodPost:
		h.handleMarketFeedback(w, r, userID)
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

func (h *APIHandler) handleCreateChat(w http.ResponseWriter, r *http.Request, userID string) {
	var req service.CreateChatInput
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	chat, err := h.costing.CreateChat(r.Context(), userID, req)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusCreated, chat)
}

func (h *APIHandler) handleListChats(w http.ResponseWriter, r *http.Request, userID string) {
	chats, err := h.costing.ListChats(r.Context(), userID)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, map[string]any{"items": chats})
}

func (h *APIHandler) handleDeleteChat(w http.ResponseWriter, r *http.Request, userID, chatID string) {
	hard := strings.EqualFold(r.URL.Query().Get("hard"), "true")
	if err := h.costing.DeleteChat(r.Context(), userID, chatID, hard); err != nil {
		writeAPIDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) handleRestoreChat(w http.ResponseWriter, r *http.Request, userID, chatID string) {
	if err := h.costing.RestoreChat(r.Context(), userID, chatID); err != nil {
		writeAPIDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

	if h.deepseek != nil && result.CalculationMode == "masterpiece" {
		settings, settingsErr := h.costing.GetUserSettings(r.Context(), userID)
		if settingsErr != nil {
			if errors.Is(settingsErr, service.ErrNotFound) {
				settings = service.DefaultUserSettings()
			} else {
				writeAPIDomainError(w, settingsErr)
				return
			}
		}

		feedback, feedbackErr := h.deepseek.AnalyzeMarketFeedback(
			r.Context(),
			buildMarketFeedbackInputFromCalculation(result),
			settings,
		)
		if feedbackErr == nil {
			result.AIFeedback = &feedback
		}
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

func (h *APIHandler) handleMarketFeedback(w http.ResponseWriter, r *http.Request, userID string) {
	if h.deepseek == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "deepseek integration is not configured")
		return
	}

	var req service.MarketFeedbackInput
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	settings, err := h.costing.GetUserSettings(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			settings = service.DefaultUserSettings()
		} else {
			writeAPIDomainError(w, err)
			return
		}
	}

	result, err := h.deepseek.AnalyzeMarketFeedback(r.Context(), req, settings)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, result)
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
	case strings.Contains(strings.ToLower(err.Error()), "rate_limit_exceeded"):
		writeAPIError(w, http.StatusTooManyRequests, err.Error())
	case strings.Contains(strings.ToLower(err.Error()), "service_unavailable"):
		writeAPIError(w, http.StatusServiceUnavailable, err.Error())
	case strings.Contains(strings.ToLower(err.Error()), "timeout"):
		writeAPIError(w, http.StatusGatewayTimeout, err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, err.Error())
	}
}

func buildMarketFeedbackInputFromCalculation(result service.CalculationResult) service.MarketFeedbackInput {
	operationCounts := make(map[string]int, len(result.AppliedOperations))
	for _, operation := range result.AppliedOperations {
		if operation.Count > 0 {
			operationCounts[operation.Name] = operation.Count
		}
	}

	return service.MarketFeedbackInput{
		GarmentType:     result.GarmentType,
		MaterialType:    result.MaterialType,
		MarketSegment:   result.MarketSegment,
		Urgency:         result.Urgency,
		Quantity:        result.Quantity,
		Fittings:        result.Fittings,
		IsCustomFigure:  result.IsCustomFigure,
		IsChild:         result.IsChild,
		Comment:         result.Comment,
		OperationCounts: operationCounts,
		Calculation: &service.MarketFeedbackCalculationInput{
			CalculationMode:        result.CalculationMode,
			BasePricePerUnitRUB:    result.MinAllowedPricePerUnit,
			CostPricePerUnitRUB:    result.CostPricePerUnit,
			PriceBeforeDiscountRUB: result.PriceBeforeDiscount,
			MinAllowedPriceRUB:     result.MinAllowedPricePerUnit,
			FinalPricePerUnitRUB:   result.PricePerUnit,
			FinalTotalRUB:          result.Total,
			DiscountPercent:        result.DiscountPercent,
			DiscountAmountRUB:      result.DiscountAmount,
			MarketStatus:           result.MarketStatus,
		},
	}
}
