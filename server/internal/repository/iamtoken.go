package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// IAMTokenSource выдаёт токен для авторизации перед HTTPRepository. Интерфейс, а не
// конкретный тип, — чтобы тесты HTTPRepository подставляли свою реализацию, не поднимая
// настоящий metadata-сервис.
type IAMTokenSource interface {
	Token(ctx context.Context) (string, error)
}

// metadataTokenEndpoint — стандартный metadata-сервис Yandex Cloud (тот же протокол, что и у
// Google Cloud: заголовок Metadata-Flavor: Google), доступен только изнутри инстанса/
// контейнера с назначенным сервисным аккаунтом (--service-account-id на ревизии). Отданный им
// токен уже привязан к SA самого бэкенд-контейнера — impersonation, которым проверяли
// db-service вручную при отладке, здесь не нужен: сервисный аккаунт уже тот самый.
const metadataTokenEndpoint = "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token"

// tokenRefreshMargin: токен обновляется чуть раньше формального истечения срока — иначе
// возможна отправка в db-service запроса с токеном, который протухнет между чтением из кэша
// и приходом ответа от IAM.
const tokenRefreshMargin = 60 * time.Second

// maxMetadataResponseBytes ограничивает тело ответа metadata-сервиса сверху: сам токен —
// несколько сотен байт, нескольких килобайт с огромным запасом достаточно для легитимного
// ответа. Без лимита неожиданно большой/испорченный ответ читался бы в память безусловно
// (та же категория риска, что и unbounded-чтение ответа db-service в http.go — code review).
const maxMetadataResponseBytes = 8 << 10 // 8 KiB

// MetadataTokenSource получает и кэширует IAM-токен с metadata-сервиса Yandex Cloud. Один
// экземпляр рассчитан на конкурентное использование из нескольких горутин HTTP-сервера
// бэкенда — отсюда мьютекс вместо кэша "на запрос".
type MetadataTokenSource struct {
	client   *http.Client
	endpoint string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewMetadataTokenSource(client *http.Client) *MetadataTokenSource {
	return &MetadataTokenSource{client: client, endpoint: metadataTokenEndpoint}
}

type metadataTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// Token отдаёт кэшированный токен или обновляет его. Сетевой запрос к metadata-сервису
// намеренно выполняется вне блокировки мьютекса (code review, security audit): держать её на
// время HTTP-вызова означало бы, что зависший/медленный ответ metadata сериализует за одной
// блокировкой вообще все параллельные запросы бэкенда к db-service, а не только тот, что
// столкнулся с истёкшим кэшем, — единственный медленный ответ превращался бы в полный простой.
// При гонке нескольких горутин на пустом кэше каждая может сходить в metadata сама — это
// несколько лишних запросов к локальному сервису на инстансе, не сетевой поход наружу, и при
// таком масштабе (единицы одновременных запросов) не стоит сложности singleflight ради этого.
func (s *MetadataTokenSource) Token(ctx context.Context) (string, error) {
	if token, ok := s.cachedToken(); ok {
		return token, nil
	}

	token, expiresAt, err := s.fetch(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.token = token
	s.expiresAt = expiresAt
	s.mu.Unlock()

	return token, nil
}

func (s *MetadataTokenSource) cachedToken() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expiresAt) {
		return s.token, true
	}
	return "", false
}

func (s *MetadataTokenSource) fetch(ctx context.Context) (token string, expiresAt time.Time, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build metadata token request: %w", err)
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetch iam token from metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("metadata token endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataResponseBytes+1))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read metadata token response: %w", err)
	}
	if len(body) > maxMetadataResponseBytes {
		return "", time.Time{}, fmt.Errorf("metadata token response exceeds %d bytes", maxMetadataResponseBytes)
	}

	var payload metadataTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", time.Time{}, fmt.Errorf("decode metadata token response: %w", err)
	}
	if payload.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("metadata token endpoint returned empty access_token")
	}

	return payload.AccessToken, time.Now().Add(time.Duration(payload.ExpiresIn)*time.Second - tokenRefreshMargin), nil
}
