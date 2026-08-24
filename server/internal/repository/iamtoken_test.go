package repository

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestMetadataSource(t *testing.T, endpoint string) *MetadataTokenSource {
	t.Helper()
	src := NewMetadataTokenSource(http.DefaultClient)
	src.endpoint = endpoint
	return src
}

func TestMetadataTokenSource_FetchesAndCaches(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Metadata-Flavor"); got != "Google" {
			t.Errorf("Metadata-Flavor = %q, ожидался Google", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok-1","expires_in":3600}`)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)

	token, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "tok-1" {
		t.Errorf("token = %q, ожидался tok-1", token)
	}

	// Второй вызов в пределах кэша не должен породить новый HTTP-запрос.
	token2, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token (2): %v", err)
	}
	if token2 != "tok-1" {
		t.Errorf("token (2) = %q, ожидался tok-1 из кэша", token2)
	}
	if requests != 1 {
		t.Errorf("requests = %d, ожидался 1 — второй вызов должен был обойтись кэшем", requests)
	}
}

func TestMetadataTokenSource_RefetchesAfterExpiry(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, requests)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)

	if _, err := src.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}

	// Симулируем истечение срока напрямую, а не сном в тесте — тот же пакет, доступ к
	// приватному полю оправдан: реальный refresh таймер здесь не тестируется.
	src.mu.Lock()
	src.expiresAt = time.Now().Add(-time.Second)
	src.mu.Unlock()

	token, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token (после истечения): %v", err)
	}
	if token != "tok-2" {
		t.Errorf("token = %q, ожидался tok-2 — истёкший кэш должен был вызвать повторный запрос", token)
	}
	if requests != 2 {
		t.Errorf("requests = %d, ожидалось 2", requests)
	}
}

func TestMetadataTokenSource_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)
	if _, err := src.Token(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка на статусе 500")
	}
}

func TestMetadataTokenSource_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)
	if _, err := src.Token(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка на невалидном JSON")
	}
}

func TestMetadataTokenSource_EmptyAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"","expires_in":3600}`)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)
	if _, err := src.Token(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка на пустом access_token")
	}
}

// TestMetadataTokenSource_ConcurrentAccess проверяет заявленное в доке MetadataTokenSource
// свойство — безопасность под конкурентным использованием несколькими горутинами
// HTTP-сервера (code review: этот тест раньше отсутствовал, а рефакторинг блокировки в
// Token() — снятие мьютекса на время сетевого вызова — как раз то место, где легко тихо
// сломать корректность). Гоняется с -race в общем прогоне пакета.
func TestMetadataTokenSource_ConcurrentAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok-1","expires_in":3600}`)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)

	const goroutines = 20
	var wg sync.WaitGroup
	tokens := make([]string, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = src.Token(context.Background())
		}(i)
	}
	wg.Wait()

	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if tokens[i] != "tok-1" {
			t.Errorf("goroutine %d: token = %q, ожидался tok-1", i, tokens[i])
		}
	}
}

func TestMetadataTokenSource_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"tok-1","expires_in":3600}`)
	}))
	defer srv.Close()

	src := newTestMetadataSource(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := src.Token(ctx); err == nil {
		t.Fatal("ожидалась ошибка на отменённом контексте")
	}
}
