package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	settingsByID map[string]service.UserSettings
	chatsByID    map[string]map[string]service.Chat
	chatHistory  map[string]map[string][]service.CalculationResult
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		settingsByID: make(map[string]service.UserSettings),
		chatsByID:    make(map[string]map[string]service.Chat),
		chatHistory:  make(map[string]map[string][]service.CalculationResult),
	}
}

var _ service.UserSettingsRepository = (*MemoryRepository)(nil)
var _ service.ChatRepository = (*MemoryRepository)(nil)
var _ service.ChatCalculationRepository = (*MemoryRepository)(nil)

func (r *MemoryRepository) UpsertSettings(_ context.Context, userID string, settings service.UserSettings) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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

func (r *MemoryRepository) AttachCalculationAIFeedback(
	_ context.Context,
	userID, chatID string,
	createdAt time.Time,
	feedback service.MarketFeedbackResult,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	byChat, ok := r.chatHistory[userID]
	if !ok {
		return fmt.Errorf("calculation for chat %q not found: %w", chatID, service.ErrNotFound)
	}

	items := byChat[chatID]
	for index := range items {
		if items[index].CreatedAt.Equal(createdAt) {
			feedbackCopy := feedback
			items[index].AIFeedback = &feedbackCopy
			byChat[chatID] = items
			return nil
		}
	}

	return fmt.Errorf("calculation for chat %q not found: %w", chatID, service.ErrNotFound)
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
