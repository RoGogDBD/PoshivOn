package handler

import "testing"

// TestNormalizePath_AdminUsersLoginCollapses — Decision 18.
//
// WithMetrics оборачивает mux снаружи авторизации (порядок CORS → Metrics → mux в main.go),
// поэтому путь запроса попадает в метку Prometheus раньше, чем RequireAdmin успевает
// отклонить запрос. Существующее правило looksLikeID не сворачивает короткие логины без
// дефиса, а именно такие логины и стоят в /api/v1/admin/users/{login}/access — значит,
// анонимный вызывающий может создавать произвольные значения метки, и то, что сам запрос
// получит 401 или 403, кардинальность уже не спасает.
func TestNormalizePath_AdminUsersLoginCollapses(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "короткий логин без дефиса",
			path: "/api/v1/admin/users/bob/access",
			want: "/api/v1/admin/users/:id/access",
		},
		{
			name: "логин администратора из миграции 004",
			path: "/api/v1/admin/users/RoGogDBD/access",
			want: "/api/v1/admin/users/:id/access",
		},
		{
			name: "числовой логин",
			path: "/api/v1/admin/users/12345/access",
			want: "/api/v1/admin/users/:id/access",
		},
		{
			name: "логин с кириллицей и пробелом после декодирования",
			path: "/api/v1/admin/users/иван петров/access",
			want: "/api/v1/admin/users/:id/access",
		},
		{
			name: "сегмент сворачивается и без хвоста /access",
			path: "/api/v1/admin/users/bob",
			want: "/api/v1/admin/users/:id",
		},
		{
			name: "сам список пользователей не трогается",
			path: "/api/v1/admin/users",
			want: "/api/v1/admin/users",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizePath(testCase.path); got != testCase.want {
				t.Errorf("normalizePath(%q) = %q, ожидалось %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// TestNormalizePath_AdminUsersCardinalityIsBounded: смысл правила не в конкретной строке,
// а в том, что два разных логина дают одну метку. Без этого утверждения реализация вида
// «схлопывать только значение bob» прошла бы предыдущий тест.
func TestNormalizePath_AdminUsersCardinalityIsBounded(t *testing.T) {
	first := normalizePath("/api/v1/admin/users/bob/access")
	second := normalizePath("/api/v1/admin/users/alice/access")

	if first != second {
		t.Fatalf("разные логины дали разные метки: %q и %q", first, second)
	}
}

// TestNormalizePath_ExistingRulesUnchanged: правка Decision 18 добавляет условие, а не
// заменяет существующие — числовые и UUID-подобные сегменты сворачиваются как раньше,
// а первый сегмент пути не сворачивается никогда.
func TestNormalizePath_ExistingRulesUnchanged(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "числовой сегмент",
			path: "/api/v1/users/42/chats",
			want: "/api/v1/users/:id/chats",
		},
		{
			name: "UUID-подобный сегмент",
			path: "/api/v1/users/ivanov/chats/3fa85f64-5717-4562-b3fc-2c963f66afa6/calculate",
			want: "/api/v1/users/ivanov/chats/:id/calculate",
		},
		{
			name: "короткий логин вне админского маршрута не сворачивается (вне объёма Decision 18)",
			path: "/api/v1/users/bob/chats",
			want: "/api/v1/users/bob/chats",
		},
		{
			name: "первый сегмент не сворачивается даже будучи числом",
			path: "/12345/health",
			want: "/12345/health",
		},
		{
			name: "корень",
			path: "/",
			want: "/",
		},
		{
			name: "статические маршруты не меняются",
			path: "/api/v1/access/me",
			want: "/api/v1/access/me",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizePath(testCase.path); got != testCase.want {
				t.Errorf("normalizePath(%q) = %q, ожидалось %q", testCase.path, got, testCase.want)
			}
		})
	}
}
