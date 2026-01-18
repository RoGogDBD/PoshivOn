package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
)

const (
	accessCookieName  = "ya_access"
	refreshCookieName = "ya_refresh"
)

type AuthHandler struct {
	store      *auth.Store
	cfg        *config.Config
	httpClient *http.Client
}

func NewAuthHandler(store *auth.Store, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		store: store,
		cfg:   cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (h *AuthHandler) HandleYandexLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	payload, err := readJSON(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	accessToken := strings.TrimSpace(getString(payload, "access_token"))
	if accessToken == "" {
		writeError(w, http.StatusBadRequest, "access_token_required")
		return
	}

	expiresIn := parseExpiresIn(payload)
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	accessExpiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)

	refreshToken := generateToken()
	refreshHash := auth.HashRefreshToken(refreshToken)
	refreshTTL := time.Duration(h.cfg.RefreshTTLHours) * time.Hour
	refreshExpiresAt := time.Now().UTC().Add(refreshTTL)

	session := &auth.Session{
		RefreshTokenHash:  refreshHash,
		YandexAccessToken: accessToken,
		AccessExpiresAt:   accessExpiresAt,
		RefreshExpiresAt:  refreshExpiresAt,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if refreshFromYandex := strings.TrimSpace(getString(payload, "refresh_token")); refreshFromYandex != "" {
		session.YandexRefreshToken = sql.NullString{String: refreshFromYandex, Valid: true}
	}

	if err := h.store.CreateSession(session); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}

	h.setAccessCookie(w, accessToken, expiresIn)
	h.setRefreshCookie(w, refreshToken, int(refreshTTL.Seconds()))

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) HandleYandexCode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	payload, err := readJSON(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	code := strings.TrimSpace(getString(payload, "code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "code_required")
		return
	}

	redirectURI := strings.TrimSpace(getString(payload, "redirect_uri"))
	if redirectURI == "" {
		redirectURI = h.cfg.YandexRedirectURI
	}
	if redirectURI == "" {
		writeError(w, http.StatusBadRequest, "redirect_uri_required")
		return
	}

	accessToken, refreshToken, expiresIn, err := h.exchangeCodeWithYandex(r.Context(), code, redirectURI)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "yandex_exchange_failed")
		return
	}

	if expiresIn <= 0 {
		expiresIn = 3600
	}
	accessExpiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)

	refreshTokenCookie := generateToken()
	refreshHash := auth.HashRefreshToken(refreshTokenCookie)
	refreshTTL := time.Duration(h.cfg.RefreshTTLHours) * time.Hour
	refreshExpiresAt := time.Now().UTC().Add(refreshTTL)

	session := &auth.Session{
		RefreshTokenHash:  refreshHash,
		YandexAccessToken: accessToken,
		AccessExpiresAt:   accessExpiresAt,
		RefreshExpiresAt:  refreshExpiresAt,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if refreshToken != "" {
		session.YandexRefreshToken = sql.NullString{String: refreshToken, Valid: true}
	}

	if err := h.store.CreateSession(session); err != nil {
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}

	h.setAccessCookie(w, accessToken, expiresIn)
	h.setRefreshCookie(w, refreshTokenCookie, int(refreshTTL.Seconds()))

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	refreshToken, err := readCookie(r, refreshCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "refresh_cookie_missing")
		return
	}

	refreshHash := auth.HashRefreshToken(refreshToken)
	session, err := h.store.FindByRefreshHash(refreshHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "session_not_found")
		return
	}

	now := time.Now().UTC()
	if session.RevokedAt.Valid || session.RefreshExpiresAt.Before(now) {
		writeError(w, http.StatusUnauthorized, "session_expired")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	refreshToken, err := readCookie(r, refreshCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "refresh_cookie_missing")
		return
	}

	refreshHash := auth.HashRefreshToken(refreshToken)
	session, err := h.store.FindByRefreshHash(refreshHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "session_not_found")
		return
	}

	now := time.Now().UTC()
	if session.RevokedAt.Valid || session.RefreshExpiresAt.Before(now) {
		writeError(w, http.StatusUnauthorized, "session_expired")
		return
	}

	if !session.YandexRefreshToken.Valid {
		writeError(w, http.StatusConflict, "reauth_required")
		return
	}

	newAccessToken, newRefreshToken, expiresIn, err := h.refreshWithYandex(r.Context(), session.YandexRefreshToken.String)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "yandex_refresh_failed")
		return
	}

	if expiresIn <= 0 {
		expiresIn = 3600
	}

	accessExpiresAt := now.Add(time.Duration(expiresIn) * time.Second)
	rotatedRefresh := generateToken()
	rotatedRefreshHash := auth.HashRefreshToken(rotatedRefresh)
	refreshTTL := time.Duration(h.cfg.RefreshTTLHours) * time.Hour
	refreshExpiresAt := now.Add(refreshTTL)

	updatedYandexRefresh := session.YandexRefreshToken
	if newRefreshToken != "" {
		updatedYandexRefresh = sql.NullString{String: newRefreshToken, Valid: true}
	}

	if err := h.store.UpdateSessionTokens(
		session.ID,
		rotatedRefreshHash,
		newAccessToken,
		updatedYandexRefresh,
		accessExpiresAt,
		refreshExpiresAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "session_update_failed")
		return
	}

	h.setAccessCookie(w, newAccessToken, expiresIn)
	h.setRefreshCookie(w, rotatedRefresh, int(refreshTTL.Seconds()))

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	refreshToken, err := readCookie(r, refreshCookieName)
	if err == nil {
		refreshHash := auth.HashRefreshToken(refreshToken)
		_ = h.store.RevokeByRefreshHash(refreshHash)
	}

	h.clearCookie(w, accessCookieName)
	h.clearCookie(w, refreshCookieName)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) refreshWithYandex(ctx context.Context, refreshToken string) (string, string, int64, error) {
	if h.cfg.YandexClientID == "" || h.cfg.YandexClientSecret == "" {
		return "", "", 0, errors.New("missing yandex client credentials")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", h.cfg.YandexClientID)
	form.Set("client_secret", h.cfg.YandexClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.YandexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, errors.New("yandex refresh failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", 0, err
	}

	accessToken := strings.TrimSpace(getString(payload, "access_token"))
	if accessToken == "" {
		return "", "", 0, errors.New("no access token")
	}

	expiresIn := parseExpiresIn(payload)
	newRefreshToken := strings.TrimSpace(getString(payload, "refresh_token"))
	return accessToken, newRefreshToken, expiresIn, nil
}

func (h *AuthHandler) exchangeCodeWithYandex(ctx context.Context, code string, redirectURI string) (string, string, int64, error) {
	if h.cfg.YandexClientID == "" || h.cfg.YandexClientSecret == "" {
		return "", "", 0, errors.New("missing yandex client credentials")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", h.cfg.YandexClientID)
	form.Set("client_secret", h.cfg.YandexClientSecret)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.YandexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, errors.New("yandex exchange failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", 0, err
	}

	accessToken := strings.TrimSpace(getString(payload, "access_token"))
	if accessToken == "" {
		return "", "", 0, errors.New("no access token")
	}
	refreshToken := strings.TrimSpace(getString(payload, "refresh_token"))
	expiresIn := parseExpiresIn(payload)
	return accessToken, refreshToken, expiresIn, nil
}

func (h *AuthHandler) setAccessCookie(w http.ResponseWriter, value string, maxAgeSeconds int64) {
	h.writeCookie(w, accessCookieName, value, int(maxAgeSeconds), true)
}

func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, value string, maxAgeSeconds int) {
	h.writeCookie(w, refreshCookieName, value, maxAgeSeconds, true)
}

func (h *AuthHandler) writeCookie(w http.ResponseWriter, name, value string, maxAge int, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     h.cfg.CookiePath,
		Domain:   h.cfg.CookieDomain,
		HttpOnly: httpOnly,
		Secure:   h.cfg.CookieSecure,
		SameSite: parseSameSite(h.cfg.CookieSameSite),
		MaxAge:   maxAge,
	})
}

func (h *AuthHandler) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     h.cfg.CookiePath,
		Domain:   h.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: parseSameSite(h.cfg.CookieSameSite),
		MaxAge:   -1,
	})
}

func parseSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func readCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func readJSON(body io.ReadCloser) (map[string]interface{}, error) {
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseExpiresIn(payload map[string]interface{}) int64 {
	switch value := payload["expires_in"].(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return parsed
		}
	case float64:
		return int64(value)
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func getString(payload map[string]interface{}, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func generateToken() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": code,
	})
}
