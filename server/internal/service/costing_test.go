package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type settingsRepoStub struct {
	settings map[string]UserSettings
}

func (s *settingsRepoStub) UpsertSettings(_ context.Context, userID string, settings UserSettings) error {
	if s.settings == nil {
		s.settings = make(map[string]UserSettings)
	}
	s.settings[userID] = settings
	return nil
}

func (s *settingsRepoStub) GetSettings(_ context.Context, userID string) (UserSettings, error) {
	settings, ok := s.settings[userID]
	if !ok {
		return UserSettings{}, ErrNotFound
	}
	return settings, nil
}

type chatRepoStub struct {
	chats []Chat
	items []CalculationResult
}

func (c *chatRepoStub) CreateChat(_ context.Context, chat Chat) (Chat, error) {
	c.chats = append(c.chats, chat)
	return chat, nil
}

func (c *chatRepoStub) ListChats(_ context.Context, _ string) ([]Chat, error) {
	result := make([]Chat, 0, len(c.chats))
	for _, chat := range c.chats {
		if chat.DeletedAt == nil {
			result = append(result, chat)
		}
	}
	return result, nil
}

func (c *chatRepoStub) DeleteChat(_ context.Context, userID, chatID, deletedBy string, hard bool) error {
	for i := range c.chats {
		if c.chats[i].UserID != userID || c.chats[i].ID != chatID {
			continue
		}
		if hard {
			c.chats = append(c.chats[:i], c.chats[i+1:]...)
			return nil
		}
		deletedAt := c.chats[i].UpdatedAt
		c.chats[i].DeletedAt = &deletedAt
		c.chats[i].DeletedBy = deletedBy
		return nil
	}
	return ErrNotFound
}

func (c *chatRepoStub) RestoreChat(_ context.Context, userID, chatID string) error {
	for i := range c.chats {
		if c.chats[i].UserID == userID && c.chats[i].ID == chatID {
			c.chats[i].DeletedAt = nil
			c.chats[i].DeletedBy = ""
			return nil
		}
	}
	return ErrNotFound
}

func (c *chatRepoStub) AppendCalculation(_ context.Context, result CalculationResult) error {
	c.items = append(c.items, result)
	return nil
}

func (c *chatRepoStub) ListCalculations(_ context.Context, _, _ string) ([]CalculationResult, error) {
	return c.items, nil
}

func (c *chatRepoStub) AttachCalculationAIFeedback(
	_ context.Context,
	_ string,
	_ string,
	createdAt time.Time,
	feedback MarketFeedbackResult,
) error {
	for index := range c.items {
		if c.items[index].CreatedAt.Equal(createdAt) {
			feedbackCopy := feedback
			c.items[index].AIFeedback = &feedbackCopy
			return nil
		}
	}
	return ErrNotFound
}

func TestCostingService_CalculateInChat_UsesExpandedPricingModel(t *testing.T) {
	t.Parallel()

	settingsRepo := &settingsRepoStub{}
	chatRepo := &chatRepoStub{}
	svc := NewCostingService(settingsRepo, chatRepo, chatRepo)

	settings := UserSettings{
		PricingRules: PricingRules{
			LaborMinuteRate:           20,
			PayrollTaxesPercent:       20,
			OverheadPercent:           10,
			LogisticsCostPerUnit:      100,
			MarginPercent:             25,
			MinMarginPercent:          15,
			IncludedFittings:          1,
			ExtraFittingMinutes:       20,
			CustomFigureCoefficient:   1.10,
			ChildCoefficient:          0.75,
			DefaultRiskPercent:        3,
			DefaultConsumablesPerUnit: 50,
		},
		Garments: map[string]GarmentConfig{
			"Пиджак": {BaseMinutes: 200, ComplexityCoeff: 1.60},
		},
		Operations: map[string]OperationConfig{
			"Карман":        {AdditionalMinutes: 25, AdditionalMaterialPerUnit: 100},
			"Трудная ткань": {AdditionalMinutes: 30, AdditionalMaterialPerUnit: 0},
		},
		Materials: map[string]MaterialConfig{
			"Костюмная ткань": {
				Coefficient:            1.05,
				FabricCostPerUnit:      1000,
				LiningCostPerUnit:      200,
				InterfacingCostPerUnit: 80,
				ThreadCostPerUnit:      30,
				HardwareCostPerUnit:    40,
				DecorCostPerUnit:       0,
				PackagingCostPerUnit:   20,
				ConsumablesCostPerUnit: 30,
				RiskPercent:            4,
			},
		},
		BatchDiscounts: []BatchDiscount{{MinQty: 1, MaxQty: 10, Percent: 0}, {MinQty: 11, MaxQty: 50, Percent: 5}},
		Urgency:        map[string]UrgencyRule{"Стандарт": {Percent: 0}, "Срочно 3-5 дней": {Percent: 15}},
		MarketBands:    map[string]MarketBand{"Средний": {MinPricePerUnit: 9000, AveragePricePerUnit: 16000, MaxPricePerUnit: 20000}},
	}

	if err := svc.SaveUserSettings(context.Background(), "u-1", settings); err != nil {
		t.Fatalf("SaveUserSettings() error = %v", err)
	}

	result, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:     "Пиджак",
		MaterialType:    "Костюмная ткань",
		Quantity:        15,
		OperationCounts: map[string]int{"Карман": 2, "Трудная ткань": 1},
		Urgency:         "Стандарт",
		Fittings:        2,
		MarketSegment:   "Средний",
	})
	if err != nil {
		t.Fatalf("CalculateInChat() error = %v", err)
	}

	if result.Total != 279955 {
		t.Fatalf("total = %d, want %d", result.Total, 279955)
	}
	if result.AdjustedMinutesPerUnit != 505 {
		t.Fatalf("adjusted minutes = %d, want 505", result.AdjustedMinutesPerUnit)
	}
	if result.DiscountPercent != 5 {
		t.Fatalf("discount percent = %d, want 5", result.DiscountPercent)
	}
	if result.MarketStatus != "in_market" {
		t.Fatalf("market status = %q, want in_market", result.MarketStatus)
	}
	if got := len(chatRepo.items); got != 1 {
		t.Fatalf("saved calculations = %d, want 1", got)
	}
}

func TestCostingService_CalculateInChat_RespectsMinimumMarginFloor(t *testing.T) {
	t.Parallel()

	settings := DefaultUserSettings()
	settings.PricingRules.MarginPercent = 5
	settings.PricingRules.MinMarginPercent = 20
	settings.BatchDiscounts = []BatchDiscount{{MinQty: 1, MaxQty: 100, Percent: 50}}

	repo := &settingsRepoStub{settings: map[string]UserSettings{"u-1": settings}}
	chatRepo := &chatRepoStub{}
	svc := NewCostingService(repo, chatRepo, chatRepo)

	result, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:   "Юбка",
		MaterialType:  "Хлопок",
		Quantity:      20,
		Urgency:       "Стандарт",
		MarketSegment: "Массмаркет",
	})
	if err != nil {
		t.Fatalf("CalculateInChat() error = %v", err)
	}

	minimumTotal := result.MinAllowedPricePerUnit * int64(result.Quantity)
	if result.Total != minimumTotal {
		t.Fatalf("total = %d, want floor %d", result.Total, minimumTotal)
	}
}

func TestCostingService_CreateDeleteRestoreChat(t *testing.T) {
	t.Parallel()

	chatRepo := &chatRepoStub{}
	svc := NewCostingService(&settingsRepoStub{}, chatRepo, chatRepo)

	chat, err := svc.CreateChat(context.Background(), "u-1", CreateChatInput{Title: "Партия пиджаков"})
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}

	if err := svc.DeleteChat(context.Background(), "u-1", chat.ID, false); err != nil {
		t.Fatalf("DeleteChat() error = %v", err)
	}

	listed, err := svc.ListChats(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed chats = %d, want 0 after soft delete", len(listed))
	}

	if err := svc.RestoreChat(context.Background(), "u-1", chat.ID); err != nil {
		t.Fatalf("RestoreChat() error = %v", err)
	}

	listed, err = svc.ListChats(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed chats = %d, want 1 after restore", len(listed))
	}
}

func TestCostingService_CalculateInChat_UnknownOperation(t *testing.T) {
	t.Parallel()

	repo := &settingsRepoStub{settings: map[string]UserSettings{"u-1": DefaultUserSettings()}}
	svc := NewCostingService(repo, &chatRepoStub{}, &chatRepoStub{})

	_, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:     "Пиджак",
		MaterialType:    "Костюмная ткань",
		Quantity:        1,
		OperationCounts: map[string]int{"Неизвестно": 1},
		Urgency:         "Стандарт",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
}

func TestCostingService_CalculateInChat_QuickMode(t *testing.T) {
	t.Parallel()

	settings := DefaultUserSettings()
	settings.PricingRules.CalculatorMode = calculatorModeQuick
	settings.Garments = map[string]GarmentConfig{
		"Худи": {BaseMinutes: 1, ComplexityCoeff: 1, QuickPrice: 3000},
	}
	settings.Operations = map[string]OperationConfig{
		"Капюшон": {AdditionalMinutes: 0, AdditionalMaterialPerUnit: 0, QuickPercent: 10},
		"Карманы": {AdditionalMinutes: 0, AdditionalMaterialPerUnit: 0, QuickPercent: 5},
	}
	settings.BatchDiscounts = []BatchDiscount{
		{MinQty: 1, MaxQty: 9, Percent: 0},
		{MinQty: 10, MaxQty: 100, Percent: 10},
	}

	repo := &settingsRepoStub{settings: map[string]UserSettings{"u-1": settings}}
	chatRepo := &chatRepoStub{}
	svc := NewCostingService(repo, chatRepo, chatRepo)

	result, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:     "Худи",
		Quantity:        10,
		OperationCounts: map[string]int{"Капюшон": 1, "Карманы": 2},
	})
	if err != nil {
		t.Fatalf("CalculateInChat() error = %v", err)
	}

	if result.CalculationMode != calculatorModeQuick {
		t.Fatalf("calculation mode = %q, want %q", result.CalculationMode, calculatorModeQuick)
	}
	if result.PriceBeforeDiscount != 3600 {
		t.Fatalf("price before discount = %d, want 3600", result.PriceBeforeDiscount)
	}
	if result.Total != 32400 {
		t.Fatalf("total = %d, want 32400", result.Total)
	}
	if len(result.AppliedOperations) != 2 {
		t.Fatalf("applied operations = %d, want 2", len(result.AppliedOperations))
	}
}
