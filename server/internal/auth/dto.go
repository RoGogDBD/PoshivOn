package auth

import (
	"database/sql"
	"time"
)

// SessionDTO — Session на wire (JSON) для RPC-границы между бэкендом и db-service: сессии
// входа идут тем же путём, что и остальные данные, когда бэкенду недоступен прямой SQL (см.
// internal/dbservice и internal/repository/http.go). Session сама без json-тегов (её
// sql.NullString/sql.NullTime читает и пишет только database/sql) — раньше это решалось
// двумя независимо поддерживаемыми копиями этого типа по разные стороны HTTP-границы; здесь
// единственный источник истины, импортируемый обеими сторонами (найдено при code review:
// ничего в системе типов не удержало бы две копии в синхроне, JSON-тег мог разойтись молча).
type SessionDTO struct {
	ID                 uint64     `json:"id"`
	RefreshTokenHash   string     `json:"refresh_token_hash"`
	YandexAccessToken  string     `json:"yandex_access_token"`
	YandexRefreshToken *string    `json:"yandex_refresh_token"`
	YandexLogin        *string    `json:"yandex_login"`
	YandexEmail        *string    `json:"yandex_email"`
	YandexDisplayName  *string    `json:"yandex_display_name"`
	AccessExpiresAt    time.Time  `json:"access_expires_at"`
	RefreshExpiresAt   time.Time  `json:"refresh_expires_at"`
	RevokedAt          *time.Time `json:"revoked_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ToDTO переводит Session в wire-форму. *string/*time.Time вместо sql.NullString/sql.NullTime
// — иначе на wire ушла бы форма {"String":"...","Valid":true}, случайная, а не осознанная.
func (s Session) ToDTO() SessionDTO {
	return SessionDTO{
		ID:                 s.ID,
		RefreshTokenHash:   s.RefreshTokenHash,
		YandexAccessToken:  s.YandexAccessToken,
		YandexRefreshToken: StringPtr(s.YandexRefreshToken),
		YandexLogin:        StringPtr(s.YandexLogin),
		YandexEmail:        StringPtr(s.YandexEmail),
		YandexDisplayName:  StringPtr(s.YandexDisplayName),
		AccessExpiresAt:    s.AccessExpiresAt,
		RefreshExpiresAt:   s.RefreshExpiresAt,
		RevokedAt:          TimePtr(s.RevokedAt),
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

// ToSession — обратное преобразование.
func (d SessionDTO) ToSession() Session {
	return Session{
		ID:                 d.ID,
		RefreshTokenHash:   d.RefreshTokenHash,
		YandexAccessToken:  d.YandexAccessToken,
		YandexRefreshToken: NullString(d.YandexRefreshToken),
		YandexLogin:        NullString(d.YandexLogin),
		YandexEmail:        NullString(d.YandexEmail),
		YandexDisplayName:  NullString(d.YandexDisplayName),
		AccessExpiresAt:    d.AccessExpiresAt,
		RefreshExpiresAt:   d.RefreshExpiresAt,
		RevokedAt:          NullTime(d.RevokedAt),
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
	}
}

// StringPtr/NullString, TimePtr/NullTime — экспортированы, а не только внутренние детали
// ToDTO/ToSession: UpdateSessionTokensPayload.RefreshToken нужен вызывающим (dbservice,
// HTTPRepository) как одно и то же поле вне целой Session, и та же nil-vs-невалидная
// семантика важна для него ровно так же (см. комментарий у SessionDTO).
func StringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func NullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func TimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}

func NullTime(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}

// CreateSessionResponse — ответ /rpc/CreateSession: только присвоенный ID, остальные поля
// вызывающий уже знает (он их и отправил).
type CreateSessionResponse struct {
	ID uint64 `json:"id"`
}

// RefreshHashPayload — запрос для /rpc/FindSessionByRefreshHash и
// /rpc/RevokeSessionByRefreshHash: обоим достаточно одного и того же хеша.
type RefreshHashPayload struct {
	RefreshHash string `json:"refresh_hash"`
}

// UpdateSessionTokensPayload — запрос /rpc/UpdateSessionTokens, один в один с аргументами
// Store.UpdateSessionTokens.
type UpdateSessionTokensPayload struct {
	SessionID        uint64    `json:"session_id"`
	RefreshTokenHash string    `json:"refresh_token_hash"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     *string   `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}
