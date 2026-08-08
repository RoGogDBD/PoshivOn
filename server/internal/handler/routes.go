package handler

import (
	"net/http"

	"github.com/RoGogDBD/PoshivOn/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RouteDeps — всё, из чего собирается таблица маршрутов. Структура, а не список аргументов:
// зависимостей семь, и позиционный вызов с двумя *http.Handler-подобными полями подряд
// слишком легко перепутать местами так, чтобы это ещё и скомпилировалось.
type RouteDeps struct {
	Auth          *AuthHandler
	API           *APIHandler
	Access        *AccessHandler
	AccessService *service.AccessService
	// ResolveSession — как узнать, чья это сессия. Зависимость, а не жёсткая ссылка на
	// auth.Store: в тестах хранилище недоступно (нужен живой *sql.DB), а маршруты должны
	// собираться той же функцией, что и в проде.
	ResolveSession SessionResolver
	// AllowedOrigins — уже разобранный список (splitCSV на стороне main.go), CookieSecure —
	// признак прод-схемы https. Обе величины приходят из конфигурации: угадывать схему по
	// запросу нельзя, r.TLS за прокси всегда nil, а X-Forwarded-Proto выставляет тот же,
	// кто присылает Origin.
	AllowedOrigins []string
	CookieSecure   bool
}

// BuildRoutes собирает маршруты вместе с их middleware-цепочками — по таблице «Маршруты и
// их обвязка» из Architecture:
//
//	/api/v1/users/  → RequireSameOrigin → RequireAuth → RequireAccess
//	/api/v1/admin/  → RequireSameOrigin → RequireAuth → RequireAdmin
//	/api/v1/access/ → RequireSameOrigin → RequireAuth
//	POST /auth/yandex/code, /auth/refresh, /auth/logout → RequireSameOrigin
//	GET /auth/status, /auth/me, /health, /metrics → без изменений
//
// Функция живёт в пакете handler, а не в main: тесты пакета handler должны вызывать ровно
// её, а не свою копию сборки — иначе они проверяли бы обвязку, которой в проде нет (ровно
// это и случилось в Task 4: middleware написаны, покрыты тестами и никуда не подключены).
// Обратное направление — держать функцию в main и импортировать её из handler — дало бы
// цикл импорта.
//
// Порядок внешних обёрток (CORS → Metrics → mux) остаётся в main() и здесь не участвует:
// Decision 18 опирается именно на то, что метрика снимается снаружи авторизации.
func BuildRoutes(deps RouteDeps) *http.ServeMux {
	mux := http.NewServeMux()

	// Каждая группа собирается на своём ServeMux и монтируется под префиксом уже обёрнутой.
	// http.ServeMux при делегировании не срезает путь, поэтому вложенный мультиплексор
	// матчит те же полные адреса, а разные префиксы получают разные цепочки — чего одна
	// общая обёртка поверх всего mux не позволяет.
	usersMux := http.NewServeMux()
	deps.API.Register(usersMux)
	mux.Handle("/api/v1/users/", deps.sameOrigin(
		RequireAuth(deps.ResolveSession,
			RequireAccess(deps.AccessService, usersMux))))

	adminMux := http.NewServeMux()
	deps.Access.RegisterAdmin(adminMux)
	mux.Handle("/api/v1/admin/", deps.sameOrigin(
		RequireAuth(deps.ResolveSession,
			RequireAdmin(deps.AccessService, adminMux))))

	// /api/v1/access/ намеренно без RequireAccess: сюда обращается именно тот, у кого
	// доступа ещё нет, — иначе заявку было бы невозможно подать.
	accessMux := http.NewServeMux()
	deps.Access.RegisterAccess(accessMux)
	mux.Handle("/api/v1/access/", deps.sameOrigin(
		RequireAuth(deps.ResolveSession, accessMux)))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Простейший healthcheck для проверки доступности сервиса.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", promhttp.Handler())

	// Маршрута /auth/yandex здесь нет намеренно: приём готового OAuth-токена от клиента
	// удалён вместе с обработчиком (Decision 7) — он создавал сессию, не проверяя, какому
	// приложению выдан токен, а из сессии теперь выводится роль администратора.
	//
	// RequireAuth на /auth/* не навешивается: эти маршруты работают до появления сессии
	// или вместо неё. RequireSameOrigin — навешивается на изменяющие: без него чужая
	// страница может POST-запросом подставить свой код авторизации в куки жертвы.
	mux.Handle("/auth/yandex/code", deps.sameOrigin(http.HandlerFunc(deps.Auth.HandleYandexCode)))
	mux.Handle("/auth/refresh", deps.sameOrigin(http.HandlerFunc(deps.Auth.HandleRefresh)))
	mux.Handle("/auth/logout", deps.sameOrigin(http.HandlerFunc(deps.Auth.HandleLogout)))
	// GET-маршруты состояния не меняют — проверка источника их не касается (Decision 8).
	mux.HandleFunc("/auth/status", deps.Auth.HandleStatus)
	mux.HandleFunc("/auth/me", deps.Auth.HandleMe)

	return mux
}

func (deps RouteDeps) sameOrigin(next http.Handler) http.Handler {
	return RequireSameOrigin(deps.AllowedOrigins, deps.CookieSecure, next)
}
