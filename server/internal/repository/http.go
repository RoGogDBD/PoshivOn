package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// maxResponseBodyBytes ограничивает тело ответа db-service сверху — зеркало
// maxRequestBodyBytes на стороне сервера (dbservice/rpc.go), которое там введено ровно
// из-за исчерпания памяти на co-located инстансе. С этой стороны риск иной (db-service —
// доверенный вызываемый, не произвольный ввод), но неограниченное io.ReadAll всё равно
// читало бы в память ответ любого размера без всякого предохранителя (code review, security
// audit) — с запасом над крупнейшим легитимным ответом (ListChats/ListCalculations).
const maxResponseBodyBytes = 4 << 20 // 4 MiB

// HTTPRepository реализует все пять репозиторных интерфейсов бэкенда поверх db-service —
// HTTP-обёртки над MariaDB, вынесенной в отдельный Serverless Container (план миграции,
// раздел «Фаза 2»). Каждый метод — ровно один POST на /rpc/<Method> db-service
// (server/internal/dbservice/handlers.go, rpc.go) с теми же формами запроса/ответа: это
// клиент к уже реализованному серверу, не отдельно придуманный протокол.
type HTTPRepository struct {
	baseURL string
	client  *http.Client
	tokens  IAMTokenSource
}

func NewHTTPRepository(baseURL string, client *http.Client, tokens IAMTokenSource) *HTTPRepository {
	// TrimRight на случай значения с завершающим "/" (например, скопированного из консоли):
	// без этого путь строился бы как ".../rpc//rpc/Method" — ServeMux на сервере ответил бы
	// 301 на канонический путь, а http.Client следует за 301 на POST, превращая его в GET
	// без тела; вся сущность запроса терялась бы молча (code review).
	return &HTTPRepository{baseURL: strings.TrimRight(baseURL, "/"), client: client, tokens: tokens}
}

var _ service.UserSettingsRepository = (*HTTPRepository)(nil)
var _ service.ChatRepository = (*HTTPRepository)(nil)
var _ service.ChatCalculationRepository = (*HTTPRepository)(nil)
var _ service.UserRepository = (*HTTPRepository)(nil)
var _ service.AccessRequestRepository = (*HTTPRepository)(nil)

// rpcErrorSentinels отображает код ошибки db-service (dbservice.classifyError) обратно на
// доменные sentinel-ошибки сервисного слоя — зеркало в обратную сторону, коды те же самые
// четыре на всех пятнадцати эндпоинтах, держим отображение в одном месте.
var rpcErrorSentinels = map[string]error{
	"invalid_request": service.ErrInvalidArgument,
	"forbidden":       service.ErrForbidden,
	"not_found":       service.ErrNotFound,
	"conflict":        service.ErrConflict,
}

type rpcErrorBody struct {
	Error string `json:"error"`
}

// callRPC — общий вызов одного db-service эндпоинта: маршалит запрос, ставит токен
// авторизации, декодирует ответ или ошибку. Один generic-хелпер вместо пятнадцати почти
// одинаковых методов, тем же приёмом, что и dbservice.rpc на стороне сервера.
func callRPC[Req any, Resp any](ctx context.Context, r *HTTPRepository, method string, req Req) (Resp, error) {
	var zero Resp

	body, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("dbservice %s: encode request: %w", method, err)
	}

	token, err := r.tokens.Token(ctx)
	if err != nil {
		return zero, fmt.Errorf("dbservice %s: get iam token: %w", method, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/rpc/"+method, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("dbservice %s: build request: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return zero, fmt.Errorf("dbservice %s: request failed: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return zero, fmt.Errorf("dbservice %s: read response: %w", method, err)
	}
	if len(respBody) > maxResponseBodyBytes {
		return zero, fmt.Errorf("dbservice %s: response exceeds %d bytes", method, maxResponseBodyBytes)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody rpcErrorBody
		if jsonErr := json.Unmarshal(respBody, &errBody); jsonErr == nil && errBody.Error != "" {
			if sentinel, ok := rpcErrorSentinels[errBody.Error]; ok {
				return zero, fmt.Errorf("dbservice %s: %s: %w", method, errBody.Error, sentinel)
			}
			return zero, fmt.Errorf("dbservice %s: %s (status %d)", method, errBody.Error, resp.StatusCode)
		}
		return zero, fmt.Errorf("dbservice %s: status %d", method, resp.StatusCode)
	}

	var out Resp
	if err := json.Unmarshal(respBody, &out); err != nil {
		return zero, fmt.Errorf("dbservice %s: decode response: %w", method, err)
	}
	return out, nil
}

type emptyRPCRequest struct{}

type emptyRPCResponse struct{}

type itemsEnvelope[T any] struct {
	Items []T `json:"items"`
}

type loginPayload struct {
	Login string `json:"login"`
}

type userIDPayload struct {
	UserID string `json:"user_id"`
}

type ensureUserPayload struct {
	Login       string `json:"login"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type setAccessPayload struct {
	Login   string `json:"login"`
	Granted bool   `json:"granted"`
}

type decideRequestPayload struct {
	Login     string `json:"login"`
	Status    string `json:"status"`
	DecidedBy string `json:"decided_by"`
}

type upsertSettingsPayload struct {
	UserID   string               `json:"user_id"`
	Settings service.UserSettings `json:"settings"`
}

type deleteChatPayload struct {
	UserID    string `json:"user_id"`
	ChatID    string `json:"chat_id"`
	DeletedBy string `json:"deleted_by"`
	Hard      bool   `json:"hard"`
}

type chatRefPayload struct {
	UserID string `json:"user_id"`
	ChatID string `json:"chat_id"`
}

// --- UserRepository -----------------------------------------------------------------

func (r *HTTPRepository) EnsureUser(ctx context.Context, login, email, displayName string) error {
	_, err := callRPC[ensureUserPayload, emptyRPCResponse](ctx, r, "EnsureUser", ensureUserPayload{
		Login: login, Email: email, DisplayName: displayName,
	})
	return err
}

func (r *HTTPRepository) GetUser(ctx context.Context, login string) (service.UserRecord, error) {
	return callRPC[loginPayload, service.UserRecord](ctx, r, "GetUser", loginPayload{Login: login})
}

func (r *HTTPRepository) ListUsers(ctx context.Context) ([]service.UserRecord, error) {
	resp, err := callRPC[emptyRPCRequest, itemsEnvelope[service.UserRecord]](ctx, r, "ListUsers", emptyRPCRequest{})
	return resp.Items, err
}

func (r *HTTPRepository) SetAccess(ctx context.Context, login string, granted bool) error {
	_, err := callRPC[setAccessPayload, emptyRPCResponse](ctx, r, "SetAccess", setAccessPayload{
		Login: login, Granted: granted,
	})
	return err
}

// --- AccessRequestRepository ----------------------------------------------------------

func (r *HTTPRepository) CreateRequest(ctx context.Context, login string) error {
	_, err := callRPC[loginPayload, emptyRPCResponse](ctx, r, "CreateRequest", loginPayload{Login: login})
	return err
}

func (r *HTTPRepository) GetRequest(ctx context.Context, login string) (service.AccessRequest, error) {
	return callRPC[loginPayload, service.AccessRequest](ctx, r, "GetRequest", loginPayload{Login: login})
}

func (r *HTTPRepository) DecideRequest(ctx context.Context, login, status, decidedBy string) error {
	_, err := callRPC[decideRequestPayload, emptyRPCResponse](ctx, r, "DecideRequest", decideRequestPayload{
		Login: login, Status: status, DecidedBy: decidedBy,
	})
	return err
}

// --- UserSettingsRepository ------------------------------------------------------------

func (r *HTTPRepository) UpsertSettings(ctx context.Context, userID string, settings service.UserSettings) error {
	_, err := callRPC[upsertSettingsPayload, emptyRPCResponse](ctx, r, "UpsertSettings", upsertSettingsPayload{
		UserID: userID, Settings: settings,
	})
	return err
}

func (r *HTTPRepository) GetSettings(ctx context.Context, userID string) (service.UserSettings, error) {
	return callRPC[userIDPayload, service.UserSettings](ctx, r, "GetSettings", userIDPayload{UserID: userID})
}

// --- ChatRepository ----------------------------------------------------------------------

func (r *HTTPRepository) CreateChat(ctx context.Context, chat service.Chat) (service.Chat, error) {
	return callRPC[service.Chat, service.Chat](ctx, r, "CreateChat", chat)
}

func (r *HTTPRepository) ListChats(ctx context.Context, userID string) ([]service.Chat, error) {
	resp, err := callRPC[userIDPayload, itemsEnvelope[service.Chat]](ctx, r, "ListChats", userIDPayload{UserID: userID})
	return resp.Items, err
}

func (r *HTTPRepository) DeleteChat(ctx context.Context, userID, chatID, deletedBy string, hard bool) error {
	_, err := callRPC[deleteChatPayload, emptyRPCResponse](ctx, r, "DeleteChat", deleteChatPayload{
		UserID: userID, ChatID: chatID, DeletedBy: deletedBy, Hard: hard,
	})
	return err
}

func (r *HTTPRepository) RestoreChat(ctx context.Context, userID, chatID string) error {
	_, err := callRPC[chatRefPayload, emptyRPCResponse](ctx, r, "RestoreChat", chatRefPayload{
		UserID: userID, ChatID: chatID,
	})
	return err
}

// --- ChatCalculationRepository ----------------------------------------------------------

func (r *HTTPRepository) AppendCalculation(ctx context.Context, result service.CalculationResult) error {
	_, err := callRPC[service.CalculationResult, emptyRPCResponse](ctx, r, "AppendCalculation", result)
	return err
}

func (r *HTTPRepository) ListCalculations(ctx context.Context, userID, chatID string) ([]service.CalculationResult, error) {
	resp, err := callRPC[chatRefPayload, itemsEnvelope[service.CalculationResult]](ctx, r, "ListCalculations", chatRefPayload{
		UserID: userID, ChatID: chatID,
	})
	return resp.Items, err
}

// --- handler.SessionStore ------------------------------------------------------------
//
// Хранилище сессий входа — отдельный от пяти репозиториев выше путь (своя таблица
// oauth_sessions, свой прямой SQL-доступ через *auth.Store на стороне db-service), но при
// APP_STORAGE=http бэкенду прямой SQL недоступен вообще ни для чего, поэтому идёт через
// db-service тем же HTTPRepository (найдено на реальном деплое — без этого бэкенд падал на
// старте, пытаясь открыть подключение только ради сессий). Методы без context.Context —
// зеркало сигнатуры *auth.Store, у которой его никогда не было (сырой database/sql, не
// *Context-варианты); используем context.Background() для самого HTTP-вызова.
//
// Известный разрыв (code review): в отличие от остальных пятнадцати методов, отмену запроса
// браузером или дедлайн вызывающего сюда не протащить — эти четыре вызова ограничены только
// общим таймаутом http.Client (100с, cmd/main.go). Чинится только протаскиванием
// context.Context через handler.SessionStore/SessionFinder и сам *auth.Store — за рамками
// этой задачи (mirror существующего паттерна, не его расширение).

func (r *HTTPRepository) CreateSession(session *auth.Session) error {
	resp, err := callRPC[auth.SessionDTO, auth.CreateSessionResponse](context.Background(), r, "CreateSession", session.ToDTO())
	if err != nil {
		return err
	}
	session.ID = resp.ID
	return nil
}

func (r *HTTPRepository) FindByRefreshHash(refreshHash string) (*auth.Session, error) {
	dto, err := callRPC[auth.RefreshHashPayload, auth.SessionDTO](context.Background(), r, "FindSessionByRefreshHash", auth.RefreshHashPayload{RefreshHash: refreshHash})
	if err != nil {
		return nil, err
	}
	session := dto.ToSession()
	return &session, nil
}

func (r *HTTPRepository) UpdateSessionTokens(sessionID uint64, refreshHash string, accessToken string, refreshToken sql.NullString, accessExpiresAt time.Time, refreshExpiresAt time.Time) error {
	_, err := callRPC[auth.UpdateSessionTokensPayload, emptyRPCResponse](context.Background(), r, "UpdateSessionTokens", auth.UpdateSessionTokensPayload{
		SessionID:        sessionID,
		RefreshTokenHash: refreshHash,
		AccessToken:      accessToken,
		RefreshToken:     auth.StringPtr(refreshToken),
		AccessExpiresAt:  accessExpiresAt,
		RefreshExpiresAt: refreshExpiresAt,
	})
	return err
}

func (r *HTTPRepository) RevokeByRefreshHash(refreshHash string) error {
	_, err := callRPC[auth.RefreshHashPayload, emptyRPCResponse](context.Background(), r, "RevokeSessionByRefreshHash", auth.RefreshHashPayload{RefreshHash: refreshHash})
	return err
}
