package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type userModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (userModel) TableName() string {
	return "users"
}

type userSettingsModel struct {
	UserID           string         `gorm:"column:user_id;primaryKey"`
	BasePrices       string         `gorm:"column:base_prices"`
	SurchargePercent string         `gorm:"column:surcharge_percent"`
	BatchDiscounts   string         `gorm:"column:batch_discounts"`
	PricingRules     sql.NullString `gorm:"column:pricing_rules"`
	Garments         sql.NullString `gorm:"column:garments"`
	Operations       sql.NullString `gorm:"column:operations"`
	Materials        sql.NullString `gorm:"column:materials"`
	Urgency          sql.NullString `gorm:"column:urgency"`
	MarketBands      sql.NullString `gorm:"column:market_bands"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
}

func (userSettingsModel) TableName() string {
	return "user_settings"
}

type chatModel struct {
	UserID    string     `gorm:"column:user_id;primaryKey"`
	ID        string     `gorm:"column:id;primaryKey"`
	Title     string     `gorm:"column:title"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
	DeletedBy *string    `gorm:"column:deleted_by"`
}

func (chatModel) TableName() string {
	return "chats"
}

type calculationModel struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	UserID            string    `gorm:"column:user_id"`
	ChatID            string    `gorm:"column:chat_id"`
	GarmentType       string    `gorm:"column:garment_type"`
	MaterialType      string    `gorm:"column:material_type"`
	Urgency           string    `gorm:"column:urgency"`
	MarketStatus      string    `gorm:"column:market_status"`
	Quantity          int       `gorm:"column:quantity"`
	PricePerUnit      int64     `gorm:"column:price_per_unit"`
	Subtotal          int64     `gorm:"column:subtotal"`
	DiscountPercent   int64     `gorm:"column:discount_percent"`
	DiscountAmount    int64     `gorm:"column:discount_amount"`
	Total             int64     `gorm:"column:total"`
	AppliedOperations string    `gorm:"column:applied_operations"`
	MaterialLines     string    `gorm:"column:material_lines"`
	OrderSnapshot     string    `gorm:"column:order_snapshot"`
	Breakdown         string    `gorm:"column:breakdown"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (calculationModel) TableName() string {
	return "calculations"
}

type chatListRow struct {
	ID                string    `gorm:"column:id"`
	Title             string    `gorm:"column:title"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
	CalculationsCount int       `gorm:"column:calculations_count"`
}

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgresRepository(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

var _ service.UserSettingsRepository = (*PostgresRepository)(nil)
var _ service.ChatRepository = (*PostgresRepository)(nil)
var _ service.ChatCalculationRepository = (*PostgresRepository)(nil)

func (r *PostgresRepository) UpsertSettings(ctx context.Context, userID string, settings service.UserSettings) error {
	legacyBasePricesJSON, err := json.Marshal(map[string]int64{})
	if err != nil {
		return fmt.Errorf("marshal legacy base prices: %w", err)
	}
	legacySurchargeJSON, err := json.Marshal(map[string]float64{})
	if err != nil {
		return fmt.Errorf("marshal legacy surcharge percent: %w", err)
	}
	garmentsJSON, err := json.Marshal(settings.Garments)
	if err != nil {
		return fmt.Errorf("marshal garments: %w", err)
	}
	operationsJSON, err := json.Marshal(settings.Operations)
	if err != nil {
		return fmt.Errorf("marshal operations: %w", err)
	}
	materialsJSON, err := json.Marshal(settings.Materials)
	if err != nil {
		return fmt.Errorf("marshal materials: %w", err)
	}
	discountsJSON, err := json.Marshal(settings.BatchDiscounts)
	if err != nil {
		return fmt.Errorf("marshal batch discounts: %w", err)
	}
	pricingRulesJSON, err := json.Marshal(settings.PricingRules)
	if err != nil {
		return fmt.Errorf("marshal pricing rules: %w", err)
	}
	urgencyJSON, err := json.Marshal(settings.Urgency)
	if err != nil {
		return fmt.Errorf("marshal urgency: %w", err)
	}
	marketBandsJSON, err := json.Marshal(settings.MarketBands)
	if err != nil {
		return fmt.Errorf("marshal market bands: %w", err)
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertUser(tx, userID); err != nil {
			return err
		}

		record := userSettingsModel{
			UserID:           userID,
			BasePrices:       string(legacyBasePricesJSON),
			SurchargePercent: string(legacySurchargeJSON),
			BatchDiscounts:   string(discountsJSON),
			PricingRules:     sql.NullString{String: string(pricingRulesJSON), Valid: true},
			Garments:         sql.NullString{String: string(garmentsJSON), Valid: true},
			Operations:       sql.NullString{String: string(operationsJSON), Valid: true},
			Materials:        sql.NullString{String: string(materialsJSON), Valid: true},
			Urgency:          sql.NullString{String: string(urgencyJSON), Valid: true},
			MarketBands:      sql.NullString{String: string(marketBandsJSON), Valid: true},
			UpdatedAt:        time.Now().UTC(),
		}

		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"base_prices":       record.BasePrices,
				"surcharge_percent": record.SurchargePercent,
				"batch_discounts":   record.BatchDiscounts,
				"pricing_rules":     record.PricingRules,
				"garments":          record.Garments,
				"operations":        record.Operations,
				"materials":         record.Materials,
				"urgency":           record.Urgency,
				"market_bands":      record.MarketBands,
				"updated_at":        record.UpdatedAt,
			}),
		}).Create(&record).Error
	})
	if err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetSettings(ctx context.Context, userID string) (service.UserSettings, error) {
	var record userSettingsModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return service.UserSettings{}, fmt.Errorf("settings for user %q not found: %w", userID, service.ErrNotFound)
		}
		return service.UserSettings{}, fmt.Errorf("query settings: %w", err)
	}

	settings := service.DefaultUserSettings()
	if record.PricingRules.Valid && record.PricingRules.String != "" {
		if err := json.Unmarshal([]byte(record.PricingRules.String), &settings.PricingRules); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode pricing rules: %w", err)
		}
	}
	if record.Garments.Valid && record.Garments.String != "" {
		if err := json.Unmarshal([]byte(record.Garments.String), &settings.Garments); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode garments: %w", err)
		}
	}
	if record.Operations.Valid && record.Operations.String != "" {
		if err := json.Unmarshal([]byte(record.Operations.String), &settings.Operations); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode operations: %w", err)
		}
	}
	if record.Materials.Valid && record.Materials.String != "" {
		if err := json.Unmarshal([]byte(record.Materials.String), &settings.Materials); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode materials: %w", err)
		}
	}
	if record.BatchDiscounts != "" {
		if err := json.Unmarshal([]byte(record.BatchDiscounts), &settings.BatchDiscounts); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode batch discounts: %w", err)
		}
	}
	if record.Urgency.Valid && record.Urgency.String != "" {
		if err := json.Unmarshal([]byte(record.Urgency.String), &settings.Urgency); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode urgency: %w", err)
		}
	}
	if record.MarketBands.Valid && record.MarketBands.String != "" {
		if err := json.Unmarshal([]byte(record.MarketBands.String), &settings.MarketBands); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode market bands: %w", err)
		}
	}

	return settings, nil
}

func (r *PostgresRepository) CreateChat(ctx context.Context, chat service.Chat) (service.Chat, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertUser(tx, chat.UserID); err != nil {
			return err
		}
		record := chatModel{
			UserID:    chat.UserID,
			ID:        chat.ID,
			Title:     chat.Title,
			CreatedAt: chat.CreatedAt,
			UpdatedAt: chat.UpdatedAt,
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return service.Chat{}, fmt.Errorf("insert chat: %w", err)
	}

	return chat, nil
}

func (r *PostgresRepository) ListChats(ctx context.Context, userID string) ([]service.Chat, error) {
	var rows []chatListRow
	err := r.db.WithContext(ctx).
		Table("chats AS c").
		Select("c.id, c.title, c.created_at, c.updated_at, COUNT(calc.id) AS calculations_count").
		Joins("LEFT JOIN calculations calc ON calc.user_id = c.user_id AND calc.chat_id = c.id").
		Where("c.user_id = ? AND c.deleted_at IS NULL", userID).
		Group("c.user_id, c.id, c.title, c.created_at, c.updated_at").
		Order("c.updated_at DESC, c.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query chats: %w", err)
	}

	items := make([]service.Chat, 0, len(rows))
	for _, row := range rows {
		items = append(items, service.Chat{
			UserID:            userID,
			ID:                row.ID,
			Title:             row.Title,
			CreatedAt:         row.CreatedAt,
			UpdatedAt:         row.UpdatedAt,
			CalculationsCount: row.CalculationsCount,
		})
	}

	return items, nil
}

func (r *PostgresRepository) DeleteChat(ctx context.Context, userID, chatID, deletedBy string, hard bool) error {
	query := r.db.WithContext(ctx).Model(&chatModel{}).Where("user_id = ? AND id = ?", userID, chatID)
	var result *gorm.DB
	if hard {
		result = query.Delete(&chatModel{})
		if result.Error != nil {
			return fmt.Errorf("delete chat permanently: %w", result.Error)
		}
		return ensureAffected(result.RowsAffected, chatID)
	}

	now := time.Now().UTC()
	result = query.Updates(map[string]any{
		"deleted_at": now,
		"deleted_by": deletedBy,
	})
	if result.Error != nil {
		return fmt.Errorf("soft delete chat: %w", result.Error)
	}
	return ensureAffected(result.RowsAffected, chatID)
}

func (r *PostgresRepository) RestoreChat(ctx context.Context, userID, chatID string) error {
	result := r.db.WithContext(ctx).
		Model(&chatModel{}).
		Where("user_id = ? AND id = ?", userID, chatID).
		Updates(map[string]any{
			"deleted_at": nil,
			"deleted_by": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("restore chat: %w", result.Error)
	}
	return ensureAffected(result.RowsAffected, chatID)
}

func (r *PostgresRepository) AppendCalculation(ctx context.Context, result service.CalculationResult) error {
	appliedOperationsJSON, err := json.Marshal(result.AppliedOperations)
	if err != nil {
		return fmt.Errorf("marshal applied operations: %w", err)
	}
	materialLinesJSON, err := json.Marshal(result.MaterialLines)
	if err != nil {
		return fmt.Errorf("marshal material lines: %w", err)
	}
	orderSnapshotJSON, err := json.Marshal(map[string]any{
		"calculation_mode": result.CalculationMode,
		"garment_type":     result.GarmentType,
		"material_type":    result.MaterialType,
		"urgency":          result.Urgency,
		"market_segment":   result.MarketSegment,
		"quantity":         result.Quantity,
		"fittings":         result.Fittings,
		"is_custom_figure": result.IsCustomFigure,
		"is_child":         result.IsChild,
		"comment":          result.Comment,
	})
	if err != nil {
		return fmt.Errorf("marshal order snapshot: %w", err)
	}
	breakdownJSON, err := json.Marshal(map[string]any{
		"base_minutes_per_unit":          result.BaseMinutesPerUnit,
		"operation_minutes_per_unit":     result.OperationMinutesPerUnit,
		"fitting_minutes_per_unit":       result.FittingMinutesPerUnit,
		"adjusted_minutes_per_unit":      result.AdjustedMinutesPerUnit,
		"labor_cost_per_unit":            result.LaborCostPerUnit,
		"payroll_cost_per_unit":          result.PayrollCostPerUnit,
		"materials_cost_per_unit":        result.MaterialsCostPerUnit,
		"consumables_cost_per_unit":      result.ConsumablesCostPerUnit,
		"overhead_cost_per_unit":         result.OverheadCostPerUnit,
		"logistics_cost_per_unit":        result.LogisticsCostPerUnit,
		"risk_reserve_per_unit":          result.RiskReservePerUnit,
		"cost_price_per_unit":            result.CostPricePerUnit,
		"margin_per_unit":                result.MarginPerUnit,
		"price_before_discount_per_unit": result.PriceBeforeDiscount,
		"min_allowed_price_per_unit":     result.MinAllowedPricePerUnit,
	})
	if err != nil {
		return fmt.Errorf("marshal breakdown: %w", err)
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertUser(tx, result.UserID); err != nil {
			return err
		}

		chat := chatModel{
			UserID:    result.UserID,
			ID:        result.ChatID,
			Title:     "Новый чат",
			CreatedAt: result.CreatedAt,
			UpdatedAt: result.CreatedAt,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"updated_at": result.CreatedAt,
				"deleted_at": nil,
				"deleted_by": nil,
			}),
		}).Create(&chat).Error; err != nil {
			return fmt.Errorf("upsert chat: %w", err)
		}

		record := calculationModel{
			UserID:            result.UserID,
			ChatID:            result.ChatID,
			GarmentType:       result.GarmentType,
			MaterialType:      result.MaterialType,
			Urgency:           result.Urgency,
			MarketStatus:      result.MarketStatus,
			Quantity:          result.Quantity,
			PricePerUnit:      result.PricePerUnit,
			Subtotal:          result.Subtotal,
			DiscountPercent:   result.DiscountPercent,
			DiscountAmount:    result.DiscountAmount,
			Total:             result.Total,
			AppliedOperations: string(appliedOperationsJSON),
			MaterialLines:     string(materialLinesJSON),
			OrderSnapshot:     string(orderSnapshotJSON),
			Breakdown:         string(breakdownJSON),
			CreatedAt:         result.CreatedAt,
		}

		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("insert calculation: %w", err)
		}

		if err := tx.Model(&chatModel{}).
			Where("user_id = ? AND id = ?", result.UserID, result.ChatID).
			Updates(map[string]any{
				"updated_at": result.CreatedAt,
				"deleted_at": nil,
				"deleted_by": nil,
			}).Error; err != nil {
			return fmt.Errorf("update chat timestamp: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *PostgresRepository) ListCalculations(ctx context.Context, userID, chatID string) ([]service.CalculationResult, error) {
	var rows []calculationModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND chat_id = ?", userID, chatID).
		Order("created_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query calculations: %w", err)
	}

	items := make([]service.CalculationResult, 0, len(rows))
	for _, row := range rows {
		item := service.CalculationResult{
			UserID:          userID,
			ChatID:          chatID,
			GarmentType:     row.GarmentType,
			MaterialType:    row.MaterialType,
			Urgency:         row.Urgency,
			MarketStatus:    row.MarketStatus,
			Quantity:        row.Quantity,
			PricePerUnit:    row.PricePerUnit,
			Subtotal:        row.Subtotal,
			DiscountPercent: row.DiscountPercent,
			DiscountAmount:  row.DiscountAmount,
			Total:           row.Total,
			CreatedAt:       row.CreatedAt,
		}

		if err := json.Unmarshal([]byte(row.AppliedOperations), &item.AppliedOperations); err != nil {
			return nil, fmt.Errorf("decode applied operations: %w", err)
		}
		if err := json.Unmarshal([]byte(row.MaterialLines), &item.MaterialLines); err != nil {
			return nil, fmt.Errorf("decode material lines: %w", err)
		}
		if err := decodeOrderSnapshot(row.OrderSnapshot, &item); err != nil {
			return nil, err
		}
		if err := decodeBreakdown(row.Breakdown, &item); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, nil
}

func decodeOrderSnapshot(raw string, item *service.CalculationResult) error {
	var payload struct {
		CalculationMode string `json:"calculation_mode"`
		MarketSegment   string `json:"market_segment"`
		Fittings        int    `json:"fittings"`
		IsCustomFigure  bool   `json:"is_custom_figure"`
		IsChild         bool   `json:"is_child"`
		Comment         string `json:"comment"`
	}
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("decode order snapshot: %w", err)
	}
	item.CalculationMode = payload.CalculationMode
	item.MarketSegment = payload.MarketSegment
	item.Fittings = payload.Fittings
	item.IsCustomFigure = payload.IsCustomFigure
	item.IsChild = payload.IsChild
	item.Comment = payload.Comment
	return nil
}

func decodeBreakdown(raw string, item *service.CalculationResult) error {
	var payload struct {
		BaseMinutesPerUnit      int                           `json:"base_minutes_per_unit"`
		OperationMinutesPerUnit int                           `json:"operation_minutes_per_unit"`
		FittingMinutesPerUnit   int                           `json:"fitting_minutes_per_unit"`
		AdjustedMinutesPerUnit  int                           `json:"adjusted_minutes_per_unit"`
		LaborCostPerUnit        int64                         `json:"labor_cost_per_unit"`
		PayrollCostPerUnit      int64                         `json:"payroll_cost_per_unit"`
		MaterialsCostPerUnit    int64                         `json:"materials_cost_per_unit"`
		ConsumablesCostPerUnit  int64                         `json:"consumables_cost_per_unit"`
		OverheadCostPerUnit     int64                         `json:"overhead_cost_per_unit"`
		LogisticsCostPerUnit    int64                         `json:"logistics_cost_per_unit"`
		RiskReservePerUnit      int64                         `json:"risk_reserve_per_unit"`
		CostPricePerUnit        int64                         `json:"cost_price_per_unit"`
		MarginPerUnit           int64                         `json:"margin_per_unit"`
		PriceBeforeDiscount     int64                         `json:"price_before_discount_per_unit"`
		MinAllowedPricePerUnit  int64                         `json:"min_allowed_price_per_unit"`
		AIFeedback              *service.MarketFeedbackResult `json:"ai_feedback"`
	}
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("decode breakdown: %w", err)
	}
	item.BaseMinutesPerUnit = payload.BaseMinutesPerUnit
	item.OperationMinutesPerUnit = payload.OperationMinutesPerUnit
	item.FittingMinutesPerUnit = payload.FittingMinutesPerUnit
	item.AdjustedMinutesPerUnit = payload.AdjustedMinutesPerUnit
	item.LaborCostPerUnit = payload.LaborCostPerUnit
	item.PayrollCostPerUnit = payload.PayrollCostPerUnit
	item.MaterialsCostPerUnit = payload.MaterialsCostPerUnit
	item.ConsumablesCostPerUnit = payload.ConsumablesCostPerUnit
	item.OverheadCostPerUnit = payload.OverheadCostPerUnit
	item.LogisticsCostPerUnit = payload.LogisticsCostPerUnit
	item.RiskReservePerUnit = payload.RiskReservePerUnit
	item.CostPricePerUnit = payload.CostPricePerUnit
	item.MarginPerUnit = payload.MarginPerUnit
	item.PriceBeforeDiscount = payload.PriceBeforeDiscount
	item.MinAllowedPricePerUnit = payload.MinAllowedPricePerUnit
	item.AIFeedback = payload.AIFeedback
	return nil
}

func ensureAffected(affected int64, chatID string) error {
	if affected == 0 {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}
	return nil
}

func upsertUser(tx *gorm.DB, userID string) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&userModel{ID: userID}).Error
}
