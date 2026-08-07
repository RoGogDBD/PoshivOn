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

// userModel — «скелет» строки users для upsertUser: только ID и CreatedAt.
//
// ИНВАРИАНТ (Decision 13): сюда НЕЛЬЗЯ добавлять колонки доступа (role, has_access, email,
// display_name). upsertUser вставляет эту модель целиком из трёх существующих путей записи
// (UpsertSettings, CreateChat, AppendCalculation). Появись здесь поле Role — GORM включит
// role=<пустая строка> в их INSERT, и каждый из них упадёт на констрейнте из 004_access_control.up.sql:
// ERROR 4025 (23000): CONSTRAINT chk_users_role failed (проверено на живой MariaDB 11.4).
// То есть сохранение настроек, создание чата и расчёт перестанут работать для любого, кто
// ещё не проходил через EnsureUser. Колонки доступа читает и пишет userAccessModel ниже —
// отдельная модель на ту же таблицу.
type userModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (userModel) TableName() string {
	return "users"
}

// userAccessModel отображает колонки доступа той же таблицы users. Существует отдельно от
// userModel ровно по причине из инварианта выше. CreatedAt здесь нет намеренно: строку
// создаёт только EnsureUser, дату проставляет DEFAULT CURRENT_TIMESTAMP, а читается она
// через userAccessRow.
type userAccessModel struct {
	ID          string         `gorm:"column:id;primaryKey"`
	Role        string         `gorm:"column:role"`
	HasAccess   bool           `gorm:"column:has_access"`
	Email       sql.NullString `gorm:"column:email"`
	DisplayName sql.NullString `gorm:"column:display_name"`
}

func (userAccessModel) TableName() string {
	return "users"
}

// userAccessRow — результат чтения users вместе со статусом заявки. Отдельный тип от
// userAccessModel, потому что status/created_at приезжают из access_requests через LEFT JOIN
// и в таблице users колонок с такими именами нет.
type userAccessRow struct {
	ID            string         `gorm:"column:id"`
	Role          string         `gorm:"column:role"`
	HasAccess     bool           `gorm:"column:has_access"`
	Email         sql.NullString `gorm:"column:email"`
	DisplayName   sql.NullString `gorm:"column:display_name"`
	CreatedAt     time.Time      `gorm:"column:created_at"`
	RequestStatus sql.NullString `gorm:"column:request_status"`
	RequestedAt   *time.Time     `gorm:"column:requested_at"`
}

type accessRequestModel struct {
	UserID    string         `gorm:"column:user_id;primaryKey"`
	Status    string         `gorm:"column:status"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	DecidedAt *time.Time     `gorm:"column:decided_at"`
	DecidedBy sql.NullString `gorm:"column:decided_by"`
}

func (accessRequestModel) TableName() string {
	return "access_requests"
}

// requestStatusPending повторяет значение из service: одноимённая константа там
// неэкспортирована, а статусы ходят через границу обычными строками.
const requestStatusPending = "pending"

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
var _ service.UserRepository = (*PostgresRepository)(nil)
var _ service.AccessRequestRepository = (*PostgresRepository)(nil)

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

// --- UserRepository -----------------------------------------------------------------

// EnsureUser вызывается на каждом входе. Список обновляемых колонок ограничен email и
// display_name (Decision 11): попади в него role или has_access — второй вход
// администратора после выкатки сбросил бы его роль в 'user', и управление доступом
// потерялось бы вместе с последним администратором. Значения role/has_access участвуют
// только во вставке новой строки.
func (r *PostgresRepository) EnsureUser(ctx context.Context, login, email, displayName string) error {
	record := userAccessModel{
		ID:          login,
		Role:        string(service.RoleUser),
		HasAccess:   false,
		Email:       nullableString(email),
		DisplayName: nullableString(displayName),
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"email":        record.Email,
			"display_name": record.DisplayName,
		}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetUser(ctx context.Context, login string) (service.UserRecord, error) {
	var row userAccessRow
	// Scan, а не First: под Table+Select GORM не отдаёт ErrRecordNotFound однородно,
	// а число прочитанных строк однозначно и не зависит от версии.
	result := r.selectUsers(ctx).Where("u.id = ?", login).Scan(&row)
	if result.Error != nil {
		return service.UserRecord{}, fmt.Errorf("query user: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return service.UserRecord{}, fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	return toUserRecord(row), nil
}

func (r *PostgresRepository) ListUsers(ctx context.Context) ([]service.UserRecord, error) {
	var rows []userAccessRow
	// LEFT JOIN, а не INNER: в списке должны быть и те, кто просто вошёл и заявку не подавал.
	err := r.selectUsers(ctx).Order("u.created_at DESC, u.id ASC").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}

	items := make([]service.UserRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUserRecord(row))
	}
	return items, nil
}

// SetAccess переключает флаг доступа. Пустой результат UPDATE сам по себе о существовании
// пользователя ничего не говорит: MariaDB считает затронутой только реально изменённую
// строку, поэтому повторная выдача уже выданного доступа тоже даёт 0. Отличить «нет такого
// логина» от «значение и так было таким» можно только отдельным чтением — оно и делается,
// но лишь на нулевой ветке.
func (r *PostgresRepository) SetAccess(ctx context.Context, login string, granted bool) error {
	result := r.db.WithContext(ctx).
		Model(&userAccessModel{}).
		Where("id = ?", login).
		Update("has_access", granted)
	if result.Error != nil {
		return fmt.Errorf("update user access: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var exists int64
	if err := r.db.WithContext(ctx).Model(&userAccessModel{}).Where("id = ?", login).Count(&exists).Error; err != nil {
		return fmt.Errorf("check user exists: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("user %q not found: %w", login, service.ErrNotFound)
	}
	return nil
}

// --- AccessRequestRepository ----------------------------------------------------------

// createRequestSQL — оператор из Decision 5 дословно. Через сырой SQL, а не через
// clause.OnConflict: условные ветки IF(...) в ON DUPLICATE KEY UPDATE GORM выразить не умеет.
//
// Число задетых строк здесь — контракт, а не деталь: 1 — вставлена новая заявка, 2 —
// существующая реально изменена (была решена, вернулась в pending), 0 — IF оставил значения
// прежними, то есть заявка уже на рассмотрении. Это семантика «affected rows» драйвера
// go-sql-driver/mysql; она верна, пока DSN не включает clientFoundRows=true — сегодня
// server/internal/db/db.go его не задаёт (buildDSN, параметры parseTime/charset/collation/
// multiStatements). С clientFoundRows=true драйвер стал бы возвращать число совпавших строк,
// нулевая ветка исчезла бы, и повторная подача при заявке на рассмотрении перестала бы
// отклоняться.
//
// decided_by и decided_at в UPDATE-ветке не участвуют и переживают повторную подачу —
// на этом держится след прошлого решения.
const createRequestSQL = `INSERT INTO access_requests (user_id, status, created_at)
VALUES (?, 'pending', ?)
ON DUPLICATE KEY UPDATE
  status     = IF(status = 'pending', status, VALUES(status)),
  created_at = IF(status = 'pending', created_at, VALUES(created_at))`

func (r *PostgresRepository) CreateRequest(ctx context.Context, login string) error {
	result := r.db.WithContext(ctx).Exec(createRequestSQL, login, time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("upsert access request: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("access request for user %q is already pending: %w", login, service.ErrConflict)
	}
	return nil
}

func (r *PostgresRepository) GetRequest(ctx context.Context, login string) (service.AccessRequest, error) {
	var record accessRequestModel
	result := r.db.WithContext(ctx).
		Model(&accessRequestModel{}).
		Where("user_id = ?", login).
		Scan(&record)
	if result.Error != nil {
		return service.AccessRequest{}, fmt.Errorf("query access request: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return service.AccessRequest{}, fmt.Errorf("access request for user %q not found: %w", login, service.ErrNotFound)
	}

	return service.AccessRequest{
		UserID:    record.UserID,
		Status:    record.Status,
		CreatedAt: record.CreatedAt,
		DecidedAt: record.DecidedAt,
		DecidedBy: record.DecidedBy.String,
	}, nil
}

// DecideRequest фиксирует решение по уже существующей заявке. Отсутствие строки — no-op, а
// не ошибка (Decision 5): администратор вправе выдать доступ и тому, кто заявку не подавал,
// и заводить её задним числом не нужно. Поэтому RowsAffected здесь не проверяется.
func (r *PostgresRepository) DecideRequest(ctx context.Context, login, status, decidedBy string) error {
	err := r.db.WithContext(ctx).
		Model(&accessRequestModel{}).
		Where("user_id = ?", login).
		Updates(map[string]any{
			"status":     status,
			"decided_at": time.Now().UTC(),
			"decided_by": decidedBy,
		}).Error
	if err != nil {
		return fmt.Errorf("decide access request: %w", err)
	}
	return nil
}

// selectUsers — общий запрос для GetUser и ListUsers: одна строка users плюс статус её
// заявки. Держим в одном месте, чтобы список и карточка не разошлись в наборе полей.
func (r *PostgresRepository) selectUsers(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("users AS u").
		Select("u.id, u.role, u.has_access, u.email, u.display_name, u.created_at, " +
			"ar.status AS request_status, ar.created_at AS requested_at").
		Joins("LEFT JOIN access_requests ar ON ar.user_id = u.id")
}

func toUserRecord(row userAccessRow) service.UserRecord {
	return service.UserRecord{
		Login:         row.ID,
		DisplayName:   row.DisplayName.String,
		Email:         row.Email.String,
		Role:          service.Role(row.Role),
		HasAccess:     row.HasAccess,
		RequestStatus: row.RequestStatus.String,
		RequestedAt:   row.RequestedAt,
		CreatedAt:     row.CreatedAt,
	}
}

// nullableString пишет пустую строку как NULL: колонки email и display_name объявлены
// NULL-able, и «Яндекс адреса не отдал» честнее хранить как отсутствие значения, чем как
// пустую строку — иначе два разных состояния сливаются в одно.
func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func upsertUser(tx *gorm.DB, userID string) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&userModel{ID: userID}).Error
}
