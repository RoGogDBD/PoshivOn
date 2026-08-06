ALTER TABLE users
  ADD COLUMN IF NOT EXISTS role VARCHAR(16) NOT NULL DEFAULT 'user',
  ADD COLUMN IF NOT EXISTS has_access BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS email VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) NULL,
  ADD CONSTRAINT IF NOT EXISTS chk_users_role CHECK (role IN ('user','admin'));

ALTER TABLE oauth_sessions
  ADD COLUMN IF NOT EXISTS yandex_login VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS yandex_email VARCHAR(255) NULL,
  ADD COLUMN IF NOT EXISTS yandex_display_name VARCHAR(255) NULL;

CREATE TABLE IF NOT EXISTS access_requests (
    user_id     VARCHAR(255)  NOT NULL PRIMARY KEY,
    status      VARCHAR(16)   NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at  TIMESTAMP     NULL,
    decided_by  VARCHAR(255)  NULL,
    CONSTRAINT fk_access_requests_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_access_requests_status
        CHECK (status IN ('pending','approved','rejected'))
);

CREATE INDEX IF NOT EXISTS idx_users_role_has_access ON users(role, has_access);

INSERT INTO users (id, role, has_access) VALUES
  ('RoGogDBD', 'admin', TRUE),
  ('irina2000aleshina', 'admin', TRUE)
ON DUPLICATE KEY UPDATE role = 'admin';
