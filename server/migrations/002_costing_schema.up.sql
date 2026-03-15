CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id VARCHAR(255) PRIMARY KEY,
    base_prices JSON NOT NULL,
    surcharge_percent JSON NOT NULL,
    batch_discounts JSON NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_user_settings_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chats (
    user_id VARCHAR(255) NOT NULL,
    id VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, id),
    CONSTRAINT fk_chats_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS calculations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    chat_id VARCHAR(64) NOT NULL,
    garment_type VARCHAR(255) NOT NULL,
    quantity INT NOT NULL,
    base_price_per_unit BIGINT NOT NULL,
    surcharge_per_unit BIGINT NOT NULL,
    price_per_unit BIGINT NOT NULL,
    subtotal BIGINT NOT NULL,
    discount_percent BIGINT NOT NULL,
    discount_amount BIGINT NOT NULL,
    total BIGINT NOT NULL,
    applied_surcharges JSON NOT NULL,
    applied_discount_min INT NOT NULL,
    applied_discount_max INT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_calculations_chat
        FOREIGN KEY (user_id, chat_id) REFERENCES chats(user_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_calculations_quantity CHECK (quantity > 0),
    CONSTRAINT chk_calculations_base_price CHECK (base_price_per_unit >= 0),
    CONSTRAINT chk_calculations_surcharge CHECK (surcharge_per_unit >= 0),
    CONSTRAINT chk_calculations_price CHECK (price_per_unit >= 0),
    CONSTRAINT chk_calculations_subtotal CHECK (subtotal >= 0),
    CONSTRAINT chk_calculations_discount_percent CHECK (discount_percent >= 0),
    CONSTRAINT chk_calculations_discount_amount CHECK (discount_amount >= 0),
    CONSTRAINT chk_calculations_total CHECK (total >= 0)
);

CREATE INDEX idx_calculations_user_chat_created_at
    ON calculations(user_id, chat_id, created_at DESC);

CREATE INDEX idx_chats_user_updated_at
    ON chats(user_id, updated_at DESC);
