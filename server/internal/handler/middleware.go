package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// ctxKey — непубличный тип ключа контекста. Строковый ключ мог бы совпасть с ключом
// другого пакета, положенным в тот же контекст; типизированный — нет.
type ctxKey int

const identityKey ctxKey = iota

// sessionIdentityMissingCode — сессия есть, а личности в ней нет (строка создана до
// миграции 004). Код намеренно не входит в список, на который клиент запускает повтор
// (shouldRefreshAuth, client/src/utils/yandexAuth.js:14-16): ротация токена личность не
// восстановит, и повтор здесь означал бы бесконечный цикл вместо отправки на вход
// (Decision 2).
const sessionIdentityMissingCode = "session_identity_missing"

// Identity — кто вызывает. Берётся из строки сессии, а не из адреса и не из тела запроса:
// в этом весь смысл US-15.
type Identity struct {
	Login       string
	Email       string
	DisplayName string
}

func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}

// requireIdentity достаёт вызывающего из контекста для обработчика, стоящего за RequireAuth.
// Возвращает false, если ответ уже отправлен.
//
// Пустой результат означает неправильно собранную цепочку (маршрут без RequireAuth впереди) —
// отвечаем как на неопознанную сессию, а не работаем с пустым логином: ошибка сборки
// маршрутов не должна превращаться в действие от имени никого.
//
// Функция общая для AccessHandler и APIHandler намеренно: правило «нет личности — 401, а не
// работа от имени никого» живёт в одном месте, как и его родственник resolveAccessState для
// middleware. Две копии разошлись бы ровно тогда, когда одну из них поправят.
func requireIdentity(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || strings.TrimSpace(identity.Login) == "" {
		writeAPIError(w, http.StatusUnauthorized, sessionIdentityMissingCode)
		return Identity{}, false
	}
	return identity, true
}

// RequireAuth пропускает дальше только запрос с опознанной сессией и кладёт личность
// в контекст. Причины отказа проходят наружу теми же слагами, что и раньше, — на них
// держится retry клиента.
func RequireAuth(resolve SessionResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, sessionRejectionCode(err))
			return
		}
		if session == nil {
			writeError(w, http.StatusUnauthorized, errSessionNotFound.Error())
			return
		}

		login := strings.TrimSpace(session.YandexLogin.String)
		if !session.YandexLogin.Valid || login == "" {
			writeError(w, http.StatusUnauthorized, sessionIdentityMissingCode)
			return
		}

		identity := Identity{
			Login:       login,
			Email:       strings.TrimSpace(session.YandexEmail.String),
			DisplayName: strings.TrimSpace(session.YandexDisplayName.String),
		}
		next.ServeHTTP(w, r.WithContext(ContextWithIdentity(r.Context(), identity)))
	})
}

// sessionRejectionCodes — исчерпывающий перечень причин отказа, которые вправе уйти
// клиенту. Резолвер сессии — точка расширения (в тестах он подменяется целиком), и без
// этого фильтра текст произвольной ошибки чужой реализации уехал бы прямо в тело 401.
// Незнакомая причина схлопывается в session_not_found: клиент на этот код повтор не
// запускает и уводит пользователя на вход, то есть деградация закрытая.
var sessionRejectionCodes = map[string]struct{}{
	errAccessCookieMissing.Error():  {},
	errRefreshCookieMissing.Error(): {},
	errSessionNotFound.Error():      {},
	errSessionExpired.Error():       {},
	errAccessExpired.Error():        {},
	errAccessMismatch.Error():       {},
}

func sessionRejectionCode(err error) string {
	code := err.Error()
	if _, ok := sessionRejectionCodes[code]; ok {
		return code
	}
	log.Printf("auth: неизвестная причина отказа проверки сессии: %v", err)
	return errSessionNotFound.Error()
}

// RequireAccess пропускает того, у кого доступ есть по существу: право считает
// AccessService (has_access || role == 'admin', Decision 10), а не эта функция —
// второй копии правила в дереве быть не должно.
func RequireAccess(access *service.AccessService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, ok := resolveAccessState(w, r, access)
		if !ok {
			return
		}
		if !state.HasAccess {
			writeAPIDomainError(w, fmt.Errorf("access is not granted for %q: %w", state.Login, service.ErrForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin пропускает только администратора. Флаг has_access здесь не при чём:
// администратор с выключенным флагом остаётся администратором, а пользователь с
// включённым им не становится.
func RequireAdmin(access *service.AccessService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state, ok := resolveAccessState(w, r, access)
		if !ok {
			return
		}
		if state.Role != service.RoleAdmin {
			writeAPIDomainError(w, fmt.Errorf("admin role is required for %q: %w", state.Login, service.ErrForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// resolveAccessState — общая часть RequireAccess и RequireAdmin. Возвращает false, если
// ответ уже отправлен. Все ветки отказа закрытые: ни отсутствие личности, ни недоступное
// хранилище не пропускают запрос дальше.
func resolveAccessState(w http.ResponseWriter, r *http.Request, access *service.AccessService) (service.AccessState, bool) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || strings.TrimSpace(identity.Login) == "" {
		// Сюда попадают только при неправильно собранной цепочке (без RequireAuth впереди).
		// Отвечаем как на неопознанную сессию, а не пропускаем: ошибка сборки маршрутов не
		// должна превращаться в открытый эндпоинт.
		writeError(w, http.StatusUnauthorized, sessionIdentityMissingCode)
		return service.AccessState{}, false
	}

	if access == nil {
		writeAPIDomainError(w, errors.New("access service is not configured"))
		return service.AccessState{}, false
	}

	state, err := access.GetAccessState(r.Context(), identity.Login)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			// Строки users нет — доступа нет. Именно 403, а не 404: 404 сообщал бы
			// вызывающему, какие логины заведены в системе.
			writeAPIDomainError(w, fmt.Errorf("no user record for %q: %w", identity.Login, service.ErrForbidden))
			return service.AccessState{}, false
		}
		writeAPIDomainError(w, err)
		return service.AccessState{}, false
	}

	return state, true
}

// RequireSameOrigin отклоняет изменяющий запрос, пришедший не с нашей страницы.
//
// Пустой allowedOrigins — не разрешение всем, а переход на сверку со scheme://r.Host
// (Decision 8): CORS_ALLOWED_ORIGINS — необязательный секрет, на проде он правдоподобно
// окажется пустым, и «пусто — значит пропускаем» отключило бы защиту именно там, где она
// нужна. Схема приходит параметром из конфигурации (COOKIE_SECURE), а не угадывается по
// запросу: r.TLS за прокси всегда nil, а заголовкам X-Forwarded-Proto здесь верить нельзя —
// их выставляет тот же, кто присылает Origin.
//
// Разбор CSV остаётся на вызывающей стороне (main.go уже делает его для CORS): middleware
// получает готовые зависимости, как RequireAccess и RequireAdmin.
func RequireSameOrigin(allowedOrigins []string, secure bool, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET и HEAD состояния не меняют — блокировать их означало бы ломать обычную
		// навигацию ради ничего.
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if !originAllowed(origin, allowed, secure, r.Host) {
			// Отсутствующий Origin — тоже отказ: браузер обязан присылать его на
			// изменяющих запросах, а запрос без него неотличим от подделанного.
			writeAPIDomainError(w, fmt.Errorf("origin %q is not allowed: %w", origin, service.ErrForbidden))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func originAllowed(origin string, allowed map[string]struct{}, secure bool, host string) bool {
	if origin == "" {
		return false
	}
	if len(allowed) > 0 {
		_, ok := allowed[origin]
		return ok
	}
	if host == "" {
		return false
	}

	scheme := "http"
	if secure {
		scheme = "https"
	}
	return origin == scheme+"://"+host
}
