package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

// handleUsers разбирает адрес маршрутов владельца и передаёт запрос дальше.
//
// Сегмента владельца в адресе больше нет (Decision 6, US-15): раньше первым сегментом шёл
// `{userID}`, и он же становился владельцем данных — то есть любой вызывающий читал и писал
// данные любого владельца, подставив чужой логин в адрес, а отзыв доступа (US-16) на этой
// поверхности не имел никакого эффекта. Теперь владелец берётся из личности в контексте, и
// других каналов не остаётся: `CreateChatInput` и `OrderInput` полей владельца не содержат,
// а `decodeJSON` включает `DisallowUnknownFields`, поэтому лишний `user_id` в теле — 400.
//
// Лишний сегмент на месте прежнего `{userID}` (`/api/v1/users/ivanov/chats`) ни в одну ветку
// switch не попадает и уходит в тот же 404 «route not found»: старая форма адреса не должна
// ни работать, ни быть источником владельца.
func (h *APIHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	parts := splitPath(path)

	if len(parts) == 0 {
		writeAPIError(w, http.StatusNotFound, "route not found")
		return
	}

	// Личность кладёт в контекст RequireAuth, а RequireAccess перед этим обработчиком уже
	// подтвердил её право (routes.go), поэтому пустой результат недостижим при правильно
	// собранной цепочке. Проверка всё равно есть и закрытая — по той же причине, что и в
	// resolveAccessState: ошибка сборки маршрутов не должна превращаться в работу от имени
	// никого. Без неё пустой логин ушёл бы в CostingService и вернулся 400 «invalid
	// argument», то есть выглядел бы ошибкой запроса, а не ошибкой конфигурации сервера.
	identity, ok := requireIdentity(w, r)
	if !ok {
		return
	}
	userID := identity.Login

	resource := parts[0]

	switch {
	case resource == "settings" && len(parts) == 1 && r.Method == http.MethodPost:
		h.handleUpsertSettings(w, r, userID)
		return
	case resource == "settings" && len(parts) == 1 && r.Method == http.MethodGet:
		h.handleGetSettings(w, r, userID)
		return
	case resource == "chats" && len(parts) == 1 && r.Method == http.MethodPost:
		h.handleCreateChat(w, r, userID)
		return
	case resource == "chats" && len(parts) == 1 && r.Method == http.MethodGet:
		h.handleListChats(w, r, userID)
		return
	case resource == "chats" && len(parts) == 2 && r.Method == http.MethodDelete:
		h.handleDeleteChat(w, r, userID, parts[1])
		return
	case resource == "chats" && len(parts) == 3 && parts[2] == "restore" && r.Method == http.MethodPost:
		h.handleRestoreChat(w, r, userID, parts[1])
		return
	case resource == "chats" && len(parts) == 3 && parts[2] == "calculate" && r.Method == http.MethodPost:
		h.handleCalculate(w, r, userID, parts[1])
		return
	case resource == "chats" && len(parts) == 3 && parts[2] == "calculations" && r.Method == http.MethodGet:
		h.handleListChatCalculations(w, r, userID, parts[1])
		return
	case resource == "market-feedback" && len(parts) == 1 && r.Method == http.MethodPost:
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
		writeAPIDecodeError(w, err)
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
		writeAPIDecodeError(w, err)
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
		writeAPIDecodeError(w, err)
		return
	}

	result, err := h.costing.CalculateInChat(r.Context(), userID, chatID, req)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	// До перехода дефолтного тарифа на quick фидбек запрашивался только для masterpiece —
	// тогда это совпадало почти со всеми пользователями. Теперь quick — это дефолт, и то же
	// условие тихо отключило бы AI-фидбек для большинства. buildMarketFeedbackInputFromCalculation
	// уже устойчив к пустым MaterialType/Urgency/MarketSegment у quick-расчётов (см. emptyOrUnknown
	// в deepseek.go), так что ограничение по режиму было для этого не нужно.
	if h.deepseek != nil {
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
		// Тот же слаг, что и у остальных 503 в classifyDomainError: свободный текст здесь
		// раскрывал состав интеграций сервера, а не только «сервис недоступен».
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable")
		return
	}

	var req service.MarketFeedbackInput
	if err := decodeJSON(r, &req); err != nil {
		writeAPIDecodeError(w, err)
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

// writeAPIDecodeError отвечает на неудачный decodeJSON фиксированным слагом. err от
// encoding/json называет неизвестное поле и Go-имена структуры/типа целиком (`invalid json:
// json: cannot unmarshal string into Go struct field PricingRules.pricing_rules.labor_minute_rate
// of type int64`) — то же по духу раскрытие внутреннего устройства, которое Decision 17 убрал
// из writeAPIDomainError, поэтому и путь наружу тот же: фиксированный текст клиенту, исходная
// ошибка — в лог. Общий helper, а не пять копий одного `log.Printf`+`writeAPIError` рядом с
// каждым decodeJSON, — чтобы шестой вызывающий не мог снова разойтись с остальными.
func writeAPIDecodeError(w http.ResponseWriter, err error) {
	log.Printf("api error: status=%d code=invalid_request err=%v", http.StatusBadRequest, err)
	writeAPIError(w, http.StatusBadRequest, "invalid_request")
}

// writeAPIDomainError отображает доменную ошибку на HTTP-ответ. Наружу уходит только
// категория фиксированным текстом, исходная ошибка остаётся в логе сервера (Decision 17):
// до этого каждая ветка отдавала err.Error(), а ошибки репозитория обёрнуты SQL-контекстом
// и попадали клиенту целиком. Правило одно на все ветки, включая существовавшие раньше, —
// иначе в одной функции жили бы две конвенции.
func writeAPIDomainError(w http.ResponseWriter, err error) {
	status, code := classifyDomainError(err)
	log.Printf("api error: status=%d code=%s err=%v", status, code, err)
	writeAPIError(w, status, code)
}

func classifyDomainError(err error) (int, string) {
	// Через эту функцию проходит каждый ответ об ошибке API, а ниже по веткам вызывается
	// err.Error() — на nil это паника. Сегодня все вызывающие проверяют ошибку до вызова,
	// но цена страховки здесь одна строка, а цена её отсутствия — 500 с паникой в проде.
	if err == nil {
		return http.StatusInternalServerError, "internal_error"
	}

	switch {
	case errors.Is(err, service.ErrInvalidArgument):
		return http.StatusBadRequest, "invalid_request"
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, service.ErrConflict):
		return http.StatusConflict, "conflict"
	// Три ветки ниже сопоставляются по тексту ошибки DeepSeek-клиента: своих сентинелов
	// он не выставляет. Сопоставление хрупкое, но теперь хотя бы не утекает в ответ.
	case strings.Contains(strings.ToLower(err.Error()), "rate_limit_exceeded"):
		return http.StatusTooManyRequests, "rate_limited"
	case strings.Contains(strings.ToLower(err.Error()), "service_unavailable"):
		return http.StatusServiceUnavailable, "service_unavailable"
	case strings.Contains(strings.ToLower(err.Error()), "timeout"):
		return http.StatusGatewayTimeout, "timeout"
	default:
		return http.StatusInternalServerError, "internal_error"
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
