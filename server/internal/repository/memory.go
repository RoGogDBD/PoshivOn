package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

type MemoryRepository struct {
	mu           sync.RWMutex
	settingsByID map[string]service.UserSettings
	chatHistory  map[string]map[string][]service.CalculationResult
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		settingsByID: make(map[string]service.UserSettings),
		chatHistory:  make(map[string]map[string][]service.CalculationResult),
	}
}

var _ service.UserSettingsRepository = (*MemoryRepository)(nil)
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

func (r *MemoryRepository) AppendCalculation(_ context.Context, result service.CalculationResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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

func copySettings(src service.UserSettings) service.UserSettings {
	basePrices := make(map[string]int64, len(src.BasePrices))
	for k, v := range src.BasePrices {
		basePrices[k] = v
	}

	surchargePercent := make(map[string]float64, len(src.SurchargePercent))
	for k, v := range src.SurchargePercent {
		surchargePercent[k] = v
	}

	batchDiscounts := make([]service.BatchDiscount, len(src.BatchDiscounts))
	copy(batchDiscounts, src.BatchDiscounts)

	return service.UserSettings{
		BasePrices:       basePrices,
		SurchargePercent: surchargePercent,
		BatchDiscounts:   batchDiscounts,
	}
}

func copyCalculation(src service.CalculationResult) service.CalculationResult {
	item := src
	item.AppliedSurcharges = make([]service.AppliedSurcharge, len(src.AppliedSurcharges))
	copy(item.AppliedSurcharges, src.AppliedSurcharges)
	return item
}
