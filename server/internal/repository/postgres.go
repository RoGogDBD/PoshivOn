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
		`INSERT INTO users(id) VALUES (?) ON DUPLICATE KEY UPDATE id = VALUES(id)`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_settings(user_id, base_prices, surcharge_percent, batch_discounts, updated_at)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP())
		ON DUPLICATE KEY UPDATE
			base_prices = VALUES(base_prices),
			surcharge_percent = VALUES(surcharge_percent),
			batch_discounts = VALUES(batch_discounts),
			updated_at = UTC_TIMESTAMP()
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
		WHERE user_id = ?
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

func (r *PostgresRepository) CreateChat(ctx context.Context, chat service.Chat) (service.Chat, error) {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chats(user_id, id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
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
		WHERE c.user_id = ?
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO users(id) VALUES (?)
		ON DUPLICATE KEY UPDATE id = VALUES(id)
	`, result.UserID)
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO chats(user_id, id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)
	`, result.UserID, result.ChatID, "Новый чат", result.CreatedAt, result.CreatedAt)
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

	_, err = tx.ExecContext(ctx, `
		UPDATE chats
		SET updated_at = ?
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
