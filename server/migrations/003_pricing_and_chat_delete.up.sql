ALTER TABLE user_settings
    ADD COLUMN IF NOT EXISTS pricing_rules JSON NULL AFTER batch_discounts,
    ADD COLUMN IF NOT EXISTS garments JSON NULL AFTER pricing_rules,
    ADD COLUMN IF NOT EXISTS operations JSON NULL AFTER garments,
    ADD COLUMN IF NOT EXISTS materials JSON NULL AFTER operations,
    ADD COLUMN IF NOT EXISTS urgency JSON NULL AFTER materials,
    ADD COLUMN IF NOT EXISTS market_bands JSON NULL AFTER urgency;

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP NULL DEFAULT NULL AFTER updated_at,
    ADD COLUMN IF NOT EXISTS deleted_by VARCHAR(255) NULL AFTER deleted_at;

ALTER TABLE calculations
    ADD COLUMN IF NOT EXISTS material_type VARCHAR(255) NULL AFTER garment_type,
    ADD COLUMN IF NOT EXISTS urgency VARCHAR(255) NULL AFTER material_type,
    ADD COLUMN IF NOT EXISTS market_status VARCHAR(64) NULL AFTER urgency,
    ADD COLUMN IF NOT EXISTS applied_operations JSON NULL AFTER total,
    ADD COLUMN IF NOT EXISTS material_lines JSON NULL AFTER applied_operations,
    ADD COLUMN IF NOT EXISTS order_snapshot JSON NULL AFTER material_lines,
    ADD COLUMN IF NOT EXISTS breakdown JSON NULL AFTER order_snapshot,
    MODIFY COLUMN base_price_per_unit BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN surcharge_per_unit BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN price_per_unit BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN subtotal BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN discount_percent BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN discount_amount BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN total BIGINT NOT NULL DEFAULT 0,
    MODIFY COLUMN applied_surcharges JSON NULL,
    MODIFY COLUMN applied_discount_min INT NOT NULL DEFAULT 0,
    MODIFY COLUMN applied_discount_max INT NOT NULL DEFAULT 0;
