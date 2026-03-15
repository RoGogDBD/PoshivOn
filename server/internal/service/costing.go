package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
)

type BatchDiscount struct {
	MinQty  int     `json:"min_qty"`
	MaxQty  int     `json:"max_qty"`
	Percent float64 `json:"percent"`
}

type UserSettings struct {
	BasePrices       map[string]int64   `json:"base_prices"`
	SurchargePercent map[string]float64 `json:"surcharge_percent"`
	BatchDiscounts   []BatchDiscount    `json:"batch_discounts"`
}

type OrderInput struct {
	GarmentType   string   `json:"garment_type"`
	Complications []string `json:"complications"`
	Quantity      int      `json:"quantity"`
}

type AppliedSurcharge struct {
	Name      string `json:"name"`
	Percent   int64  `json:"percent"`
	Amount    int64  `json:"amount"`
	Repeats   int    `json:"repeats"`
	PerUnit   bool   `json:"per_unit"`
	CalcBasis string `json:"calc_basis"`
}

type CalculationResult struct {
	UserID             string             `json:"user_id"`
	ChatID             string             `json:"chat_id"`
	GarmentType        string             `json:"garment_type"`
	Quantity           int                `json:"quantity"`
	BasePricePerUnit   int64              `json:"base_price_per_unit"`
	SurchargePerUnit   int64              `json:"surcharge_per_unit"`
	PricePerUnit       int64              `json:"price_per_unit"`
	Subtotal           int64              `json:"subtotal"`
	DiscountPercent    int64              `json:"discount_percent"`
	DiscountAmount     int64              `json:"discount_amount"`
	Total              int64              `json:"total"`
	AppliedSurcharges  []AppliedSurcharge `json:"applied_surcharges"`
	AppliedDiscountMin int                `json:"applied_discount_min"`
	AppliedDiscountMax int                `json:"applied_discount_max"`
	CreatedAt          time.Time          `json:"created_at"`
}

type UserSettingsRepository interface {
	UpsertSettings(ctx context.Context, userID string, settings UserSettings) error
	GetSettings(ctx context.Context, userID string) (UserSettings, error)
}

type ChatCalculationRepository interface {
	AppendCalculation(ctx context.Context, result CalculationResult) error
	ListCalculations(ctx context.Context, userID, chatID string) ([]CalculationResult, error)
}

type CostingService struct {
	settingsRepo UserSettingsRepository
	chatRepo     ChatCalculationRepository
}

func NewCostingService(settingsRepo UserSettingsRepository, chatRepo ChatCalculationRepository) *CostingService {
	return &CostingService{
		settingsRepo: settingsRepo,
		chatRepo:     chatRepo,
	}
}

func (s *CostingService) SaveUserSettings(ctx context.Context, userID string, settings UserSettings) error {
	if userID == "" {
		return fmt.Errorf("user id is required: %w", ErrInvalidArgument)
	}
	if err := validateSettings(settings); err != nil {
		return err
	}
	if err := s.settingsRepo.UpsertSettings(ctx, userID, normalizeSettings(settings)); err != nil {
		return fmt.Errorf("save user settings: %w", err)
	}
	return nil
}

func (s *CostingService) GetUserSettings(ctx context.Context, userID string) (UserSettings, error) {
	if userID == "" {
		return UserSettings{}, fmt.Errorf("user id is required: %w", ErrInvalidArgument)
	}

	settings, err := s.settingsRepo.GetSettings(ctx, userID)
	if err != nil {
		return UserSettings{}, fmt.Errorf("get user settings: %w", err)
	}
	return settings, nil
}

func (s *CostingService) CalculateInChat(ctx context.Context, userID, chatID string, order OrderInput) (CalculationResult, error) {
	if userID == "" || chatID == "" {
		return CalculationResult{}, fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	if order.GarmentType == "" {
		return CalculationResult{}, fmt.Errorf("garment type is required: %w", ErrInvalidArgument)
	}
	if order.Quantity <= 0 {
		return CalculationResult{}, fmt.Errorf("quantity should be positive: %w", ErrInvalidArgument)
	}

	settings, err := s.settingsRepo.GetSettings(ctx, userID)
	if err != nil {
		return CalculationResult{}, fmt.Errorf("get settings for calculation: %w", err)
	}

	basePricePerUnit, ok := settings.BasePrices[order.GarmentType]
	if !ok {
		return CalculationResult{}, fmt.Errorf("unknown garment type %q: %w", order.GarmentType, ErrInvalidArgument)
	}

	repeatsByName := make(map[string]int, len(order.Complications))
	for _, name := range order.Complications {
		repeatsByName[name]++
	}

	appliedSurcharges := make([]AppliedSurcharge, 0, len(repeatsByName))
	totalSurchargePerUnit := int64(0)
	for name, repeats := range repeatsByName {
		percent, exists := settings.SurchargePercent[name]
		if !exists {
			return CalculationResult{}, fmt.Errorf("unknown complication %q: %w", name, ErrInvalidArgument)
		}

		surchargeAmount := int64(float64(basePricePerUnit) * (percent / 100.0) * float64(repeats))
		totalSurchargePerUnit += surchargeAmount

		appliedSurcharges = append(appliedSurcharges, AppliedSurcharge{
			Name:      name,
			Percent:   int64(percent),
			Amount:    surchargeAmount,
			Repeats:   repeats,
			PerUnit:   true,
			CalcBasis: "base_price",
		})
	}
	sort.Slice(appliedSurcharges, func(i, j int) bool {
		return appliedSurcharges[i].Name < appliedSurcharges[j].Name
	})

	pricePerUnit := basePricePerUnit + totalSurchargePerUnit
	subtotal := pricePerUnit * int64(order.Quantity)

	discount := pickDiscount(settings.BatchDiscounts, order.Quantity)
	discountAmount := int64(float64(subtotal) * (discount.Percent / 100.0))
	total := subtotal - discountAmount

	result := CalculationResult{
		UserID:             userID,
		ChatID:             chatID,
		GarmentType:        order.GarmentType,
		Quantity:           order.Quantity,
		BasePricePerUnit:   basePricePerUnit,
		SurchargePerUnit:   totalSurchargePerUnit,
		PricePerUnit:       pricePerUnit,
		Subtotal:           subtotal,
		DiscountPercent:    int64(discount.Percent),
		DiscountAmount:     discountAmount,
		Total:              total,
		AppliedSurcharges:  appliedSurcharges,
		AppliedDiscountMin: discount.MinQty,
		AppliedDiscountMax: discount.MaxQty,
		CreatedAt:          time.Now().UTC(),
	}

	if err := s.chatRepo.AppendCalculation(ctx, result); err != nil {
		return CalculationResult{}, fmt.Errorf("save calculation: %w", err)
	}

	return result, nil
}

func (s *CostingService) ListChatCalculations(ctx context.Context, userID, chatID string) ([]CalculationResult, error) {
	if userID == "" || chatID == "" {
		return nil, fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	items, err := s.chatRepo.ListCalculations(ctx, userID, chatID)
	if err != nil {
		return nil, fmt.Errorf("list chat calculations: %w", err)
	}
	return items, nil
}

func validateSettings(settings UserSettings) error {
	if len(settings.BasePrices) == 0 {
		return fmt.Errorf("base prices are required: %w", ErrInvalidArgument)
	}
	for product, price := range settings.BasePrices {
		if product == "" {
			return fmt.Errorf("base price product should not be empty: %w", ErrInvalidArgument)
		}
		if price <= 0 {
			return fmt.Errorf("base price should be positive for %q: %w", product, ErrInvalidArgument)
		}
	}

	for name, percent := range settings.SurchargePercent {
		if name == "" {
			return fmt.Errorf("surcharge name should not be empty: %w", ErrInvalidArgument)
		}
		if percent < 0 {
			return fmt.Errorf("surcharge percent should be non-negative for %q: %w", name, ErrInvalidArgument)
		}
	}

	for _, tier := range settings.BatchDiscounts {
		if tier.MinQty <= 0 {
			return fmt.Errorf("batch discount min_qty should be positive: %w", ErrInvalidArgument)
		}
		if tier.MaxQty < tier.MinQty {
			return fmt.Errorf("batch discount max_qty should be >= min_qty: %w", ErrInvalidArgument)
		}
		if tier.Percent < 0 || tier.Percent > 100 {
			return fmt.Errorf("batch discount percent should be in [0, 100]: %w", ErrInvalidArgument)
		}
	}
	return nil
}

func normalizeSettings(settings UserSettings) UserSettings {
	normalizedBasePrices := make(map[string]int64, len(settings.BasePrices))
	for k, v := range settings.BasePrices {
		normalizedBasePrices[k] = v
	}

	normalizedSurcharge := make(map[string]float64, len(settings.SurchargePercent))
	for k, v := range settings.SurchargePercent {
		normalizedSurcharge[k] = v
	}

	normalizedDiscounts := make([]BatchDiscount, len(settings.BatchDiscounts))
	copy(normalizedDiscounts, settings.BatchDiscounts)

	sort.Slice(normalizedDiscounts, func(i, j int) bool {
		if normalizedDiscounts[i].MinQty == normalizedDiscounts[j].MinQty {
			return normalizedDiscounts[i].MaxQty < normalizedDiscounts[j].MaxQty
		}
		return normalizedDiscounts[i].MinQty < normalizedDiscounts[j].MinQty
	})

	return UserSettings{
		BasePrices:       normalizedBasePrices,
		SurchargePercent: normalizedSurcharge,
		BatchDiscounts:   normalizedDiscounts,
	}
}

func pickDiscount(discounts []BatchDiscount, qty int) BatchDiscount {
	applied := BatchDiscount{MinQty: 1, MaxQty: 0, Percent: 0}
	for _, tier := range discounts {
		if qty < tier.MinQty || qty > tier.MaxQty {
			continue
		}
		if tier.Percent >= applied.Percent {
			applied = tier
		}
	}
	return applied
}
