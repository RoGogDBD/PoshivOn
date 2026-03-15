package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

type Session struct {
	ID                 uint64
	RefreshTokenHash   string
	YandexAccessToken  string
	YandexRefreshToken sql.NullString
	AccessExpiresAt    time.Time
	RefreshExpiresAt   time.Time
	RevokedAt          sql.NullTime
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(session *Session) error {
	query := `
		INSERT INTO oauth_sessions (
			refresh_token_hash,
			yandex_access_token,
			yandex_refresh_token,
			access_expires_at,
			refresh_expires_at,
			revoked_at,
			created_at,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(
		query,
		session.RefreshTokenHash,
		session.YandexAccessToken,
		session.YandexRefreshToken,
		session.AccessExpiresAt,
		session.RefreshExpiresAt,
		session.RevokedAt,
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	session.ID = uint64(id)
	return nil
}

func (s *Store) FindByRefreshHash(refreshHash string) (*Session, error) {
	row := s.db.QueryRow(`
		SELECT id,
			refresh_token_hash,
			yandex_access_token,
			yandex_refresh_token,
			access_expires_at,
			refresh_expires_at,
			revoked_at,
			created_at,
			updated_at
		FROM oauth_sessions
		WHERE refresh_token_hash = ?
		LIMIT 1
	`, refreshHash)

	var session Session
	if err := row.Scan(
		&session.ID,
		&session.RefreshTokenHash,
		&session.YandexAccessToken,
		&session.YandexRefreshToken,
		&session.AccessExpiresAt,
		&session.RefreshExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *Store) UpdateSessionTokens(sessionID uint64, refreshHash string, accessToken string, refreshToken sql.NullString, accessExpiresAt time.Time, refreshExpiresAt time.Time) error {
	_, err := s.db.Exec(`
		UPDATE oauth_sessions
		SET refresh_token_hash = ?,
			yandex_access_token = ?,
			yandex_refresh_token = ?,
			access_expires_at = ?,
			refresh_expires_at = ?,
			updated_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`,
		refreshHash,
		accessToken,
		refreshToken,
		accessExpiresAt,
		refreshExpiresAt,
		time.Now().UTC(),
		sessionID,
	)
	return err
}

func (s *Store) RevokeByRefreshHash(refreshHash string) error {
	result, err := s.db.Exec(`
		UPDATE oauth_sessions
		SET revoked_at = ?, updated_at = ?
		WHERE refresh_token_hash = ? AND revoked_at IS NULL
	`, time.Now().UTC(), time.Now().UTC(), refreshHash)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("session not found")
	}
	return nil
}
