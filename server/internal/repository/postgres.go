package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users(id) VALUES (?) ON DUPLICATE KEY UPDATE id = VALUES(id)`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_settings(
			user_id,
			base_prices,
			surcharge_percent,
			batch_discounts,
			pricing_rules,
			garments,
			operations,
			materials,
			urgency,
			market_bands,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP())
		ON DUPLICATE KEY UPDATE
			base_prices = VALUES(base_prices),
			surcharge_percent = VALUES(surcharge_percent),
			batch_discounts = VALUES(batch_discounts),
			pricing_rules = VALUES(pricing_rules),
			garments = VALUES(garments),
			operations = VALUES(operations),
			materials = VALUES(materials),
			urgency = VALUES(urgency),
			market_bands = VALUES(market_bands),
			updated_at = UTC_TIMESTAMP()
	`, userID, string(legacyBasePricesJSON), string(legacySurchargeJSON), string(discountsJSON), string(pricingRulesJSON), string(garmentsJSON), string(operationsJSON), string(materialsJSON), string(urgencyJSON), string(marketBandsJSON))
	if err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit tx: %w", commitErr)
	}
	return nil
}

func (r *PostgresRepository) GetSettings(ctx context.Context, userID string) (service.UserSettings, error) {
	var (
		pricingRulesRaw sql.NullString
		garmentsRaw     sql.NullString
		operationsRaw   sql.NullString
		materialsRaw    sql.NullString
		discountsRaw    sql.NullString
		urgencyRaw      sql.NullString
		marketBandsRaw  sql.NullString
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT pricing_rules, garments, operations, materials, batch_discounts, urgency, market_bands
		FROM user_settings
		WHERE user_id = ?
	`, userID).Scan(
		&pricingRulesRaw,
		&garmentsRaw,
		&operationsRaw,
		&materialsRaw,
		&discountsRaw,
		&urgencyRaw,
		&marketBandsRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.UserSettings{}, fmt.Errorf("settings for user %q not found: %w", userID, service.ErrNotFound)
		}
		return service.UserSettings{}, fmt.Errorf("query settings: %w", err)
	}

	settings := service.DefaultUserSettings()
	if pricingRulesRaw.Valid && pricingRulesRaw.String != "" {
		if err := json.Unmarshal([]byte(pricingRulesRaw.String), &settings.PricingRules); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode pricing rules: %w", err)
		}
	}
	if garmentsRaw.Valid && garmentsRaw.String != "" {
		if err := json.Unmarshal([]byte(garmentsRaw.String), &settings.Garments); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode garments: %w", err)
		}
	}
	if operationsRaw.Valid && operationsRaw.String != "" {
		if err := json.Unmarshal([]byte(operationsRaw.String), &settings.Operations); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode operations: %w", err)
		}
	}
	if materialsRaw.Valid && materialsRaw.String != "" {
		if err := json.Unmarshal([]byte(materialsRaw.String), &settings.Materials); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode materials: %w", err)
		}
	}
	if discountsRaw.Valid && discountsRaw.String != "" {
		if err := json.Unmarshal([]byte(discountsRaw.String), &settings.BatchDiscounts); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode batch discounts: %w", err)
		}
	}
	if urgencyRaw.Valid && urgencyRaw.String != "" {
		if err := json.Unmarshal([]byte(urgencyRaw.String), &settings.Urgency); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode urgency: %w", err)
		}
	}
	if marketBandsRaw.Valid && marketBandsRaw.String != "" {
		if err := json.Unmarshal([]byte(marketBandsRaw.String), &settings.MarketBands); err != nil {
			return service.UserSettings{}, fmt.Errorf("decode market bands: %w", err)
		}
	}

	return settings, nil
}

func (r *PostgresRepository) CreateChat(ctx context.Context, chat service.Chat) (service.Chat, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chats(user_id, id, title, created_at, updated_at, deleted_at, deleted_by)
		VALUES (?, ?, ?, ?, ?, NULL, NULL)
	`, chat.UserID, chat.ID, chat.Title, chat.CreatedAt, chat.UpdatedAt)
	if err != nil {
		return service.Chat{}, fmt.Errorf("insert chat: %w", err)
	}
	return chat, nil
}

func (r *PostgresRepository) ListChats(ctx context.Context, userID string) ([]service.Chat, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id,
			c.title,
			c.created_at,
			c.updated_at,
			COUNT(calc.id) AS calculations_count
		FROM chats c
		LEFT JOIN calculations calc
			ON calc.user_id = c.user_id AND calc.chat_id = c.id
		WHERE c.user_id = ? AND c.deleted_at IS NULL
		GROUP BY c.user_id, c.id, c.title, c.created_at, c.updated_at
		ORDER BY c.updated_at DESC, c.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query chats: %w", err)
	}
	defer rows.Close()

	items := make([]service.Chat, 0, 16)
	for rows.Next() {
		var item service.Chat
		item.UserID = userID
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.CalculationsCount,
		); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chats: %w", err)
	}

	return items, nil
}

func (r *PostgresRepository) DeleteChat(ctx context.Context, userID, chatID, deletedBy string, hard bool) error {
	if hard {
		result, err := r.db.ExecContext(ctx, `DELETE FROM chats WHERE user_id = ? AND id = ?`, userID, chatID)
		if err != nil {
			return fmt.Errorf("delete chat permanently: %w", err)
		}
		return ensureAffected(result, chatID)
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE chats
		SET deleted_at = UTC_TIMESTAMP(), deleted_by = ?
		WHERE user_id = ? AND id = ? AND deleted_at IS NULL
	`, deletedBy, userID, chatID)
	if err != nil {
		return fmt.Errorf("soft delete chat: %w", err)
	}
	return ensureAffected(result, chatID)
}

func (r *PostgresRepository) RestoreChat(ctx context.Context, userID, chatID string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE chats
		SET deleted_at = NULL, deleted_by = NULL
		WHERE user_id = ? AND id = ?
	`, userID, chatID)
	if err != nil {
		return fmt.Errorf("restore chat: %w", err)
	}
	return ensureAffected(result, chatID)
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
		"garment_type":   result.GarmentType,
		"material_type":  result.MaterialType,
		"urgency":        result.Urgency,
		"market_segment": result.MarketSegment,
		"quantity":       result.Quantity,
		"fittings":       result.Fittings,
		"comment":        result.Comment,
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users(id) VALUES (?)
		ON DUPLICATE KEY UPDATE id = VALUES(id)
	`, result.UserID)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO chats(user_id, id, title, created_at, updated_at, deleted_at, deleted_by)
		VALUES (?, ?, ?, ?, ?, NULL, NULL)
		ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at), deleted_at = NULL, deleted_by = NULL
	`, result.UserID, result.ChatID, "Новый чат", result.CreatedAt, result.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert chat: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO calculations(
			user_id,
			chat_id,
			garment_type,
			material_type,
			urgency,
			market_status,
			quantity,
			price_per_unit,
			subtotal,
			discount_percent,
			discount_amount,
			total,
			applied_operations,
			material_lines,
			order_snapshot,
			breakdown,
			created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		result.UserID,
		result.ChatID,
		result.GarmentType,
		result.MaterialType,
		result.Urgency,
		result.MarketStatus,
		result.Quantity,
		result.PricePerUnit,
		result.Subtotal,
		result.DiscountPercent,
		result.DiscountAmount,
		result.Total,
		string(appliedOperationsJSON),
		string(materialLinesJSON),
		string(orderSnapshotJSON),
		string(breakdownJSON),
		result.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert calculation: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE chats
		SET updated_at = ?, deleted_at = NULL, deleted_by = NULL
		WHERE user_id = ? AND id = ?
	`, result.CreatedAt, result.UserID, result.ChatID)
	if err != nil {
		return fmt.Errorf("update chat timestamp: %w", err)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit tx: %w", commitErr)
	}
	return nil
}

func (r *PostgresRepository) ListCalculations(ctx context.Context, userID, chatID string) ([]service.CalculationResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			garment_type,
			material_type,
			urgency,
			market_status,
			quantity,
			price_per_unit,
			subtotal,
			discount_percent,
			discount_amount,
			total,
			applied_operations,
			material_lines,
			order_snapshot,
			breakdown,
			created_at
		FROM calculations
		WHERE user_id = ? AND chat_id = ?
		ORDER BY created_at ASC
	`, userID, chatID)
	if err != nil {
		return nil, fmt.Errorf("query calculations: %w", err)
	}
	defer rows.Close()

	items := make([]service.CalculationResult, 0, 16)
	for rows.Next() {
		var (
			item             service.CalculationResult
			appliedRaw       string
			materialLinesRaw string
			orderSnapshotRaw string
			breakdownRaw     string
		)
		if err := rows.Scan(
			&item.GarmentType,
			&item.MaterialType,
			&item.Urgency,
			&item.MarketStatus,
			&item.Quantity,
			&item.PricePerUnit,
			&item.Subtotal,
			&item.DiscountPercent,
			&item.DiscountAmount,
			&item.Total,
			&appliedRaw,
			&materialLinesRaw,
			&orderSnapshotRaw,
			&breakdownRaw,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan calculation: %w", err)
		}

		if err := json.Unmarshal([]byte(appliedRaw), &item.AppliedOperations); err != nil {
			return nil, fmt.Errorf("decode applied operations: %w", err)
		}
		if err := json.Unmarshal([]byte(materialLinesRaw), &item.MaterialLines); err != nil {
			return nil, fmt.Errorf("decode material lines: %w", err)
		}
		if err := decodeOrderSnapshot(orderSnapshotRaw, &item); err != nil {
			return nil, err
		}
		if err := decodeBreakdown(breakdownRaw, &item); err != nil {
			return nil, err
		}
		item.UserID = userID
		item.ChatID = chatID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calculations: %w", err)
	}
	return items, nil
}

func decodeOrderSnapshot(raw string, item *service.CalculationResult) error {
	var payload struct {
		MarketSegment string `json:"market_segment"`
		Fittings      int    `json:"fittings"`
		Comment       string `json:"comment"`
	}
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return fmt.Errorf("decode order snapshot: %w", err)
	}
	item.MarketSegment = payload.MarketSegment
	item.Fittings = payload.Fittings
	item.Comment = payload.Comment
	return nil
}

func decodeBreakdown(raw string, item *service.CalculationResult) error {
	var payload struct {
		BaseMinutesPerUnit      int   `json:"base_minutes_per_unit"`
		OperationMinutesPerUnit int   `json:"operation_minutes_per_unit"`
		FittingMinutesPerUnit   int   `json:"fitting_minutes_per_unit"`
		AdjustedMinutesPerUnit  int   `json:"adjusted_minutes_per_unit"`
		LaborCostPerUnit        int64 `json:"labor_cost_per_unit"`
		PayrollCostPerUnit      int64 `json:"payroll_cost_per_unit"`
		MaterialsCostPerUnit    int64 `json:"materials_cost_per_unit"`
		ConsumablesCostPerUnit  int64 `json:"consumables_cost_per_unit"`
		OverheadCostPerUnit     int64 `json:"overhead_cost_per_unit"`
		LogisticsCostPerUnit    int64 `json:"logistics_cost_per_unit"`
		RiskReservePerUnit      int64 `json:"risk_reserve_per_unit"`
		CostPricePerUnit        int64 `json:"cost_price_per_unit"`
		MarginPerUnit           int64 `json:"margin_per_unit"`
		PriceBeforeDiscount     int64 `json:"price_before_discount_per_unit"`
		MinAllowedPricePerUnit  int64 `json:"min_allowed_price_per_unit"`
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
	return nil
}

func ensureAffected(result sql.Result, chatID string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("chat %q not found: %w", chatID, service.ErrNotFound)
	}
	return nil
}
