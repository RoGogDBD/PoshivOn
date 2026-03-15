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
	result := make([]Chat, len(c.chats))
	copy(result, c.chats)
	return result, nil
}

func (c *chatRepoStub) AppendCalculation(_ context.Context, result CalculationResult) error {
	c.items = append(c.items, result)
	return nil
}

func (c *chatRepoStub) ListCalculations(_ context.Context, _, _ string) ([]CalculationResult, error) {
	return c.items, nil
}

func TestCostingService_CalculateInChat_Example114000(t *testing.T) {
	t.Parallel()

	settingsRepo := &settingsRepoStub{}
	chatRepo := &chatRepoStub{}
	svc := NewCostingService(settingsRepo, chatRepo, chatRepo)

	settings := UserSettings{
		BasePrices: map[string]int64{
			"Пиджак":  5000,
			"Юбка":    2000,
			"Рубашка": 3000,
		},
		SurchargePercent: map[string]float64{
			"Карман":        20,
			"Трудная ткань": 20,
			"Молния":        20,
		},
		BatchDiscounts: []BatchDiscount{
			{MinQty: 1, MaxQty: 10, Percent: 0},
			{MinQty: 11, MaxQty: 50, Percent: 5},
			{MinQty: 51, MaxQty: 100, Percent: 10},
		},
	}

	if err := svc.SaveUserSettings(context.Background(), "u-1", settings); err != nil {
		t.Fatalf("SaveUserSettings() error = %v", err)
	}

	result, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:   "Пиджак",
		Complications: []string{"Карман", "Карман", "Трудная ткань"},
		Quantity:      15,
	})
	if err != nil {
		t.Fatalf("CalculateInChat() error = %v", err)
	}

	if result.Total != 114000 {
		t.Fatalf("total = %d, want %d", result.Total, 114000)
	}

	if got := len(chatRepo.items); got != 1 {
		t.Fatalf("saved calculations = %d, want 1", got)
	}
}

func TestCostingService_CalculateInChat_UnknownComplication(t *testing.T) {
	t.Parallel()

	settingsRepo := &settingsRepoStub{
		settings: map[string]UserSettings{
			"u-1": {
				BasePrices: map[string]int64{"Пиджак": 5000},
				SurchargePercent: map[string]float64{
					"Карман": 20,
				},
			},
		},
	}
	svc := NewCostingService(settingsRepo, &chatRepoStub{}, &chatRepoStub{})

	_, err := svc.CalculateInChat(context.Background(), "u-1", "chat-1", OrderInput{
		GarmentType:   "Пиджак",
		Complications: []string{"Неизвестно"},
		Quantity:      1,
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
}

func TestCostingService_CreateChat(t *testing.T) {
	t.Parallel()

	chatRepo := &chatRepoStub{}
	svc := NewCostingService(&settingsRepoStub{}, chatRepo, chatRepo)

	chat, err := svc.CreateChat(context.Background(), "u-1", CreateChatInput{Title: "Партия пиджаков"})
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}
	if chat.ID == "" {
		t.Fatal("CreateChat() returned empty id")
	}
	if chat.Title != "Партия пиджаков" {
		t.Fatalf("chat title = %q, want %q", chat.Title, "Партия пиджаков")
	}
	if len(chatRepo.chats) != 1 {
		t.Fatalf("stored chats = %d, want 1", len(chatRepo.chats))
	}
	if chat.CreatedAt.IsZero() || chat.UpdatedAt.IsZero() {
		t.Fatalf("chat timestamps must be set: %+v", chat)
	}
	if chat.UpdatedAt.Before(chat.CreatedAt.Add(-1 * time.Second)) {
		t.Fatalf("updated_at = %v should not be before created_at = %v", chat.UpdatedAt, chat.CreatedAt)
	}
}
