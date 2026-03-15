CREATE TABLE IF NOT EXISTS oauth_sessions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  refresh_token_hash CHAR(64) NOT NULL UNIQUE,
  yandex_access_token TEXT NOT NULL,
  yandex_refresh_token TEXT NULL,
  access_expires_at DATETIME NOT NULL,
  refresh_expires_at DATETIME NOT NULL,
  revoked_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
