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
var _ service.ChatCalculationRepository = (*PostgresRepository)(nil)

func (r *PostgresRepository) UpsertSettings(ctx context.Context, userID string, settings service.UserSettings) error {
	basePricesJSON, err := json.Marshal(settings.BasePrices)
	if err != nil {
		return fmt.Errorf("marshal base prices: %w", err)
	}
	surchargeJSON, err := json.Marshal(settings.SurchargePercent)
	if err != nil {
		return fmt.Errorf("marshal surcharge percent: %w", err)
	}
	discountJSON, err := json.Marshal(settings.BatchDiscounts)
	if err != nil {
		return fmt.Errorf("marshal batch discounts: %w", err)
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
		`INSERT INTO users(id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_settings(user_id, base_prices, surcharge_percent, batch_discounts, updated_at)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			base_prices = EXCLUDED.base_prices,
			surcharge_percent = EXCLUDED.surcharge_percent,
			batch_discounts = EXCLUDED.batch_discounts,
			updated_at = NOW()
	`, userID, string(basePricesJSON), string(surchargeJSON), string(discountJSON))
	if err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit tx: %w", commitErr)
	}
	return nil
}

func (r *PostgresRepository) GetSettings(ctx context.Context, userID string) (service.UserSettings, error) {
	var basePricesRaw string
	var surchargeRaw string
	var discountsRaw string

	err := r.db.QueryRowContext(ctx, `
		SELECT base_prices, surcharge_percent, batch_discounts
		FROM user_settings
		WHERE user_id = $1
	`, userID).Scan(&basePricesRaw, &surchargeRaw, &discountsRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.UserSettings{}, fmt.Errorf("settings for user %q not found: %w", userID, service.ErrNotFound)
		}
		return service.UserSettings{}, fmt.Errorf("query settings: %w", err)
	}

	settings := service.UserSettings{}
	if err := json.Unmarshal([]byte(basePricesRaw), &settings.BasePrices); err != nil {
		return service.UserSettings{}, fmt.Errorf("decode base prices: %w", err)
	}
	if err := json.Unmarshal([]byte(surchargeRaw), &settings.SurchargePercent); err != nil {
		return service.UserSettings{}, fmt.Errorf("decode surcharge percent: %w", err)
	}
	if err := json.Unmarshal([]byte(discountsRaw), &settings.BatchDiscounts); err != nil {
		return service.UserSettings{}, fmt.Errorf("decode batch discounts: %w", err)
	}

	return settings, nil
}

func (r *PostgresRepository) AppendCalculation(ctx context.Context, result service.CalculationResult) error {
	appliedSurchargesJSON, err := json.Marshal(result.AppliedSurcharges)
	if err != nil {
		return fmt.Errorf("marshal applied surcharges: %w", err)
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

	_, err = tx.ExecContext(ctx, `INSERT INTO users(id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, result.UserID)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO chats(user_id, id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, id) DO NOTHING
	`, result.UserID, result.ChatID)
	if err != nil {
		return fmt.Errorf("upsert chat: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO calculations(
			user_id,
			chat_id,
			garment_type,
			quantity,
			base_price_per_unit,
			surcharge_per_unit,
			price_per_unit,
			subtotal,
			discount_percent,
			discount_amount,
			total,
			applied_surcharges,
			applied_discount_min,
			applied_discount_max,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14, $15)
	`,
		result.UserID,
		result.ChatID,
		result.GarmentType,
		result.Quantity,
		result.BasePricePerUnit,
		result.SurchargePerUnit,
		result.PricePerUnit,
		result.Subtotal,
		result.DiscountPercent,
		result.DiscountAmount,
		result.Total,
		string(appliedSurchargesJSON),
		result.AppliedDiscountMin,
		result.AppliedDiscountMax,
		result.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert calculation: %w", err)
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
			quantity,
			base_price_per_unit,
			surcharge_per_unit,
			price_per_unit,
			subtotal,
			discount_percent,
			discount_amount,
			total,
			applied_surcharges,
			applied_discount_min,
			applied_discount_max,
			created_at
		FROM calculations
		WHERE user_id = $1 AND chat_id = $2
		ORDER BY created_at ASC
	`, userID, chatID)
	if err != nil {
		return nil, fmt.Errorf("query calculations: %w", err)
	}
	defer rows.Close()

	items := make([]service.CalculationResult, 0, 16)
	for rows.Next() {
		var (
			item                 service.CalculationResult
			appliedSurchargesRaw string
		)
		if err := rows.Scan(
			&item.GarmentType,
			&item.Quantity,
			&item.BasePricePerUnit,
			&item.SurchargePerUnit,
			&item.PricePerUnit,
			&item.Subtotal,
			&item.DiscountPercent,
			&item.DiscountAmount,
			&item.Total,
			&appliedSurchargesRaw,
			&item.AppliedDiscountMin,
			&item.AppliedDiscountMax,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan calculation: %w", err)
		}

		if err := json.Unmarshal([]byte(appliedSurchargesRaw), &item.AppliedSurcharges); err != nil {
			return nil, fmt.Errorf("decode applied surcharges: %w", err)
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
