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
// с нулевым соединением здесь допустимо: проверяется состав маршрутов, а не их работа,
// и до обращения к БД ни один из тестов ниже не доходит.
func testMux(t *testing.T) *http.ServeMux {
	t.Helper()

	repo := repository.NewMemoryRepository()
	cfg := &config.Config{CookiePath: "/", CookieSameSite: "Lax", RefreshTTLHours: 720}
	authHandler := handler.NewAuthHandler(auth.NewStore(nil), cfg, service.NewAccessService(repo, repo))
	apiHandler := handler.NewAPIHandler(nil, nil)

	return newMux(authHandler, apiHandler)
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
	mux := testMux(t)

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
	mux := testMux(t)

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

// TestNewMux_SameOriginGuardRejectsForeignOriginOnLogin: RequireSameOrigin, навешенный на
// настоящую регистрацию /auth/yandex/code, отклоняет запрос с чужого origin до обращения
// к Яндексу (Decision 8, login CSRF).
//
// Сама сборка цепочек middleware на маршруты — задача 5/6, поэтому здесь проверяется
// пригодность: обработчик, взятый из таблицы маршрутов, корректно работает под защитой.
// Когда цепочки появятся в main(), этот тест начнёт проверять и сам факт их навешивания.
func TestNewMux_SameOriginGuardRejectsForeignOriginOnLogin(t *testing.T) {
	mux := testMux(t)

	req := httptest.NewRequest(http.MethodPost, "http://api.example/auth/yandex/code", strings.NewReader(`{"code":"attacker-code"}`))
	req.Host = "api.example"
	req.Header.Set("Origin", "http://evil.example")

	route, pattern := mux.Handler(req)
	if pattern == "" {
		t.Fatalf("маршрут /auth/yandex/code не зарегистрирован")
	}

	guarded := handler.RequireSameOrigin(nil, false, route)
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("статус = %d, ожидался 403", rec.Code)
	}
	if len(rec.Header().Values("Set-Cookie")) != 0 {
		t.Errorf("отклонённый запрос выставил куки: %v", rec.Header().Values("Set-Cookie"))
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
