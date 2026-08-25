package dbservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// Deps — репозитории, которые db-service оборачивает как HTTP. В проде на всех пяти полях
// UserRepository..Calculations — один и тот же *repository.PostgresRepository (та же
// реализация, что сегодня использует бэкенд напрямую, см. server/cmd/main.go,
// buildRepositories) — раздельные поля нужны только чтобы тесты могли подменить любую
// комбинацию, не реализуя все методы ради проверки одного эндпоинта.
//
// Sessions — отдельный путь: хранилище сессий входа (*auth.Store) исторически не входит в
// пятёрку репозиториев сервисного слоя, у него свой прямой SQL-доступ (server/internal/
// auth/store.go). При APP_STORAGE=http бэкенду прямой SQL недоступен вообще ни для чего —
// поэтому сессии идут через db-service тем же способом, что и остальные данные, а не
// остаются забытым исключением (найдено на реальном деплое: бэкенд падал на старте, пытаясь
// открыть прямое подключение к БД только ради сессий).
type Deps struct {
	Users        service.UserRepository
	AccessReqs   service.AccessRequestRepository
	Settings     service.UserSettingsRepository
	Chats        service.ChatRepository
	Calculations service.ChatCalculationRepository
	Sessions     SessionRepository
}

// SessionRepository — набор методов, идентичный handler.SessionStore на стороне бэкенда
// (server/internal/handler/auth.go), определён здесь заново, а не импортирован: db-service
// не должен зависеть от browser-facing пакета handler. *auth.Store уже реализует его один в
// один — см. var _ ниже.
type SessionRepository interface {
	CreateSession(session *auth.Session) error
	FindByRefreshHash(refreshHash string) (*auth.Session, error)
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

var _ SessionRepository = (*auth.Store)(nil)

// Routes собирает HTTP-маршруты db-service: один POST-эндпоинт на метод репозитория, путь
// /rpc/<MethodName>. Не REST-иерархия ресурсов, а RPC-по-HTTP — единственный вызывающий это
// сам бэкенд (см. IAM-ограничение serverless-containers.containerInvoker в плане миграции,
// раздел «Фаза 2»); придумывать вложенные пути ради клиентов, которых не будет, — не то,
// на что стоит тратить площадь дизайна.
//
// ГРАНИЦА ДОВЕРИЯ (важно для любого, кто добавляет вызывающего): это транспорт над сырым
// репозиторным слоем, не над AccessService/CostingService — проверки прав (RequireAdmin),
// валидация бизнес-правил и генерация ID/дефолтов, которые в browser-facing бэкенде живут
// в сервисном слое (server/internal/service/access.go, costing.go), здесь НЕ повторяются
// (кроме точечных проверок непустоты полей атрибуции ниже — они дёшевы и закрывают
// конкретный сценарий "пустой decided_by/deleted_by"). Единственная защита от вызова кем
// попало — IAM-биндинг на платформе, ограничивающий invoker ровно сервисным аккаунтом
// бэкенда. Вызывать db-service в обход сервисного слоя бэкенда (напрямую из скрипта,
// нового интеграционного пути и т.п.) — значит терять эти проверки молча.
func Routes(deps Deps) *http.ServeMux {
	if deps.Users == nil || deps.AccessReqs == nil || deps.Settings == nil ||
		deps.Chats == nil || deps.Calculations == nil || deps.Sessions == nil {
		panic("dbservice.Routes: Deps содержит nil-поле — все шесть зависимостей обязательны")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handleHealth)

	// --- UserRepository ---
	mux.HandleFunc("POST /rpc/EnsureUser", rpc(func(ctx context.Context, req ensureUserRequest) (emptyResponse, error) {
		return emptyResponse{}, deps.Users.EnsureUser(ctx, req.Login, req.Email, req.DisplayName)
	}))
	mux.HandleFunc("POST /rpc/GetUser", rpc(func(ctx context.Context, req loginRequest) (service.UserRecord, error) {
		return deps.Users.GetUser(ctx, req.Login)
	}))
	mux.HandleFunc("POST /rpc/ListUsers", rpc(func(ctx context.Context, _ emptyRequest) (itemsResponse[service.UserRecord], error) {
		items, err := deps.Users.ListUsers(ctx)
		return itemsResponse[service.UserRecord]{Items: items}, err
	}))
	mux.HandleFunc("POST /rpc/SetAccess", audited("SetAccess", rpc(func(ctx context.Context, req setAccessRequest) (emptyResponse, error) {
		return emptyResponse{}, deps.Users.SetAccess(ctx, req.Login, req.Granted)
	})))

	// --- AccessRequestRepository ---
	mux.HandleFunc("POST /rpc/CreateRequest", rpc(func(ctx context.Context, req loginRequest) (emptyResponse, error) {
		return emptyResponse{}, deps.AccessReqs.CreateRequest(ctx, req.Login)
	}))
	mux.HandleFunc("POST /rpc/GetRequest", rpc(func(ctx context.Context, req loginRequest) (service.AccessRequest, error) {
		return deps.AccessReqs.GetRequest(ctx, req.Login)
	}))
	mux.HandleFunc("POST /rpc/DecideRequest", audited("DecideRequest", rpc(func(ctx context.Context, req decideRequestRequest) (emptyResponse, error) {
		// decided_by — единственный след того, кто решил заявку (тот же принцип, что и в
		// AccessService.SetAccess, service/access.go:187-189, которую эта RPC обходит —
		// репозиторный уровень сам по себе такой проверки не делает). Пустое значение
		// здесь не "неизвестный автор", а потерянная атрибуция, поэтому явный отказ, а не
		// молчаливая запись NULL/пустой строки.
		if strings.TrimSpace(req.DecidedBy) == "" {
			return emptyResponse{}, fmt.Errorf("decided_by is required: %w", service.ErrInvalidArgument)
		}
		return emptyResponse{}, deps.AccessReqs.DecideRequest(ctx, req.Login, req.Status, req.DecidedBy)
	})))

	// --- UserSettingsRepository ---
	mux.HandleFunc("POST /rpc/UpsertSettings", rpc(func(ctx context.Context, req upsertSettingsRequest) (emptyResponse, error) {
		return emptyResponse{}, deps.Settings.UpsertSettings(ctx, req.UserID, req.Settings)
	}))
	mux.HandleFunc("POST /rpc/GetSettings", rpc(func(ctx context.Context, req userIDRequest) (service.UserSettings, error) {
		return deps.Settings.GetSettings(ctx, req.UserID)
	}))

	// --- ChatRepository ---
	// Запрос — это сам service.Chat целиком (уже несёт JSON-теги, отдельный DTO не добавляет
	// ничего, кроме лишнего маппинга полей один в один).
	mux.HandleFunc("POST /rpc/CreateChat", rpc(func(ctx context.Context, req service.Chat) (service.Chat, error) {
		return deps.Chats.CreateChat(ctx, req)
	}))
	mux.HandleFunc("POST /rpc/ListChats", rpc(func(ctx context.Context, req userIDRequest) (itemsResponse[service.Chat], error) {
		items, err := deps.Chats.ListChats(ctx, req.UserID)
		return itemsResponse[service.Chat]{Items: items}, err
	}))
	mux.HandleFunc("POST /rpc/DeleteChat", rpc(func(ctx context.Context, req deleteChatRequest) (emptyResponse, error) {
		// deleted_by — тот же принцип атрибуции, что и decided_by у DecideRequest выше;
		// в текущем единственном вызывающем (CostingService.DeleteChat) всегда равен
		// user_id, но это инвариант вызывающего кода, а не то, что репозиторный уровень
		// проверяет сам — закрываем здесь, а не полагаемся на то, что он не нарушится.
		if strings.TrimSpace(req.DeletedBy) == "" {
			return emptyResponse{}, fmt.Errorf("deleted_by is required: %w", service.ErrInvalidArgument)
		}
		return emptyResponse{}, deps.Chats.DeleteChat(ctx, req.UserID, req.ChatID, req.DeletedBy, req.Hard)
	}))
	mux.HandleFunc("POST /rpc/RestoreChat", rpc(func(ctx context.Context, req chatRefRequest) (emptyResponse, error) {
		return emptyResponse{}, deps.Chats.RestoreChat(ctx, req.UserID, req.ChatID)
	}))

	// --- ChatCalculationRepository ---
	mux.HandleFunc("POST /rpc/AppendCalculation", rpc(func(ctx context.Context, req service.CalculationResult) (emptyResponse, error) {
		return emptyResponse{}, deps.Calculations.AppendCalculation(ctx, req)
	}))
	mux.HandleFunc("POST /rpc/ListCalculations", rpc(func(ctx context.Context, req chatRefRequest) (itemsResponse[service.CalculationResult], error) {
		items, err := deps.Calculations.ListCalculations(ctx, req.UserID, req.ChatID)
		return itemsResponse[service.CalculationResult]{Items: items}, err
	}))

	// --- SessionRepository --- методы *auth.Store не принимают context.Context (сырой
	// database/sql, не переведён на *Context-варианты) — ctx в замыканиях ниже поэтому не
	// используется, это не упущение.
	//
	// CreateSession и RevokeSessionByRefreshHash — под audited(), тем же принципом, что и
	// SetAccess/DecideRequest выше: выдача и отзыв сессии входа — такая же security-sensitive
	// мутация, и без сетевого следа об успехе расследование инцидента (спорный логаут,
	// подозрительные повторные входы) опиралось бы только на содержимое oauth_sessions,
	// без операционного контекста (security audit). FindSessionByRefreshHash/
	// UpdateSessionTokens намеренно не аудируются — они срабатывают на каждый
	// авторизованный запрос/рефреш, и лог захлебнулся бы шумом (тот же довод, что уже
	// объясняет отсутствие audited() у ListChats/GetSettings).
	mux.HandleFunc("POST /rpc/CreateSession", audited("CreateSession", rpc(func(_ context.Context, req auth.SessionDTO) (auth.CreateSessionResponse, error) {
		session := req.ToSession()
		if err := deps.Sessions.CreateSession(&session); err != nil {
			return auth.CreateSessionResponse{}, err
		}
		return auth.CreateSessionResponse{ID: session.ID}, nil
	})))
	mux.HandleFunc("POST /rpc/FindSessionByRefreshHash", rpc(func(_ context.Context, req auth.RefreshHashPayload) (auth.SessionDTO, error) {
		session, err := deps.Sessions.FindByRefreshHash(req.RefreshHash)
		if err != nil {
			// Различаем "точно нет такой строки" (sql.ErrNoRows от *auth.Store.
			// FindByRefreshHash — единственное, что оно возвращает на пустой результат) от
			// любой другой ошибки: раньше здесь любая ошибка молча становилась 404,
			// маскируя реальный сбой БД под "сессии нет" — сразу у всех залогиненных
			// пользователей разом при обычном сетевом сбое, да ещё без исходного текста
			// ошибки в логе (code review — тот же принцип уже соблюдён в
			// repository/postgres.go для GetUser/GetRequest/GetSettings, здесь просто не
			// был перенесён при первой версии).
			if errors.Is(err, sql.ErrNoRows) {
				return auth.SessionDTO{}, fmt.Errorf("session not found: %w", service.ErrNotFound)
			}
			return auth.SessionDTO{}, err
		}
		return session.ToDTO(), nil
	}))
	mux.HandleFunc("POST /rpc/UpdateSessionTokens", rpc(func(_ context.Context, req auth.UpdateSessionTokensPayload) (emptyResponse, error) {
		return emptyResponse{}, deps.Sessions.UpdateSessionTokens(
			req.SessionID, req.RefreshTokenHash, req.AccessToken, auth.NullString(req.RefreshToken),
			req.AccessExpiresAt, req.RefreshExpiresAt,
		)
	}))
	mux.HandleFunc("POST /rpc/RevokeSessionByRefreshHash", audited("RevokeSessionByRefreshHash", rpc(func(_ context.Context, req auth.RefreshHashPayload) (emptyResponse, error) {
		return emptyResponse{}, deps.Sessions.RevokeByRefreshHash(req.RefreshHash)
	})))

	return mux
}

// handleHealth не бьёт по БД намеренно: db-service либо жив и слушает HTTP (тогда 200
// осмысленно вернуть), либо процесс вообще не отвечает — второе host отличает сам, без
// содержимого ответа. Проверка реального SQL-подключения (миграции применились, соединение
// живо) — задача go-live чек-листа, а не health-эндпоинта, который дергают на каждый холодный
// старт.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type emptyRequest struct{}

type emptyResponse struct{}

type itemsResponse[T any] struct {
	Items []T `json:"items"`
}

type loginRequest struct {
	Login string `json:"login"`
}

type userIDRequest struct {
	UserID string `json:"user_id"`
}

type ensureUserRequest struct {
	Login       string `json:"login"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type setAccessRequest struct {
	Login   string `json:"login"`
	Granted bool   `json:"granted"`
}

type decideRequestRequest struct {
	Login     string `json:"login"`
	Status    string `json:"status"`
	DecidedBy string `json:"decided_by"`
}

type upsertSettingsRequest struct {
	UserID   string               `json:"user_id"`
	Settings service.UserSettings `json:"settings"`
}

type deleteChatRequest struct {
	UserID    string `json:"user_id"`
	ChatID    string `json:"chat_id"`
	DeletedBy string `json:"deleted_by"`
	Hard      bool   `json:"hard"`
}

type chatRefRequest struct {
	UserID string `json:"user_id"`
	ChatID string `json:"chat_id"`
}
