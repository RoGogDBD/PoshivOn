package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// memoryUser — строка users в памяти. Отдельный тип, а не service.UserRecord: статус
// заявки живёт в requestsByLogin и подставляется при чтении, ровно как LEFT JOIN на
// стороне PostgresRepository, иначе те же данные хранились бы в двух местах.
type memoryUser struct {
	Login       string
	Email       string
	DisplayName string
	Role        service.Role
	HasAccess   bool
	CreatedAt   time.Time
}

type MemoryRepository struct {
	mu              sync.RWMutex
	settingsByID    map[string]service.UserSettings
	chatsByID       map[string]map[string]service.Chat
	chatHistory     map[string]map[string][]service.CalculationResult
	usersByLogin    map[string]memoryUser
	requestsByLogin map[string]service.AccessRequest
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		settingsByID:    make(map[string]service.UserSettings),
		chatsByID:       make(map[string]map[string]service.Chat),
		chatHistory:     make(map[string]map[string][]service.CalculationResult),
		usersByLogin:    make(map[string]memoryUser),
		requestsByLogin: make(map[string]service.AccessRequest),
	}
}

var _ service.UserSettingsRepository = (*MemoryRepository)(nil)
var _ service.ChatRepository = (*MemoryRepository)(nil)
var _ service.ChatCalculationRepository = (*MemoryRepository)(nil)
var _ service.UserRepository = (*MemoryRepository)(nil)
var _ service.AccessRequestRepository = (*MemoryRepository)(nil)

func (r *MemoryRepository) UpsertSettings(_ context.Context, userID string, settings service.UserSettings) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.touchUserRow(userID)
	r.settingsByID[userID] = copySettings(settings)
	return nil
}

func (r *MemoryRepository) GetSettings(_ context.Context, userID string) (service.UserSettings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	settings, ok := r.settingsByID[userID]
	if !ok {
		return service.UserSettings{}, fmt.Errorf("settings for user %q not found: %w", userID, service.ErrNotFound)
	}

	return copySettings(settings), nil
}

func (r *MemoryRepository) CreateChat(_ context.Context, chat service.Chat) (service.Chat, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.touchUserRow(chat.UserID)
	byUser, ok := r.chatsByID[chat.UserID]
	if !ok {
		byUser = make(map[string]service.Chat)
		r.chatsByID[chat.UserID] = byUser
	}

	byUser[chat.ID] = copyChat(chat)
	return copyChat(chat), nil
}

func (r *MemoryRepository) ListChats(_ context.Context, userID string) ([]service.Chat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byUser, ok := r.chatsByID[userID]
	if !ok || len(byUser) == 0 {
		return []service.Chat{}, nil
	}

	items := make([]service.Chat, 0, len(byUser))
	for _, chat := range byUser {
		if chat.DeletedAt != nil {
			continue
		}
		items = append(items, copyChat(chat))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	return items, nil
}

func (r *MemoryRepository) DeleteChat(_ context.Context, userID, chatID, deletedBy string, hard bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	byUser, ok := r.chatsByID[userID]
	if !ok {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}
	chat, ok := byUser[chatID]
	if !ok {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}

	if hard {
		delete(byUser, chatID)
		if historyByUser, ok := r.chatHistory[userID]; ok {
			delete(historyByUser, chatID)
		}
		return nil
	}

	now := time.Now().UTC()
	chat.DeletedAt = &now
	chat.DeletedBy = deletedBy
	byUser[chatID] = chat
	return nil
}

func (r *MemoryRepository) RestoreChat(_ context.Context, userID, chatID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	byUser, ok := r.chatsByID[userID]
	if !ok {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}
	chat, ok := byUser[chatID]
	if !ok {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}
	chat.DeletedAt = nil
	chat.DeletedBy = ""
	byUser[chatID] = chat
	return nil
}

func (r *MemoryRepository) AppendCalculation(_ context.Context, result service.CalculationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.touchUserRow(result.UserID)
	byUser, ok := r.chatsByID[result.UserID]
	if !ok {
		byUser = make(map[string]service.Chat)
		r.chatsByID[result.UserID] = byUser
	}
	chat, ok := byUser[result.ChatID]
	if !ok {
		chat = service.Chat{
			UserID:    result.UserID,
			ID:        result.ChatID,
			Title:     "Новый чат",
			CreatedAt: result.CreatedAt,
			UpdatedAt: result.CreatedAt,
		}
	}
	chat.UpdatedAt = result.CreatedAt
	chat.CalculationsCount++
	byUser[result.ChatID] = copyChat(chat)

	byChat, ok := r.chatHistory[result.UserID]
	if !ok {
		byChat = make(map[string][]service.CalculationResult)
		r.chatHistory[result.UserID] = byChat
	}

	chatItems := byChat[result.ChatID]
	chatItems = append(chatItems, copyCalculation(result))
	byChat[result.ChatID] = chatItems
	return nil
}

func (r *MemoryRepository) ListCalculations(_ context.Context, userID, chatID string) ([]service.CalculationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byChat, ok := r.chatHistory[userID]
	if !ok {
		return []service.CalculationResult{}, nil
	}

	items := byChat[chatID]
	if len(items) == 0 {
		return []service.CalculationResult{}, nil
	}

	result := make([]service.CalculationResult, len(items))
	for i := range items {
		result[i] = copyCalculation(items[i])
	}
	return result, nil
}

// --- UserRepository / AccessRequestRepository ---------------------------------------
//
// Обе реализации проверяются одним и тем же контрактным набором тестов
// (access_repo_test.go), поэтому наблюдаемое поведение здесь повторяет PostgresRepository:
// те же ошибки, те же поля, которые меняются и не меняются. Отличается только механика —
// вместо SQL обычные условия под уже существующим r.mu.

// EnsureUser создаёт строку с ролью user и без доступа, а у существующей обновляет только
// email и display_name. Роль и флаг доступа не трогаются никогда (Decision 11): метод
// вызывается на каждом входе, и перезапись этих полей разжаловала бы администратора.
func (r *MemoryRepository) EnsureUser(_ context.Context, login, email, displayName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.usersByLogin[login]
	if !ok {
		r.usersByLogin[login] = memoryUser{
			Login:       login,
			Email:       email,
			DisplayName: displayName,
			Role:        service.RoleUser,
			HasAccess:   false,
			CreatedAt:   time.Now().UTC(),
		}
		return nil
	}

	record.Email = email
	record.DisplayName = displayName
	r.usersByLogin[login] = record
	return nil
}

func (r *MemoryRepository) GetUser(_ context.Context, login string) (service.UserRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.usersByLogin[login]
	if !ok {
		return service.UserRecord{}, fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	return r.toUserRecord(record), nil
}

func (r *MemoryRepository) ListUsers(_ context.Context) ([]service.UserRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]service.UserRecord, 0, len(r.usersByLogin))
	for _, record := range r.usersByLogin {
		items = append(items, r.toUserRecord(record))
	}
	// Порядок тот же, что у PostgresRepository.ListUsers: сначала недавно появившиеся,
	// логин как тай-брейк, чтобы выдача не зависела от обхода мапы.
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Login < items[j].Login
	})
	return items, nil
}

func (r *MemoryRepository) SetAccess(_ context.Context, login string, granted bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.usersByLogin[login]
	if !ok {
		return fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	// Присваиваем granted, а не «поднимаем флаг»: отзыв доступа обязан возвращать true в
	// false, иначе снятая галочка администратора не имеет эффекта.
	record.HasAccess = granted
	r.usersByLogin[login] = record
	return nil
}

// CreateRequest повторяет семантику upsert'а из Decision 5: заявка на рассмотрении
// повторной подачей не сдвигается (ErrConflict), а после решения — возвращается в pending
// с новой датой обращения, сохраняя decided_by и decided_at прошлого решения.
func (r *MemoryRepository) CreateRequest(_ context.Context, login string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.requestsByLogin[login]
	if ok && existing.Status == requestStatusPending {
		return fmt.Errorf("access request for user %q is already pending: %w", login, service.ErrConflict)
	}

	existing.UserID = login
	existing.Status = requestStatusPending
	existing.CreatedAt = time.Now().UTC()
	r.requestsByLogin[login] = existing
	return nil
}

func (r *MemoryRepository) GetRequest(_ context.Context, login string) (service.AccessRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	request, ok := r.requestsByLogin[login]
	if !ok {
		return service.AccessRequest{}, fmt.Errorf("access request for user %q not found: %w", login, service.ErrNotFound)
	}
	return copyAccessRequest(request), nil
}

// DecideRequest фиксирует решение по уже существующей заявке. Отсутствие строки — no-op,
// а не ошибка: администратор вправе выдать доступ тому, кто заявку не подавал, и заводить
// её задним числом не нужно (Decision 5).
func (r *MemoryRepository) DecideRequest(_ context.Context, login, status, decidedBy string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	request, ok := r.requestsByLogin[login]
	if !ok {
		return nil
	}

	decidedAt := time.Now().UTC()
	request.Status = status
	request.DecidedAt = &decidedAt
	request.DecidedBy = decidedBy
	r.requestsByLogin[login] = request
	return nil
}

// touchUserRow повторяет upsertUser из PostgresRepository: существующие пути записи
// (настройки, чат, расчёт) заводят строку пользователя, если её ещё нет, и не трогают уже
// заведённую. Без этого ListUsers в памяти терял бы тех, кого в MariaDB создаёт та же
// вставка. Вызывается только под уже взятым r.mu.Lock().
func (r *MemoryRepository) touchUserRow(login string) {
	if _, ok := r.usersByLogin[login]; ok {
		return
	}
	r.usersByLogin[login] = memoryUser{
		Login:     login,
		Role:      service.RoleUser,
		HasAccess: false,
		CreatedAt: time.Now().UTC(),
	}
}

// toUserRecord собирает доменную запись из строки пользователя и её заявки.
// Вызывается только под уже взятым r.mu (чтение).
func (r *MemoryRepository) toUserRecord(record memoryUser) service.UserRecord {
	item := service.UserRecord{
		Login:       record.Login,
		DisplayName: record.DisplayName,
		Email:       record.Email,
		Role:        record.Role,
		HasAccess:   record.HasAccess,
		CreatedAt:   record.CreatedAt,
	}
	if request, ok := r.requestsByLogin[record.Login]; ok {
		requestedAt := request.CreatedAt
		item.RequestStatus = request.Status
		item.RequestedAt = &requestedAt
	}
	return item
}

func copyAccessRequest(src service.AccessRequest) service.AccessRequest {
	item := src
	if src.DecidedAt != nil {
		value := *src.DecidedAt
		item.DecidedAt = &value
	}
	return item
}

func copySettings(src service.UserSettings) service.UserSettings {
	item := src
	item.Garments = make(map[string]service.GarmentConfig, len(src.Garments))
	for key, value := range src.Garments {
		item.Garments[key] = value
	}
	item.Operations = make(map[string]service.OperationConfig, len(src.Operations))
	for key, value := range src.Operations {
		item.Operations[key] = value
	}
	item.Materials = make(map[string]service.MaterialConfig, len(src.Materials))
	for key, value := range src.Materials {
		item.Materials[key] = value
	}
	item.Urgency = make(map[string]service.UrgencyRule, len(src.Urgency))
	for key, value := range src.Urgency {
		item.Urgency[key] = value
	}
	item.MarketBands = make(map[string]service.MarketBand, len(src.MarketBands))
	for key, value := range src.MarketBands {
		item.MarketBands[key] = value
	}
	item.BatchDiscounts = append([]service.BatchDiscount(nil), src.BatchDiscounts...)
	return item
}

func copyCalculation(src service.CalculationResult) service.CalculationResult {
	item := src
	item.AppliedOperations = append([]service.AppliedOperation(nil), src.AppliedOperations...)
	item.MaterialLines = append([]service.MaterialLine(nil), src.MaterialLines...)
	if src.AIFeedback != nil {
		feedbackCopy := *src.AIFeedback
		feedbackCopy.KeyDrivers = append([]string(nil), src.AIFeedback.KeyDrivers...)
		feedbackCopy.Risks = append([]string(nil), src.AIFeedback.Risks...)
		feedbackCopy.Recommendations = append([]string(nil), src.AIFeedback.Recommendations...)
		if src.AIFeedback.SelectedMarketBand != nil {
			bandCopy := *src.AIFeedback.SelectedMarketBand
			feedbackCopy.SelectedMarketBand = &bandCopy
		}
		item.AIFeedback = &feedbackCopy
	}
	return item
}

func copyChat(src service.Chat) service.Chat {
	item := src
	if src.DeletedAt != nil {
		value := *src.DeletedAt
		item.DeletedAt = &value
	}
	return item
}
