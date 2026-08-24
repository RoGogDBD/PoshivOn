package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/config"
	"github.com/RoGogDBD/PoshivOn/internal/handler"
	"github.com/RoGogDBD/PoshivOn/internal/repository"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// testMux собирает ровно ту таблицу маршрутов, которую поднимает main(). Хранилище сессий
// с нулевым соединением здесь допустимо: проверяется состав маршрутов и обвязка, а не
// работа хранилища, и до обращения к БД ни один из тестов ниже не доходит.
func testMux(t *testing.T, cfg *config.Config) *http.ServeMux {
	t.Helper()

	repo := repository.NewMemoryRepository()
	accessService := service.NewAccessService(repo, repo)
	authHandler := handler.NewAuthHandler(auth.NewStore(nil), cfg, accessService)
	apiHandler := handler.NewAPIHandler(service.NewCostingService(repo, repo, repo), nil)
	accessHandler := handler.NewAccessHandler(accessService, cfg.ContactEmail)

	return newMux(cfg, authHandler, apiHandler, accessHandler, accessService, handler.StoreSessionResolver(auth.NewStore(nil)))
}

func testConfig() *config.Config {
	return &config.Config{CookiePath: "/", CookieSameSite: "Lax", RefreshTTLHours: 720}
}

// TestNewMux_LegacyTokenRouteIsGone: POST /auth/yandex отдаёт 404 на настоящей таблице
// маршрутов (Decision 7).
//
// Проверка существует именно здесь, а не в handler-тестах: go build ловит только одно
// направление регрессии — обработчик удалён, а регистрация осталась (ошибка компиляции).
// Обратное — обработчик оставлен, а строку регистрации забыли убрать — собирается и
// проходит все остальные тесты, оставляя в проде ровно ту поверхность атаки, ради
// закрытия которой Decision 7 и существует.
func TestNewMux_LegacyTokenRouteIsGone(t *testing.T) {
	mux := testMux(t, testConfig())

	req := httptest.NewRequest(http.MethodPost, "http://api.example/auth/yandex", strings.NewReader(`{"access_token":"stolen"}`))
	if _, pattern := mux.Handler(req); pattern != "" {
		t.Errorf("маршрут /auth/yandex зарегистрирован как %q — приём готового OAuth-токена должен быть удалён", pattern)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /auth/yandex вернул %d, ожидался 404", rec.Code)
	}
}

// TestNewMux_ExpectedRoutesRegistered — обратная сторона предыдущего теста: удалять надо
// было одну строку, а не соседние.
func TestNewMux_ExpectedRoutesRegistered(t *testing.T) {
	mux := testMux(t, testConfig())

	cases := []struct {
		method string
		target string
	}{
		{http.MethodPost, "http://api.example/auth/yandex/code"},
		{http.MethodGet, "http://api.example/auth/status"},
		{http.MethodGet, "http://api.example/auth/me"},
		{http.MethodPost, "http://api.example/auth/refresh"},
		{http.MethodPost, "http://api.example/auth/logout"},
		{http.MethodGet, "http://api.example/health"},
		{http.MethodGet, "http://api.example/metrics"},
		{http.MethodGet, "http://api.example/api/v1/users/ivanov/chats"},
	}

	for _, testCase := range cases {
		t.Run(testCase.target, func(t *testing.T) {
			req := httptest.NewRequest(testCase.method, testCase.target, nil)
			if _, pattern := mux.Handler(req); pattern == "" {
				t.Errorf("маршрут %s не зарегистрирован", testCase.target)
			}
		})
	}
}

// TestNewMux_SameOriginGuardRejectsForeignOriginOnLogin: RequireSameOrigin действительно
// навешен на /auth/yandex/code в самой таблице маршрутов, а не просто «применим» к нему
// (Decision 8, login CSRF). Запрос идёт прямо в mux, без ручной обёртки в тесте: обёртка
// проверяла бы работоспособность функции, а не факт её подключения — а именно факт
// подключения Task 4 оставила несделанным.
func TestNewMux_SameOriginGuardRejectsForeignOriginOnLogin(t *testing.T) {
	mux := testMux(t, testConfig())

	req := httptest.NewRequest(http.MethodPost, "http://api.example/auth/yandex/code", strings.NewReader(`{"code":"attacker-code"}`))
	req.Host = "api.example"
	req.Header.Set("Origin", "http://evil.example")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", rec.Code)
	}
	if len(rec.Header().Values("Set-Cookie")) != 0 {
		t.Errorf("отклонённый запрос выставил куки: %v", rec.Header().Values("Set-Cookie"))
	}
}

// TestNewMux_ConfigValuesReachMiddleware: newMux — единственное место, где значения из
// config превращаются в зависимости middleware, и единственное, где ошибка проводки может
// остаться незамеченной. Тесты пакета handler собирают RouteDeps руками и этот шов не
// проходят вовсе: newMux, забывший передать cfg.CookieSecure, скомпилировался бы и прошёл
// их все, а на проде проверка Origin сверялась бы с http:// вместо https://.
//
// Обе величины проверяются через наблюдаемое поведение, а не сверкой полей: значение
// доехало ровно тогда, когда от него зависит ответ.
func TestNewMux_ConfigValuesReachMiddleware(t *testing.T) {
	t.Run("CookieSecure задаёт схему в same-origin fallback", func(t *testing.T) {
		cases := []struct {
			name         string
			cookieSecure bool
			origin       string
			wantStatus   int
		}{
			{name: "прод: https-origin совпадает с хостом", cookieSecure: true, origin: "https://api.example", wantStatus: http.StatusBadRequest},
			{name: "прод: http-origin того же хоста отклонён", cookieSecure: true, origin: "http://api.example", wantStatus: http.StatusForbidden},
			{name: "разработка: http-origin совпадает с хостом", cookieSecure: false, origin: "http://api.example", wantStatus: http.StatusBadRequest},
			{name: "разработка: https-origin того же хоста отклонён", cookieSecure: false, origin: "https://api.example", wantStatus: http.StatusForbidden},
		}

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				cfg := testConfig()
				cfg.CookieSecure = testCase.cookieSecure
				// CORS_ALLOWED_ORIGINS пуст — именно та прод-конфигурация, ради которой
				// Decision 8 ввёл сверку со scheme://r.Host вместо пропуска.
				mux := testMux(t, cfg)

				req := httptest.NewRequest(http.MethodPost, "http://api.example/auth/yandex/code", strings.NewReader(`{}`))
				req.Host = "api.example"
				req.Header.Set("Origin", testCase.origin)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				// 400 — ответ самого обработчика входа на тело без кода авторизации, то есть
				// проверка источника пройдена. Отличать «пропустил» от «отклонил» по коду
				// надёжнее, чем по факту вызова: до Яндекса такой запрос всё равно не дойдёт.
				if rec.Code != testCase.wantStatus {
					t.Fatalf("статус = %d, ожидался %d (тело: %s)", rec.Code, testCase.wantStatus, rec.Body.String())
				}
			})
		}
	})

	t.Run("AllowedOrigins разбирается из CSV и попадает в проверку", func(t *testing.T) {
		cfg := testConfig()
		cfg.CookieSecure = true
		// Пробелы и пустой элемент — ровно то, что splitCSV обязан вычистить; без вызова
		// splitCSV значение " https://panel.example" в список не попало бы.
		cfg.AllowedOrigins = "https://app.example, https://panel.example,"
		mux := testMux(t, cfg)

		cases := []struct {
			origin     string
			wantStatus int
		}{
			{origin: "https://app.example", wantStatus: http.StatusBadRequest},
			{origin: "https://panel.example", wantStatus: http.StatusBadRequest},
			{origin: "https://evil.example", wantStatus: http.StatusForbidden},
			// Список непуст, значит сверка идёт только с ним: сам хост API в него не входит.
			{origin: "https://api.example", wantStatus: http.StatusForbidden},
		}

		for _, testCase := range cases {
			t.Run(testCase.origin, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "https://api.example/auth/yandex/code", strings.NewReader(`{}`))
				req.Host = "api.example"
				req.Header.Set("Origin", testCase.origin)
				rec := httptest.NewRecorder()

				mux.ServeHTTP(rec, req)

				if rec.Code != testCase.wantStatus {
					t.Fatalf("статус = %d, ожидался %d (тело: %s)", rec.Code, testCase.wantStatus, rec.Body.String())
				}
			})
		}
	})
}

// TestNewMux_AccessChainsAreAttached: цепочки навешены на все префиксы таблицы Architecture
// в той сборке, которую поднимает main(), а не только в фикстуре handler-тестов.
func TestNewMux_AccessChainsAreAttached(t *testing.T) {
	mux := testMux(t, testConfig())

	cases := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{name: "users закрыт", method: http.MethodGet, target: "http://api.example/api/v1/users/ivanov/chats", wantStatus: http.StatusUnauthorized},
		{name: "admin закрыт", method: http.MethodGet, target: "http://api.example/api/v1/admin/users", wantStatus: http.StatusUnauthorized},
		{name: "access закрыт", method: http.MethodGet, target: "http://api.example/api/v1/access/me", wantStatus: http.StatusUnauthorized},
		{name: "health открыт", method: http.MethodGet, target: "http://api.example/health", wantStatus: http.StatusOK},
		{name: "metrics открыт", method: http.MethodGet, target: "http://api.example/metrics", wantStatus: http.StatusOK},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			req := httptest.NewRequest(testCase.method, testCase.target, nil)
			req.Host = "api.example"
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != testCase.wantStatus {
				t.Fatalf("статус = %d, ожидался %d", rec.Code, testCase.wantStatus)
			}
		})
	}
}

func TestBuildRepositories_ProvidesAccessRepositories(t *testing.T) {
	settingsRepo, chatRepo, calculationRepo, userRepo, accessRequestRepo, cleanup, err := buildRepositories(&config.Config{Storage: "memory"})
	if err != nil {
		t.Fatalf("buildRepositories: %v", err)
	}
	defer cleanup()

	if settingsRepo == nil || chatRepo == nil || calculationRepo == nil {
		t.Fatalf("существующая тройка репозиториев не собрана")
	}
	if userRepo == nil || accessRequestRepo == nil {
		t.Fatalf("репозитории доступа не собраны — AccessService оказался бы заглушкой")
	}
	if service.NewAccessService(userRepo, accessRequestRepo) == nil {
		t.Fatalf("AccessService не собирается из выданных репозиториев")
	}
}

func TestBuildRepositories_UnknownStorageFails(t *testing.T) {
	_, _, _, _, _, _, err := buildRepositories(&config.Config{Storage: "cassandra"})
	if err == nil {
		t.Fatalf("неподдерживаемое хранилище принято без ошибки")
	}
}

// TestBuildRepositories_HTTPStorage — APP_STORAGE=http собирает HTTPRepository без обращения
// к сети при самой сборке (это ленивый клиент, соединение открывается на первый вызов метода,
// не здесь) — проверяем только то, что ветка не паникует и отдаёт все пять интерфейсов.
func TestBuildRepositories_HTTPStorage(t *testing.T) {
	settingsRepo, chatRepo, calculationRepo, userRepo, accessRequestRepo, cleanup, err := buildRepositories(&config.Config{
		Storage:      "http",
		DBServiceURL: "https://db-service.example",
	})
	if err != nil {
		t.Fatalf("buildRepositories: %v", err)
	}
	defer cleanup()

	if settingsRepo == nil || chatRepo == nil || calculationRepo == nil {
		t.Fatalf("тройка репозиториев не собрана для APP_STORAGE=http")
	}
	if userRepo == nil || accessRequestRepo == nil {
		t.Fatalf("репозитории доступа не собраны для APP_STORAGE=http")
	}
	if service.NewAccessService(userRepo, accessRequestRepo) == nil {
		t.Fatalf("AccessService не собирается из репозиториев http storage")
	}

	// Проверка конкретного типа, а не только non-nil (test review): без неё диспетчерская
	// ошибка вида "http и postgres перепутаны местами" (или подмена на memory) прошла бы тест
	// незамеченной — MemoryRepository тоже реализует все пять интерфейсов и тоже не nil.
	if _, ok := settingsRepo.(*repository.HTTPRepository); !ok {
		t.Fatalf("APP_STORAGE=http собрал не HTTPRepository, а %T", settingsRepo)
	}
}
