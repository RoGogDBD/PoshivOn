-- Initial schema for tailoring costing service.
-- PostgreSQL 14+

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    base_prices JSONB NOT NULL,
    surcharge_percent JSONB NOT NULL,
    batch_discounts JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS chats (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, id)
);

CREATE TABLE IF NOT EXISTS calculations (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id TEXT NOT NULL,
    garment_type TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    base_price_per_unit BIGINT NOT NULL CHECK (base_price_per_unit >= 0),
    surcharge_per_unit BIGINT NOT NULL CHECK (surcharge_per_unit >= 0),
    price_per_unit BIGINT NOT NULL CHECK (price_per_unit >= 0),
    subtotal BIGINT NOT NULL CHECK (subtotal >= 0),
    discount_percent BIGINT NOT NULL CHECK (discount_percent >= 0),
    discount_amount BIGINT NOT NULL CHECK (discount_amount >= 0),
    total BIGINT NOT NULL CHECK (total >= 0),
    applied_surcharges JSONB NOT NULL,
    applied_discount_min INTEGER NOT NULL,
    applied_discount_max INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (user_id, chat_id) REFERENCES chats(user_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_calculations_user_chat_created_at
    ON calculations(user_id, chat_id, created_at DESC);
