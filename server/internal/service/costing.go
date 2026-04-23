package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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

type PricingRules struct {
	CalculatorMode            string  `json:"calculator_mode"`
	LaborMinuteRate           int64   `json:"labor_minute_rate"`
	PayrollTaxesPercent       float64 `json:"payroll_taxes_percent"`
	OverheadPercent           float64 `json:"overhead_percent"`
	LogisticsCostPerUnit      int64   `json:"logistics_cost_per_unit"`
	MarginPercent             float64 `json:"margin_percent"`
	MinMarginPercent          float64 `json:"min_margin_percent"`
	IncludedFittings          int     `json:"included_fittings"`
	ExtraFittingMinutes       int     `json:"extra_fitting_minutes"`
	CustomFigureCoefficient   float64 `json:"custom_figure_coefficient"`
	ChildCoefficient          float64 `json:"child_coefficient"`
	DefaultRiskPercent        float64 `json:"default_risk_percent"`
	DefaultConsumablesPerUnit int64   `json:"default_consumables_per_unit"`
}

type GarmentConfig struct {
	BaseMinutes     int     `json:"base_minutes"`
	ComplexityCoeff float64 `json:"complexity_coeff"`
	QuickPrice      int64   `json:"quick_price"`
}

type OperationConfig struct {
	AdditionalMinutes         int     `json:"additional_minutes"`
	AdditionalMaterialPerUnit int64   `json:"additional_material_per_unit"`
	QuickPercent              float64 `json:"quick_percent"`
}

type MaterialConfig struct {
	Coefficient            float64 `json:"coefficient"`
	FabricCostPerUnit      int64   `json:"fabric_cost_per_unit"`
	LiningCostPerUnit      int64   `json:"lining_cost_per_unit"`
	InterfacingCostPerUnit int64   `json:"interfacing_cost_per_unit"`
	ThreadCostPerUnit      int64   `json:"thread_cost_per_unit"`
	HardwareCostPerUnit    int64   `json:"hardware_cost_per_unit"`
	DecorCostPerUnit       int64   `json:"decor_cost_per_unit"`
	PackagingCostPerUnit   int64   `json:"packaging_cost_per_unit"`
	ConsumablesCostPerUnit int64   `json:"consumables_cost_per_unit"`
	RiskPercent            float64 `json:"risk_percent"`
}

type UrgencyRule struct {
	Percent float64 `json:"percent"`
}

type MarketBand struct {
	MinPricePerUnit     int64 `json:"min_price_per_unit"`
	AveragePricePerUnit int64 `json:"average_price_per_unit"`
	MaxPricePerUnit     int64 `json:"max_price_per_unit"`
}

type UserSettings struct {
	PricingRules   PricingRules               `json:"pricing_rules"`
	Garments       map[string]GarmentConfig   `json:"garments"`
	Operations     map[string]OperationConfig `json:"operations"`
	Materials      map[string]MaterialConfig  `json:"materials"`
	BatchDiscounts []BatchDiscount            `json:"batch_discounts"`
	Urgency        map[string]UrgencyRule     `json:"urgency"`
	MarketBands    map[string]MarketBand      `json:"market_bands"`
}

type OrderInput struct {
	GarmentType     string         `json:"garment_type"`
	MaterialType    string         `json:"material_type"`
	Quantity        int            `json:"quantity"`
	OperationCounts map[string]int `json:"operation_counts,omitempty"`
	Complications   []string       `json:"complications,omitempty"`
	Urgency         string         `json:"urgency"`
	Fittings        int            `json:"fittings"`
	MarketSegment   string         `json:"market_segment"`
	IsCustomFigure  bool           `json:"is_custom_figure"`
	IsChild         bool           `json:"is_child"`
	Comment         string         `json:"comment"`
}

type AppliedOperation struct {
	Name                   string `json:"name"`
	Count                  int    `json:"count"`
	AdditionalMinutes      int    `json:"additional_minutes"`
	AdditionalMaterialCost int64  `json:"additional_material_cost"`
}

type MaterialLine struct {
	Name        string `json:"name"`
	CostPerUnit int64  `json:"cost_per_unit"`
}

type CalculationResult struct {
	UserID                  string                `json:"user_id"`
	ChatID                  string                `json:"chat_id"`
	CalculationMode         string                `json:"calculation_mode"`
	GarmentType             string                `json:"garment_type"`
	MaterialType            string                `json:"material_type"`
	Urgency                 string                `json:"urgency"`
	MarketSegment           string                `json:"market_segment"`
	Quantity                int                   `json:"quantity"`
	Fittings                int                   `json:"fittings"`
	IsCustomFigure          bool                  `json:"is_custom_figure"`
	IsChild                 bool                  `json:"is_child"`
	Comment                 string                `json:"comment"`
	BaseMinutesPerUnit      int                   `json:"base_minutes_per_unit"`
	OperationMinutesPerUnit int                   `json:"operation_minutes_per_unit"`
	FittingMinutesPerUnit   int                   `json:"fitting_minutes_per_unit"`
	AdjustedMinutesPerUnit  int                   `json:"adjusted_minutes_per_unit"`
	LaborCostPerUnit        int64                 `json:"labor_cost_per_unit"`
	PayrollCostPerUnit      int64                 `json:"payroll_cost_per_unit"`
	MaterialsCostPerUnit    int64                 `json:"materials_cost_per_unit"`
	ConsumablesCostPerUnit  int64                 `json:"consumables_cost_per_unit"`
	OverheadCostPerUnit     int64                 `json:"overhead_cost_per_unit"`
	LogisticsCostPerUnit    int64                 `json:"logistics_cost_per_unit"`
	RiskReservePerUnit      int64                 `json:"risk_reserve_per_unit"`
	CostPricePerUnit        int64                 `json:"cost_price_per_unit"`
	MarginPerUnit           int64                 `json:"margin_per_unit"`
	PriceBeforeDiscount     int64                 `json:"price_before_discount_per_unit"`
	MinAllowedPricePerUnit  int64                 `json:"min_allowed_price_per_unit"`
	PricePerUnit            int64                 `json:"price_per_unit"`
	Subtotal                int64                 `json:"subtotal"`
	DiscountPercent         int64                 `json:"discount_percent"`
	DiscountAmount          int64                 `json:"discount_amount"`
	Total                   int64                 `json:"total"`
	MarketStatus            string                `json:"market_status"`
	AppliedOperations       []AppliedOperation    `json:"applied_operations"`
	MaterialLines           []MaterialLine        `json:"material_lines"`
	AIFeedback              *MarketFeedbackResult `json:"ai_feedback,omitempty"`
	CreatedAt               time.Time             `json:"created_at"`
}

type Chat struct {
	UserID            string     `json:"user_id"`
	ID                string     `json:"id"`
	Title             string     `json:"title"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CalculationsCount int        `json:"calculations_count"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
	DeletedBy         string     `json:"deleted_by,omitempty"`
}

type CreateChatInput struct {
	Title string `json:"title"`
}

type UserSettingsRepository interface {
	UpsertSettings(ctx context.Context, userID string, settings UserSettings) error
	GetSettings(ctx context.Context, userID string) (UserSettings, error)
}

type ChatRepository interface {
	CreateChat(ctx context.Context, chat Chat) (Chat, error)
	ListChats(ctx context.Context, userID string) ([]Chat, error)
	DeleteChat(ctx context.Context, userID, chatID, deletedBy string, hard bool) error
	RestoreChat(ctx context.Context, userID, chatID string) error
}

type ChatCalculationRepository interface {
	AppendCalculation(ctx context.Context, result CalculationResult) error
	ListCalculations(ctx context.Context, userID, chatID string) ([]CalculationResult, error)
}

type CostingService struct {
	settingsRepo UserSettingsRepository
	chatRepo     ChatRepository
	calcRepo     ChatCalculationRepository
}

const (
	calculatorModeMasterpiece = "masterpiece"
	calculatorModeQuick       = "quick"
)

func NewCostingService(
	settingsRepo UserSettingsRepository,
	chatRepo ChatRepository,
	calculationRepo ChatCalculationRepository,
) *CostingService {
	return &CostingService{
		settingsRepo: settingsRepo,
		chatRepo:     chatRepo,
		calcRepo:     calculationRepo,
	}
}

func DefaultUserSettings() UserSettings {
	return UserSettings{
		PricingRules: PricingRules{
			CalculatorMode:            calculatorModeMasterpiece,
			LaborMinuteRate:           18,
			PayrollTaxesPercent:       30,
			OverheadPercent:           18,
			LogisticsCostPerUnit:      120,
			MarginPercent:             25,
			MinMarginPercent:          12,
			IncludedFittings:          1,
			ExtraFittingMinutes:       20,
			CustomFigureCoefficient:   1.10,
			ChildCoefficient:          0.75,
			DefaultRiskPercent:        3,
			DefaultConsumablesPerUnit: 90,
		},
		Garments: map[string]GarmentConfig{
			"Пиджак":  {BaseMinutes: 260, ComplexityCoeff: 1.60, QuickPrice: 7000},
			"Юбка":    {BaseMinutes: 90, ComplexityCoeff: 1.00, QuickPrice: 3200},
			"Рубашка": {BaseMinutes: 140, ComplexityCoeff: 1.15, QuickPrice: 4200},
			"Платье":  {BaseMinutes: 180, ComplexityCoeff: 1.30, QuickPrice: 5600},
		},
		Operations: map[string]OperationConfig{
			"Карман накладной":       {AdditionalMinutes: 15, AdditionalMaterialPerUnit: 80, QuickPercent: 8},
			"Карман прорезной":       {AdditionalMinutes: 25, AdditionalMaterialPerUnit: 120, QuickPercent: 12},
			"Подклад":                {AdditionalMinutes: 35, AdditionalMaterialPerUnit: 350, QuickPercent: 15},
			"Потайная молния":        {AdditionalMinutes: 12, AdditionalMaterialPerUnit: 120, QuickPercent: 6},
			"Воротник":               {AdditionalMinutes: 20, AdditionalMaterialPerUnit: 90, QuickPercent: 10},
			"Манжеты":                {AdditionalMinutes: 15, AdditionalMaterialPerUnit: 70, QuickPercent: 8},
			"Шлица":                  {AdditionalMinutes: 18, AdditionalMaterialPerUnit: 50, QuickPercent: 7},
			"Декоративная отстрочка": {AdditionalMinutes: 18, AdditionalMaterialPerUnit: 0, QuickPercent: 5},
		},
		Materials: map[string]MaterialConfig{
			"Хлопок": {
				Coefficient:            1.00,
				FabricCostPerUnit:      650,
				LiningCostPerUnit:      0,
				InterfacingCostPerUnit: 60,
				ThreadCostPerUnit:      35,
				HardwareCostPerUnit:    50,
				DecorCostPerUnit:       0,
				PackagingCostPerUnit:   20,
				ConsumablesCostPerUnit: 30,
				RiskPercent:            2,
			},
			"Костюмная ткань": {
				Coefficient:            1.05,
				FabricCostPerUnit:      1200,
				LiningCostPerUnit:      320,
				InterfacingCostPerUnit: 120,
				ThreadCostPerUnit:      45,
				HardwareCostPerUnit:    90,
				DecorCostPerUnit:       0,
				PackagingCostPerUnit:   25,
				ConsumablesCostPerUnit: 40,
				RiskPercent:            3,
			},
			"Лён": {
				Coefficient:            1.10,
				FabricCostPerUnit:      980,
				LiningCostPerUnit:      0,
				InterfacingCostPerUnit: 70,
				ThreadCostPerUnit:      40,
				HardwareCostPerUnit:    60,
				DecorCostPerUnit:       0,
				PackagingCostPerUnit:   20,
				ConsumablesCostPerUnit: 35,
				RiskPercent:            4,
			},
			"Трикотаж": {
				Coefficient:            1.15,
				FabricCostPerUnit:      870,
				LiningCostPerUnit:      0,
				InterfacingCostPerUnit: 40,
				ThreadCostPerUnit:      45,
				HardwareCostPerUnit:    30,
				DecorCostPerUnit:       0,
				PackagingCostPerUnit:   20,
				ConsumablesCostPerUnit: 45,
				RiskPercent:            4,
			},
			"Шёлк": {
				Coefficient:            1.30,
				FabricCostPerUnit:      1750,
				LiningCostPerUnit:      450,
				InterfacingCostPerUnit: 90,
				ThreadCostPerUnit:      55,
				HardwareCostPerUnit:    70,
				DecorCostPerUnit:       30,
				PackagingCostPerUnit:   30,
				ConsumablesCostPerUnit: 50,
				RiskPercent:            7,
			},
		},
		BatchDiscounts: []BatchDiscount{
			{MinQty: 1, MaxQty: 10, Percent: 0},
			{MinQty: 11, MaxQty: 50, Percent: 5},
			{MinQty: 51, MaxQty: 100, Percent: 10},
			{MinQty: 101, MaxQty: 1000000, Percent: 12},
		},
		Urgency: map[string]UrgencyRule{
			"Стандарт":        {Percent: 0},
			"Срочно 3-5 дней": {Percent: 15},
			"Срочно 1-2 дня":  {Percent: 30},
			"В день заказа":   {Percent: 50},
		},
		MarketBands: map[string]MarketBand{
			"Массмаркет": {MinPricePerUnit: 2500, AveragePricePerUnit: 4500, MaxPricePerUnit: 7000},
			"Средний":    {MinPricePerUnit: 5000, AveragePricePerUnit: 9000, MaxPricePerUnit: 15000},
			"Премиум":    {MinPricePerUnit: 9000, AveragePricePerUnit: 16000, MaxPricePerUnit: 26000},
		},
	}
}

func (s *CostingService) CreateChat(ctx context.Context, userID string, input CreateChatInput) (Chat, error) {
	if userID == "" {
		return Chat{}, fmt.Errorf("user id is required: %w", ErrInvalidArgument)
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "Новый чат"
	}

	now := time.Now().UTC()
	chat := Chat{
		UserID:            userID,
		ID:                newChatID(),
		Title:             title,
		CreatedAt:         now,
		UpdatedAt:         now,
		CalculationsCount: 0,
	}

	createdChat, err := s.chatRepo.CreateChat(ctx, chat)
	if err != nil {
		return Chat{}, fmt.Errorf("create chat: %w", err)
	}

	return createdChat, nil
}

func (s *CostingService) ListChats(ctx context.Context, userID string) ([]Chat, error) {
	if userID == "" {
		return nil, fmt.Errorf("user id is required: %w", ErrInvalidArgument)
	}

	chats, err := s.chatRepo.ListChats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}

	return chats, nil
}

func (s *CostingService) DeleteChat(ctx context.Context, userID, chatID string, hard bool) error {
	if userID == "" || chatID == "" {
		return fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	if err := s.chatRepo.DeleteChat(ctx, userID, chatID, userID, hard); err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}
	return nil
}

func (s *CostingService) RestoreChat(ctx context.Context, userID, chatID string) error {
	if userID == "" || chatID == "" {
		return fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	if err := s.chatRepo.RestoreChat(ctx, userID, chatID); err != nil {
		return fmt.Errorf("restore chat: %w", err)
	}
	return nil
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
	return normalizeSettings(settings), nil
}

func (s *CostingService) CalculateInChat(ctx context.Context, userID, chatID string, order OrderInput) (CalculationResult, error) {
	if userID == "" || chatID == "" {
		return CalculationResult{}, fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	if strings.TrimSpace(order.GarmentType) == "" {
		return CalculationResult{}, fmt.Errorf("garment type is required: %w", ErrInvalidArgument)
	}
	if order.Quantity <= 0 {
		return CalculationResult{}, fmt.Errorf("quantity should be positive: %w", ErrInvalidArgument)
	}

	settings, err := s.settingsRepo.GetSettings(ctx, userID)
	if err != nil {
		return CalculationResult{}, fmt.Errorf("get settings for calculation: %w", err)
	}
	settings = normalizeSettings(settings)
	calculationMode := normalizeCalculatorMode(settings.PricingRules.CalculatorMode)

	garment, ok := settings.Garments[order.GarmentType]
	if !ok {
		return CalculationResult{}, fmt.Errorf("unknown garment type %q: %w", order.GarmentType, ErrInvalidArgument)
	}
	if calculationMode == calculatorModeQuick {
		return s.calculateQuickInChat(ctx, userID, chatID, order, settings, garment)
	}

	materialName := strings.TrimSpace(order.MaterialType)
	if materialName == "" {
		materialName = pickFirstMaterial(settings.Materials)
	}
	material, ok := settings.Materials[materialName]
	if !ok {
		return CalculationResult{}, fmt.Errorf("unknown material type %q: %w", materialName, ErrInvalidArgument)
	}

	operationCounts := normalizeOperationCounts(order)
	appliedOperations := make([]AppliedOperation, 0, len(operationCounts))
	operationMinutesPerUnit := 0
	extraMaterialsPerUnit := int64(0)
	for name, count := range operationCounts {
		cfg, exists := settings.Operations[name]
		if !exists {
			return CalculationResult{}, fmt.Errorf("unknown operation %q: %w", name, ErrInvalidArgument)
		}
		operationMinutesPerUnit += cfg.AdditionalMinutes * count
		extraMaterialsPerUnit += cfg.AdditionalMaterialPerUnit * int64(count)
		appliedOperations = append(appliedOperations, AppliedOperation{
			Name:                   name,
			Count:                  count,
			AdditionalMinutes:      cfg.AdditionalMinutes * count,
			AdditionalMaterialCost: cfg.AdditionalMaterialPerUnit * int64(count),
		})
	}
	sort.Slice(appliedOperations, func(i, j int) bool {
		return appliedOperations[i].Name < appliedOperations[j].Name
	})

	extraFittings := order.Fittings - settings.PricingRules.IncludedFittings
	if extraFittings < 0 {
		extraFittings = 0
	}
	fittingMinutesPerUnit := extraFittings * settings.PricingRules.ExtraFittingMinutes
	rawMinutes := garment.BaseMinutes + operationMinutesPerUnit + fittingMinutesPerUnit

	multiplier := garment.ComplexityCoeff * material.Coefficient
	if order.IsCustomFigure {
		multiplier *= settings.PricingRules.CustomFigureCoefficient
	}
	if order.IsChild {
		multiplier *= settings.PricingRules.ChildCoefficient
	}
	adjustedMinutes := int(math.Ceil(float64(rawMinutes) * multiplier))
	if adjustedMinutes < 1 {
		adjustedMinutes = 1
	}

	laborCostPerUnit := int64(adjustedMinutes) * settings.PricingRules.LaborMinuteRate
	payrollCostPerUnit := percentOf(laborCostPerUnit, settings.PricingRules.PayrollTaxesPercent)

	materialLines := buildMaterialLines(material, extraMaterialsPerUnit, order.IsChild, settings.PricingRules.ChildCoefficient)
	materialsCostPerUnit, consumablesCostPerUnit := sumMaterialLines(materialLines)
	consumablesCostPerUnit += moneyMul(settings.PricingRules.DefaultConsumablesPerUnit, childOrOne(order.IsChild, settings.PricingRules.ChildCoefficient))
	logisticsCostPerUnit := moneyMul(settings.PricingRules.LogisticsCostPerUnit, childOrOne(order.IsChild, settings.PricingRules.ChildCoefficient))

	overheadBase := laborCostPerUnit + payrollCostPerUnit + materialsCostPerUnit + consumablesCostPerUnit
	overheadCostPerUnit := percentOf(overheadBase, settings.PricingRules.OverheadPercent)
	riskPercent := settings.PricingRules.DefaultRiskPercent
	if material.RiskPercent > riskPercent {
		riskPercent = material.RiskPercent
	}
	riskReservePerUnit := percentOf(laborCostPerUnit+materialsCostPerUnit+consumablesCostPerUnit, riskPercent)

	costPricePerUnit := laborCostPerUnit + payrollCostPerUnit + materialsCostPerUnit + consumablesCostPerUnit + overheadCostPerUnit + logisticsCostPerUnit + riskReservePerUnit
	marginPerUnit := percentOf(costPricePerUnit, settings.PricingRules.MarginPercent)
	minAllowedPricePerUnit := costPricePerUnit + percentOf(costPricePerUnit, settings.PricingRules.MinMarginPercent)
	priceBeforeDiscount := costPricePerUnit + marginPerUnit
	if priceBeforeDiscount < minAllowedPricePerUnit {
		priceBeforeDiscount = minAllowedPricePerUnit
	}

	urgencyName := strings.TrimSpace(order.Urgency)
	if urgencyName == "" {
		urgencyName = "Стандарт"
	}
	urgencyRule, ok := settings.Urgency[urgencyName]
	if !ok {
		return CalculationResult{}, fmt.Errorf("unknown urgency %q: %w", urgencyName, ErrInvalidArgument)
	}
	priceBeforeDiscount += percentOf(priceBeforeDiscount, urgencyRule.Percent)

	subtotal := priceBeforeDiscount * int64(order.Quantity)
	discount := pickDiscount(settings.BatchDiscounts, order.Quantity)
	discountAmount := percentOf(subtotal, discount.Percent)
	total := subtotal - discountAmount
	minimumTotal := minAllowedPricePerUnit * int64(order.Quantity)
	if total < minimumTotal {
		total = minimumTotal
		discountAmount = subtotal - total
		if discountAmount < 0 {
			discountAmount = 0
		}
	}
	pricePerUnit := int64(math.Round(float64(total) / float64(order.Quantity)))
	marketStatus := detectMarketStatus(settings, order.MarketSegment, pricePerUnit)

	result := CalculationResult{
		UserID:                  userID,
		ChatID:                  chatID,
		CalculationMode:         calculationMode,
		GarmentType:             order.GarmentType,
		MaterialType:            materialName,
		Urgency:                 urgencyName,
		MarketSegment:           order.MarketSegment,
		Quantity:                order.Quantity,
		Fittings:                order.Fittings,
		IsCustomFigure:          order.IsCustomFigure,
		IsChild:                 order.IsChild,
		Comment:                 strings.TrimSpace(order.Comment),
		BaseMinutesPerUnit:      garment.BaseMinutes,
		OperationMinutesPerUnit: operationMinutesPerUnit,
		FittingMinutesPerUnit:   fittingMinutesPerUnit,
		AdjustedMinutesPerUnit:  adjustedMinutes,
		LaborCostPerUnit:        laborCostPerUnit,
		PayrollCostPerUnit:      payrollCostPerUnit,
		MaterialsCostPerUnit:    materialsCostPerUnit,
		ConsumablesCostPerUnit:  consumablesCostPerUnit,
		OverheadCostPerUnit:     overheadCostPerUnit,
		LogisticsCostPerUnit:    logisticsCostPerUnit,
		RiskReservePerUnit:      riskReservePerUnit,
		CostPricePerUnit:        costPricePerUnit,
		MarginPerUnit:           marginPerUnit,
		PriceBeforeDiscount:     priceBeforeDiscount,
		MinAllowedPricePerUnit:  minAllowedPricePerUnit,
		PricePerUnit:            pricePerUnit,
		Subtotal:                subtotal,
		DiscountPercent:         int64(discount.Percent),
		DiscountAmount:          discountAmount,
		Total:                   total,
		MarketStatus:            marketStatus,
		AppliedOperations:       appliedOperations,
		MaterialLines:           materialLines,
		CreatedAt:               time.Now().UTC(),
	}

	if err := s.calcRepo.AppendCalculation(ctx, result); err != nil {
		return CalculationResult{}, fmt.Errorf("save calculation: %w", err)
	}

	return result, nil
}

func (s *CostingService) ListChatCalculations(ctx context.Context, userID, chatID string) ([]CalculationResult, error) {
	if userID == "" || chatID == "" {
		return nil, fmt.Errorf("user id and chat id are required: %w", ErrInvalidArgument)
	}
	items, err := s.calcRepo.ListCalculations(ctx, userID, chatID)
	if err != nil {
		return nil, fmt.Errorf("list chat calculations: %w", err)
	}
	return items, nil
}

func validateSettings(settings UserSettings) error {
	if len(settings.Garments) == 0 {
		return fmt.Errorf("garments are required: %w", ErrInvalidArgument)
	}
	if len(settings.Materials) == 0 {
		return fmt.Errorf("materials are required: %w", ErrInvalidArgument)
	}
	if len(settings.Urgency) == 0 {
		return fmt.Errorf("urgency rules are required: %w", ErrInvalidArgument)
	}
	if settings.PricingRules.LaborMinuteRate <= 0 {
		return fmt.Errorf("labor minute rate should be positive: %w", ErrInvalidArgument)
	}
	if mode := normalizeCalculatorMode(settings.PricingRules.CalculatorMode); mode == "" {
		return fmt.Errorf("calculator mode should be valid: %w", ErrInvalidArgument)
	}
	if settings.PricingRules.MarginPercent < 0 || settings.PricingRules.MinMarginPercent < 0 {
		return fmt.Errorf("margin should be non-negative: %w", ErrInvalidArgument)
	}
	for name, garment := range settings.Garments {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("garment name should not be empty: %w", ErrInvalidArgument)
		}
		if garment.BaseMinutes <= 0 {
			return fmt.Errorf("garment base minutes should be positive for %q: %w", name, ErrInvalidArgument)
		}
		if garment.ComplexityCoeff <= 0 {
			return fmt.Errorf("garment coefficient should be positive for %q: %w", name, ErrInvalidArgument)
		}
		if garment.QuickPrice < 0 {
			return fmt.Errorf("garment quick price should be non-negative for %q: %w", name, ErrInvalidArgument)
		}
	}
	for name, material := range settings.Materials {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("material name should not be empty: %w", ErrInvalidArgument)
		}
		if material.Coefficient <= 0 {
			return fmt.Errorf("material coefficient should be positive for %q: %w", name, ErrInvalidArgument)
		}
	}
	for name, op := range settings.Operations {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("operation name should not be empty: %w", ErrInvalidArgument)
		}
		if op.AdditionalMinutes < 0 || op.AdditionalMaterialPerUnit < 0 {
			return fmt.Errorf("operation values should be non-negative for %q: %w", name, ErrInvalidArgument)
		}
		if op.QuickPercent < 0 {
			return fmt.Errorf("operation quick percent should be non-negative for %q: %w", name, ErrInvalidArgument)
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
	defaults := DefaultUserSettings()

	normalized := UserSettings{
		PricingRules:   defaults.PricingRules,
		Garments:       copyGarments(defaults.Garments),
		Operations:     copyOperations(defaults.Operations),
		Materials:      copyMaterials(defaults.Materials),
		BatchDiscounts: append([]BatchDiscount(nil), defaults.BatchDiscounts...),
		Urgency:        copyUrgency(defaults.Urgency),
		MarketBands:    copyMarketBands(defaults.MarketBands),
	}

	if settings.PricingRules.LaborMinuteRate > 0 {
		normalized.PricingRules = settings.PricingRules
	}
	for key, value := range settings.Garments {
		normalized.Garments[key] = value
	}
	for key, value := range settings.Operations {
		normalized.Operations[key] = value
	}
	for key, value := range settings.Materials {
		normalized.Materials[key] = value
	}
	if len(settings.BatchDiscounts) > 0 {
		normalized.BatchDiscounts = append([]BatchDiscount(nil), settings.BatchDiscounts...)
	}
	for key, value := range settings.Urgency {
		normalized.Urgency[key] = value
	}
	for key, value := range settings.MarketBands {
		normalized.MarketBands[key] = value
	}

	sort.Slice(normalized.BatchDiscounts, func(i, j int) bool {
		if normalized.BatchDiscounts[i].MinQty == normalized.BatchDiscounts[j].MinQty {
			return normalized.BatchDiscounts[i].MaxQty < normalized.BatchDiscounts[j].MaxQty
		}
		return normalized.BatchDiscounts[i].MinQty < normalized.BatchDiscounts[j].MinQty
	})

	if normalized.PricingRules.CustomFigureCoefficient <= 0 {
		normalized.PricingRules.CustomFigureCoefficient = defaults.PricingRules.CustomFigureCoefficient
	}
	if normalized.PricingRules.ChildCoefficient <= 0 {
		normalized.PricingRules.ChildCoefficient = defaults.PricingRules.ChildCoefficient
	}
	normalized.PricingRules.CalculatorMode = normalizeCalculatorMode(normalized.PricingRules.CalculatorMode)

	return normalized
}

func (s *CostingService) calculateQuickInChat(
	ctx context.Context,
	userID string,
	chatID string,
	order OrderInput,
	settings UserSettings,
	garment GarmentConfig,
) (CalculationResult, error) {
	if garment.QuickPrice <= 0 {
		return CalculationResult{}, fmt.Errorf("quick price should be positive for %q: %w", order.GarmentType, ErrInvalidArgument)
	}

	operationCounts := normalizeOperationCounts(order)
	appliedOperations := make([]AppliedOperation, 0, len(operationCounts))
	totalQuickPercent := 0.0
	for name, count := range operationCounts {
		cfg, exists := settings.Operations[name]
		if !exists {
			return CalculationResult{}, fmt.Errorf("unknown operation %q: %w", name, ErrInvalidArgument)
		}
		appliedPercent := cfg.QuickPercent * float64(count)
		totalQuickPercent += appliedPercent
		appliedOperations = append(appliedOperations, AppliedOperation{
			Name:                   name,
			Count:                  count,
			AdditionalMaterialCost: percentOf(garment.QuickPrice, appliedPercent),
		})
	}
	sort.Slice(appliedOperations, func(i, j int) bool {
		return appliedOperations[i].Name < appliedOperations[j].Name
	})

	priceBeforeDiscount := garment.QuickPrice + percentOf(garment.QuickPrice, totalQuickPercent)
	subtotal := priceBeforeDiscount * int64(order.Quantity)
	discount := pickDiscount(settings.BatchDiscounts, order.Quantity)
	discountAmount := percentOf(subtotal, discount.Percent)
	total := subtotal - discountAmount
	pricePerUnit := int64(math.Round(float64(total) / float64(order.Quantity)))

	result := CalculationResult{
		UserID:                 userID,
		ChatID:                 chatID,
		CalculationMode:        calculatorModeQuick,
		GarmentType:            order.GarmentType,
		Quantity:               order.Quantity,
		IsCustomFigure:         order.IsCustomFigure,
		IsChild:                order.IsChild,
		Comment:                strings.TrimSpace(order.Comment),
		PriceBeforeDiscount:    priceBeforeDiscount,
		MinAllowedPricePerUnit: garment.QuickPrice,
		PricePerUnit:           pricePerUnit,
		Subtotal:               subtotal,
		DiscountPercent:        int64(discount.Percent),
		DiscountAmount:         discountAmount,
		Total:                  total,
		MarketStatus:           "unknown",
		AppliedOperations:      appliedOperations,
		CreatedAt:              time.Now().UTC(),
	}

	if err := s.calcRepo.AppendCalculation(ctx, result); err != nil {
		return CalculationResult{}, fmt.Errorf("save calculation: %w", err)
	}

	return result, nil
}

func normalizeCalculatorMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", calculatorModeMasterpiece:
		return calculatorModeMasterpiece
	case calculatorModeQuick:
		return calculatorModeQuick
	default:
		return ""
	}
}

func newChatID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("chat-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(raw[:])
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

func normalizeOperationCounts(order OrderInput) map[string]int {
	result := make(map[string]int, len(order.OperationCounts)+len(order.Complications))
	for name, count := range order.OperationCounts {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || count <= 0 {
			continue
		}
		result[trimmed] += count
	}
	for _, name := range order.Complications {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		result[trimmed]++
	}
	return result
}

func buildMaterialLines(material MaterialConfig, extraMaterialsPerUnit int64, isChild bool, childCoefficient float64) []MaterialLine {
	multiplier := childOrOne(isChild, childCoefficient)
	lines := []MaterialLine{
		{Name: "Ткань", CostPerUnit: moneyMul(material.FabricCostPerUnit, multiplier)},
		{Name: "Подклад", CostPerUnit: moneyMul(material.LiningCostPerUnit, multiplier)},
		{Name: "Дублерин/флизелин", CostPerUnit: moneyMul(material.InterfacingCostPerUnit, multiplier)},
		{Name: "Фурнитура", CostPerUnit: moneyMul(material.HardwareCostPerUnit, multiplier)},
		{Name: "Декор", CostPerUnit: moneyMul(material.DecorCostPerUnit, multiplier)},
		{Name: "Нитки", CostPerUnit: moneyMul(material.ThreadCostPerUnit, multiplier)},
		{Name: "Упаковка", CostPerUnit: moneyMul(material.PackagingCostPerUnit, multiplier)},
		{Name: "Расходники", CostPerUnit: moneyMul(material.ConsumablesCostPerUnit, multiplier)},
	}
	if extraMaterialsPerUnit > 0 {
		lines = append(lines, MaterialLine{Name: "Доп. материалы операций", CostPerUnit: moneyMul(extraMaterialsPerUnit, multiplier)})
	}
	return lines
}

func sumMaterialLines(lines []MaterialLine) (int64, int64) {
	materials := int64(0)
	consumables := int64(0)
	for _, line := range lines {
		switch line.Name {
		case "Нитки", "Упаковка", "Расходники":
			consumables += line.CostPerUnit
		default:
			materials += line.CostPerUnit
		}
	}
	return materials, consumables
}

func detectMarketStatus(settings UserSettings, segment string, pricePerUnit int64) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return "unknown"
	}
	band, ok := settings.MarketBands[segment]
	if !ok {
		return "unknown"
	}
	if pricePerUnit < band.MinPricePerUnit {
		return "below_market"
	}
	if pricePerUnit > band.MaxPricePerUnit {
		return "above_market"
	}
	return "in_market"
}

func percentOf(value int64, percent float64) int64 {
	if value <= 0 || percent <= 0 {
		return 0
	}
	return int64(math.Round(float64(value) * percent / 100.0))
}

func moneyMul(value int64, multiplier float64) int64 {
	if value <= 0 {
		return 0
	}
	return int64(math.Round(float64(value) * multiplier))
}

func childOrOne(isChild bool, childCoefficient float64) float64 {
	if isChild {
		return childCoefficient
	}
	return 1
}

func pickFirstMaterial(materials map[string]MaterialConfig) string {
	if len(materials) == 0 {
		return ""
	}
	keys := make([]string, 0, len(materials))
	for key := range materials {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0]
}

func copyGarments(src map[string]GarmentConfig) map[string]GarmentConfig {
	dst := make(map[string]GarmentConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyOperations(src map[string]OperationConfig) map[string]OperationConfig {
	dst := make(map[string]OperationConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyMaterials(src map[string]MaterialConfig) map[string]MaterialConfig {
	dst := make(map[string]MaterialConfig, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyUrgency(src map[string]UrgencyRule) map[string]UrgencyRule {
	dst := make(map[string]UrgencyRule, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func copyMarketBands(src map[string]MarketBand) map[string]MarketBand {
	dst := make(map[string]MarketBand, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
