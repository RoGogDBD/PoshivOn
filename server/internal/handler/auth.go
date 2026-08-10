package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

const (
	accessCookieName  = "ya_access"
	refreshCookieName = "ya_refresh"
)

// SessionFinder — минимум, нужный проверке сессии. Отдельный от SessionStore интерфейс:
// middleware читает сессию и ничего в ней не меняет, и сужение прав здесь бесплатно.
type SessionFinder interface {
	FindByRefreshHash(refreshHash string) (*auth.Session, error)
}

// SessionStore — то, что обработчикам входа нужно от хранилища сессий. Интерфейс, а не
// *auth.Store, по двум причинам: обработчики не должны знать про SQL, и без этого шва
// handler-тесты требовали бы живой MariaDB, тогда как по замыслу они идут всегда.
// Прод передаёт сюда *auth.Store — сигнатура NewAuthHandler от этого не меняется.
type SessionStore interface {
	SessionFinder
	CreateSession(session *auth.Session) error
	UpdateSessionTokens(
		sessionID uint64,
		refreshHash string,
		accessToken string,
		refreshToken sql.NullString,
		accessExpiresAt time.Time,
		refreshExpiresAt time.Time,
	) error
	RevokeByRefreshHash(refreshHash string) error
}

var _ SessionStore = (*auth.Store)(nil)

// AuthHandler обслуживает /auth/*. accessService нужен ровно для одного: завести строку
// users при входе (Decision 11) — проверки прав живут в middleware, не здесь.
type AuthHandler struct {
	store         SessionStore
	cfg           *config.Config
	accessService *service.AccessService
	httpClient    *http.Client
}

func NewAuthHandler(store SessionStore, cfg *config.Config, accessService *service.AccessService) *AuthHandler {
	return &AuthHandler{
		store:         store,
		cfg:           cfg,
		accessService: accessService,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// authFailure — причина отказа, уже переведённая в ответ: статус и машиночитаемый слаг.
// Шаги входа возвращают её вместо того, чтобы писать в ResponseWriter самим, — так ответ
// формируется в одном месте, а шаги остаются проверяемыми по возвращаемому значению.
type authFailure struct {
	status int
	code   string
}

// yandexTokens — результат обмена кода. ExpiresIn здесь уже нормализован: значение уходит
// и в срок жизни сессии, и в MaxAge куки, и расходиться этим двум нельзя.
type yandexTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// HandleYandexCode — вход по коду авторизации. Функция только оркеструет шаги и пишет
// ответ; каждый шаг ниже отвечает за одну задачу.
func (h *AuthHandler) HandleYandexCode(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	code, redirectURI, failure := h.readLoginRequest(r)
	if failure != nil {
		writeError(w, failure.status, failure.code)
		return
	}

	tokens, profile, failure := h.establishIdentity(r.Context(), code, redirectURI)
	if failure != nil {
		writeError(w, failure.status, failure.code)
		return
	}

	session, refreshCookie, refreshTTL := h.newLoginSession(tokens, profile)
	if err := h.store.CreateSession(session); err != nil {
		log.Printf("вход: не удалось сохранить сессию: %v", err)
		writeError(w, http.StatusInternalServerError, "session_create_failed")
		return
	}

	h.issueSessionCookies(w, tokens.AccessToken, tokens.ExpiresIn, refreshCookie, refreshTTL)
	w.WriteHeader(http.StatusNoContent)
}

// readLoginRequest разбирает тело запроса на вход и подставляет redirect_uri из
// конфигурации, если клиент его не прислал.
func (h *AuthHandler) readLoginRequest(r *http.Request) (string, string, *authFailure) {
	payload, err := readJSON(r.Body)
	if err != nil {
		return "", "", &authFailure{http.StatusBadRequest, "invalid_json"}
	}

	code := strings.TrimSpace(getString(payload, "code"))
	if code == "" {
		return "", "", &authFailure{http.StatusBadRequest, "code_required"}
	}

	redirectURI := strings.TrimSpace(getString(payload, "redirect_uri"))
	if redirectURI == "" {
		redirectURI = h.cfg.YandexRedirectURI
	}
	if redirectURI == "" {
		return "", "", &authFailure{http.StatusBadRequest, "redirect_uri_required"}
	}

	return code, redirectURI, nil
}

// establishIdentity меняет код на токены, узнаёт, кто вошёл, и заводит ему строку users.
// Все три шага здесь, потому что у них общий инвариант: пока личность не установлена и не
// сохранена, сессии и кук быть не должно — отсюда возврат отказа, а не частичного успеха.
func (h *AuthHandler) establishIdentity(ctx context.Context, code, redirectURI string) (yandexTokens, *yandexProfile, *authFailure) {
	accessToken, refreshToken, expiresIn, err := h.exchangeCodeWithYandex(ctx, code, redirectURI)
	if err != nil {
		log.Printf("вход: обмен кода не удался: %v", err)
		return yandexTokens{}, nil, &authFailure{http.StatusUnauthorized, "yandex_exchange_failed"}
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	tokens := yandexTokens{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: expiresIn}

	// Единственный за весь вход поход за профилем (Decision 1): дальше личность живёт
	// в строке сессии и на горячем пути к Яндексу никто не ходит.
	profile, err := h.fetchYandexProfile(ctx, accessToken)
	if err != nil {
		log.Printf("вход: не удалось получить профиль Яндекса: %v", err)
		return yandexTokens{}, nil, &authFailure{http.StatusBadGateway, "yandex_profile_failed"}
	}
	// Сессия без логина неотличима от домиграционной и была бы отвергнута первым же
	// RequireAuth (Decision 2). Проще не заводить её вовсе, чем выдать куки, с которыми
	// пользователь не сможет сделать ничего.
	if profile.Login == "" {
		log.Printf("вход: профиль Яндекса не содержит логина")
		return yandexTokens{}, nil, &authFailure{http.StatusBadGateway, "yandex_profile_failed"}
	}

	// Строка users заводится до сессии и до кук: пользователь, вошедший в систему, где его
	// нет, не смог бы ни подать заявку, ни попасть в админский список (Decision 11).
	if h.accessService == nil {
		log.Printf("вход: AccessService не сконфигурирован")
		return yandexTokens{}, nil, &authFailure{http.StatusInternalServerError, "user_ensure_failed"}
	}
	if err := h.accessService.EnsureUser(ctx, profile.Login, profile.Email, profile.Name); err != nil {
		log.Printf("вход: не удалось завести пользователя: %v", err)
		return yandexTokens{}, nil, &authFailure{http.StatusInternalServerError, "user_ensure_failed"}
	}

	return tokens, profile, nil
}

// newLoginSession собирает строку сессии по установленной личности и выдаёт значение
// refresh-куки вместе со сроком её жизни. Возвращает именно значение куки, а не хеш:
// в БД уходит хеш, клиенту — исходный токен, и путать их нельзя.
func (h *AuthHandler) newLoginSession(tokens yandexTokens, profile *yandexProfile) (*auth.Session, string, time.Duration) {
	now := time.Now().UTC()
	refreshCookie := generateToken()
	refreshTTL := time.Duration(h.cfg.RefreshTTLHours) * time.Hour

	session := &auth.Session{
		RefreshTokenHash:  auth.HashRefreshToken(refreshCookie),
		YandexAccessToken: tokens.AccessToken,
		AccessExpiresAt:   now.Add(time.Duration(tokens.ExpiresIn) * time.Second),
		RefreshExpiresAt:  now.Add(refreshTTL),
		CreatedAt:         now,
		UpdatedAt:         now,
		YandexLogin:       sql.NullString{String: profile.Login, Valid: true},
	}

	// Пустые email и имя — не ошибка входа (default_email у Яндекса необязателен):
	// в колонку уходит NULL, а не пустая строка, чтобы «нет значения» и «пустое значение»
	// не смешивались.
	session.YandexEmail = nullableString(profile.Email)
	session.YandexDisplayName = nullableString(profile.Name)
	session.YandexRefreshToken = nullableString(tokens.RefreshToken)

	return session, refreshCookie, refreshTTL
}

// issueSessionCookies выставляет пару кук сессии. Общая для входа и для ротации: разойтись
// в атрибутах этим двум местам нельзя, иначе после рефреша куки станут другими.
func (h *AuthHandler) issueSessionCookies(w http.ResponseWriter, accessToken string, expiresIn int64, refreshCookie string, refreshTTL time.Duration) {
	h.setAccessCookie(w, accessToken, expiresIn)
	h.setRefreshCookie(w, refreshCookie, int(refreshTTL.Seconds()))
}

// nullableString: пустая строка — это NULL в колонке, а не пустое значение.
func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func (h *AuthHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, err := requireIdentifiedSession(h.store, r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	session, err := requireIdentifiedSession(h.store, r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	profile, err := h.fetchYandexProfile(r.Context(), session.YandexAccessToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "yandex_profile_failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

func (h *AuthHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	session, failure := h.sessionForRefresh(r)
	if failure != nil {
		writeError(w, failure.status, failure.code)
		return
	}

	newAccessToken, newRefreshToken, expiresIn, err := h.refreshWithYandex(r.Context(), session.YandexRefreshToken.String)
	if err != nil {
		log.Printf("ротация: обновление токена у Яндекса не удалось: %v", err)
		writeError(w, http.StatusUnauthorized, "yandex_refresh_failed")
		return
	}
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	rotatedRefresh, refreshTTL, err := h.rotateSessionTokens(session, newAccessToken, newRefreshToken, expiresIn)
	if err != nil {
		log.Printf("ротация: не удалось обновить сессию: %v", err)
		writeError(w, http.StatusInternalServerError, "session_update_failed")
		return
	}

	h.issueSessionCookies(w, newAccessToken, expiresIn, rotatedRefresh, refreshTTL)
	w.WriteHeader(http.StatusNoContent)
}

// rotateSessionTokens записывает вращённые токены и возвращает значение новой refresh-куки
// вместе со сроком её жизни.
//
// За профилем здесь никто не ходит: личность уже лежит в строке сессии и переживает ротацию
// сама — UpdateSessionTokens не трогает её колонки (Decision 1). Повторный запрос к Яндексу
// на каждом рефреше вернул бы ровно ту зависимость, ради устранения которой личность и
// переехала в БД.
func (h *AuthHandler) rotateSessionTokens(session *auth.Session, accessToken, newRefreshToken string, expiresIn int64) (string, time.Duration, error) {
	now := time.Now().UTC()
	rotatedRefresh := generateToken()
	refreshTTL := time.Duration(h.cfg.RefreshTTLHours) * time.Hour

	// Яндекс не всегда присылает новый refresh-токен — тогда остаётся прежний, иначе
	// следующая ротация станет невозможной.
	updatedYandexRefresh := session.YandexRefreshToken
	if newRefreshToken != "" {
		updatedYandexRefresh = sql.NullString{String: newRefreshToken, Valid: true}
	}

	err := h.store.UpdateSessionTokens(
		session.ID,
		auth.HashRefreshToken(rotatedRefresh),
		accessToken,
		updatedYandexRefresh,
		now.Add(time.Duration(expiresIn)*time.Second),
		now.Add(refreshTTL),
	)
	return rotatedRefresh, refreshTTL, err
}

// sessionForRefresh находит сессию, которую вообще можно вращать. Отдельно от самой
// ротации: причин отказа четыре, и в одной функции с обновлением токенов они тонут.
func (h *AuthHandler) sessionForRefresh(r *http.Request) (*auth.Session, *authFailure) {
	refreshToken, err := readCookie(r, refreshCookieName)
	if err != nil {
		return nil, &authFailure{http.StatusUnauthorized, errRefreshCookieMissing.Error()}
	}

	session, err := h.store.FindByRefreshHash(auth.HashRefreshToken(refreshToken))
	if err != nil || session == nil {
		return nil, &authFailure{http.StatusUnauthorized, errSessionNotFound.Error()}
	}

	if session.RevokedAt.Valid || session.RefreshExpiresAt.Before(time.Now().UTC()) {
		return nil, &authFailure{http.StatusUnauthorized, errSessionExpired.Error()}
	}

	// Сессия, заведённая без refresh-токена Яндекса, вращаться не может — только заново
	// пройти вход.
	if !session.YandexRefreshToken.Valid {
		return nil, &authFailure{http.StatusConflict, "reauth_required"}
	}

	return session, nil
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

// Причины отказа проверки сессии. Значения — контракт с клиентом: shouldRefreshAuth
// (client/src/utils/yandexAuth.js:14-16) матчит access_cookie_missing/access_expired/
// access_mismatch по точному тексту и на них запускает повтор через /auth/refresh.
// Переименование любого из них молча ломает retry-логику клиента.
var (
	errAccessCookieMissing  = errors.New("access_cookie_missing")
	errRefreshCookieMissing = errors.New("refresh_cookie_missing")
	errSessionNotFound      = errors.New("session_not_found")
	errSessionExpired       = errors.New("session_expired")
	errAccessExpired        = errors.New("access_expired")
	errAccessMismatch       = errors.New("access_mismatch")
)

// SessionResolver отвечает на вопрос «чья это сессия» по одному запросу. Тип нужен, чтобы
// RequireAuth не зависел ни от AuthHandler, ни от конкретного хранилища.
type SessionResolver func(r *http.Request) (*auth.Session, error)

// StoreSessionResolver — прод-реализация резолвера поверх хранилища сессий.
func StoreSessionResolver(finder SessionFinder) SessionResolver {
	return func(r *http.Request) (*auth.Session, error) {
		return ResolveSession(finder, r)
	}
}

// ResolveSession — бывший (*AuthHandler).validateSession, вынесенный в функцию: ровно те же
// проверки в том же порядке, но вызывать её может и middleware, у которого экземпляра
// AuthHandler нет и быть не должно.
func ResolveSession(finder SessionFinder, r *http.Request) (*auth.Session, error) {
	accessToken, err := readCookie(r, accessCookieName)
	if err != nil {
		return nil, errAccessCookieMissing
	}

	refreshToken, err := readCookie(r, refreshCookieName)
	if err != nil {
		return nil, errRefreshCookieMissing
	}

	refreshHash := auth.HashRefreshToken(refreshToken)
	session, err := finder.FindByRefreshHash(refreshHash)
	if err != nil || session == nil {
		return nil, errSessionNotFound
	}

	now := time.Now().UTC()
	if session.RevokedAt.Valid || session.RefreshExpiresAt.Before(now) {
		return nil, errSessionExpired
	}

	if session.AccessExpiresAt.Before(now) {
		return nil, errAccessExpired
	}

	if session.YandexAccessToken != accessToken {
		return nil, errAccessMismatch
	}

	return session, nil
}

// requireIdentifiedSession — ResolveSession с дополнительной проверкой личности: сессия
// валидна (куки, хеш, срок), но строка создана до миграции 004 и yandex_login в ней ещё
// NULL (Decision 2). /auth/status и /auth/me звали голый ResolveSession и поэтому считали
// такую сессию «залогинен», хотя RequireAuth на /api/v1/** её уже отклоняет как
// session_identity_missing, — рассинхрон двух источников «авторизован» гонял пользователя
// со старой сессией по кругу: лендинг (App.jsx) видел /auth/status=200, редиректил на
// /panel, там GET /api/v1/access/me отклонялся, бутстрап-эффект уводил обратно на /, и
// лендинг снова видел /auth/status=200 — быстрый цикл редиректов вместо однократной
// отправки на вход, которую сам слаг session_identity_missing и предполагает (см.
// комментарий у sessionIdentityMissingCode в middleware.go).
func requireIdentifiedSession(store SessionFinder, r *http.Request) (*auth.Session, error) {
	session, err := ResolveSession(store, r)
	if err != nil {
		return nil, err
	}
	if !session.YandexLogin.Valid || strings.TrimSpace(session.YandexLogin.String) == "" {
		return nil, errors.New(sessionIdentityMissingCode)
	}
	return session, nil
}

// yandexProfile — то, что берётся из ответа login.yandex.ru/info. Email добавлен ради
// колонки users.email и письма администраторам в Phase 2 (Decision 12).
type yandexProfile struct {
	Name  string `json:"name"`
	Login string `json:"login"`
	Email string `json:"email"`
}

func (h *AuthHandler) fetchYandexProfile(ctx context.Context, accessToken string) (*yandexProfile, error) {
	if h.cfg.YandexUserInfoURL == "" {
		return nil, errors.New("user_info_url_missing")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.YandexUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("user_info_failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	return parseYandexProfile(payload), nil
}

// parseYandexProfile достаёт из ответа Яндекса то, что нужно нам. Отдельно от похода по
// сети: разбор — чистая функция, и проверять его без поднятого сервера дешевле.
func parseYandexProfile(payload map[string]interface{}) *yandexProfile {
	login := strings.TrimSpace(getString(payload, "login"))
	displayName := strings.TrimSpace(getString(payload, "display_name"))
	realName := strings.TrimSpace(getString(payload, "real_name"))
	firstName := strings.TrimSpace(getString(payload, "first_name"))
	lastName := strings.TrimSpace(getString(payload, "last_name"))
	// default_email извлекается тем же паттерном, что login и name, и так же необязателен:
	// его отсутствие — не ошибка входа (Decision 12).
	email := strings.TrimSpace(getString(payload, "default_email"))

	// Имя собирается по убыванию точности: как человек себя назвал, затем настоящее имя,
	// затем имя с фамилией, и только в крайнем случае логин.
	name := displayName
	if name == "" {
		name = realName
	}
	if name == "" {
		name = strings.TrimSpace(strings.Join([]string{firstName, lastName}, " "))
	}
	if name == "" {
		name = login
	}

	return &yandexProfile{
		Name:  name,
		Login: login,
		Email: email,
	}
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

// Ошибка отброшена намеренно, не по недосмотру (Task 10 code-audit отметил это как
// критическое расхождение с `costing.go:newChatID`, который ошибку проверяет; Task 11
// security-audit независимо проверил и подтвердил обратное — см. ниже). На Go 1.24+
// `crypto/rand.Read` документированно никогда не возвращает ошибку: при сбое источника
// энтропии пакет сам останавливает процесс (`go doc crypto/rand.Read`), а не возвращает
// частично заполненный/нулевой срез. Проверка `if err != nil` здесь была бы недостижимой
// веткой, которую ничто не может покрыть тестом. Секрет сессии не может тихо стать 64
// нулями — либо получится настоящий случайный токен, либо процесс упадёт раньше, чем
// вернёт что угодно.
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
