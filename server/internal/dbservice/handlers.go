package dbservice

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// Deps — репозитории, которые db-service оборачивает как HTTP. В проде на всех пяти полях
// один и тот же *repository.PostgresRepository (та же реализация, что сегодня использует
// бэкенд напрямую, см. server/cmd/main.go, buildRepositories) — раздельные поля нужны
// только чтобы тесты могли подменить любую комбинацию, не реализуя все пятнадцать методов
// ради проверки одного эндпоинта.
type Deps struct {
	Users        service.UserRepository
	AccessReqs   service.AccessRequestRepository
	Settings     service.UserSettingsRepository
	Chats        service.ChatRepository
	Calculations service.ChatCalculationRepository
}

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
		deps.Chats == nil || deps.Calculations == nil {
		panic("dbservice.Routes: Deps содержит nil-поле — все пять репозиториев обязательны")
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
