package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// AccessHandler — HTTP-слой контура доступа: четыре маршрута из таблицы «Контракт API».
//
// Ни один из них не решает, вправе ли вызывающий сюда попасть: роль и доступ проверяет
// middleware на префиксе (RequireAuth / RequireAccess / RequireAdmin), и разбирать их
// повторно здесь означало бы завести вторую копию правил авторизации. Хендлер отвечает за
// другое: взять личность из контекста, вызвать сервис и отобразить результат на HTTP.
//
// Отправка письма администраторам (SMTPNotifier) — Phase 2, Task 16; нотификатора этот
// хендлер не знает вовсе.
type AccessHandler struct {
	access *service.AccessService
	// contactEmail из CONTACT_EMAIL: адрес, который видит на плашке пользователь без
	// доступа. Хранится строкой, а не *config.Config, — хендлеру нужна одна величина.
	contactEmail string
}

func NewAccessHandler(access *service.AccessService, contactEmail string) *AccessHandler {
	return &AccessHandler{access: access, contactEmail: strings.TrimSpace(contactEmail)}
}

// RegisterAccess и RegisterAdmin разделены не по смыслу ресурса, а по цепочке middleware:
// /api/v1/access/ закрыт только RequireAuth (туда идёт именно тот, у кого доступа ещё нет),
// /api/v1/admin/ — ещё и RequireAdmin. Одна регистрация на оба префикса не дала бы навесить
// на них разные цепочки, а разделение по строке пути внутри хендлера означало бы, что
// авторизация зависит от разбора адреса в коде обработчика.
func (h *AccessHandler) RegisterAccess(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/access/me", h.handleMe)
	mux.HandleFunc("POST /api/v1/access/requests", h.handleCreateRequest)
}

func (h *AccessHandler) RegisterAdmin(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/users", h.handleListUsers)
	mux.HandleFunc("POST /api/v1/admin/users/{login}/access", h.handleSetAccess)
}

// accessMeResponse — ответ GET /api/v1/access/me. Отдельный тип, а не service.AccessState:
// contact_email доменным полем не является, он приходит из конфигурации.
type accessMeResponse struct {
	Login         string       `json:"login"`
	DisplayName   string       `json:"display_name"`
	Email         string       `json:"email"`
	Role          service.Role `json:"role"`
	HasAccess     bool         `json:"has_access"`
	RequestStatus string       `json:"request_status"`
	ContactEmail  string       `json:"contact_email"`
}

// handleMe отдаёт состояние доступа текущего пользователя: на этом ответе клиент решает,
// показать рабочий интерфейс или плашку.
//
// has_access здесь — итоговое право из AccessState (Decision 10), поэтому администратор со
// снятой галочкой видит интерфейс, а не плашку.
func (h *AccessHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}

	state, err := h.access.GetAccessState(r.Context(), identity.Login)
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, accessMeResponse{
		Login:         state.Login,
		DisplayName:   state.DisplayName,
		Email:         state.Email,
		Role:          state.Role,
		HasAccess:     state.HasAccess,
		RequestStatus: state.RequestStatus,
		ContactEmail:  h.contactEmail,
	})
}

// handleCreateRequest подаёт заявку от текущего пользователя. Чей это запрос, решает
// сессия, а не тело: логин в теле позволил бы подать заявку от чужого имени (US-15).
//
// 409 приходит из сервиса в двух разных случаях — заявка уже на рассмотрении (Decision 5)
// и доступ уже выдан; различать их в ответе незачем, оба означают «повторно подавать
// нечего».
func (h *AccessHandler) handleCreateRequest(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}

	if err := h.access.CreateRequest(r.Context(), identity.Login); err != nil {
		writeAPIDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// handleListUsers отдаёт полный список пользователей администратору. Право на вызов уже
// проверил RequireAdmin в routes.go, но identity() здесь не лишняя копия той же проверки:
// это единственный из четырёх хендлеров, что отдаёт email/роль/доступ каждого пользователя
// разом, — если цепочка middleware когда-нибудь окажется собрана неверно, это первое место,
// где стоит перестраховаться, а не полагаться на то, что маршрутизация не ошибётся.
func (h *AccessHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.identity(w, r); !ok {
		return
	}

	users, err := h.access.ListUsers(r.Context())
	if err != nil {
		writeAPIDomainError(w, err)
		return
	}

	writeAPIJSON(w, http.StatusOK, map[string]any{"items": users})
}

// setAccessRequest — тело POST /api/v1/admin/users/{login}/access.
//
// granted — указатель, потому что нулевое значение bool неотличимо от отсутствующего поля:
// на `{}` доступ молча снимался бы, а `{}` — самое вероятное тело у ошибшегося клиента.
// Незаданное поле поэтому 400, а не «снять».
type setAccessRequest struct {
	Granted *bool `json:"granted"`
}

// handleSetAccess переключает флаг доступа и фиксирует решение по заявке.
//
// Кто решает — берётся из сессии (identity.Login), а не из тела и не из адреса: decided_by
// это единственный след того, кто выдал доступ. Право на сам вызов проверил RequireAdmin;
// AccessService.SetAccess своей проверки роли не делает и полагается на это.
func (h *AccessHandler) handleSetAccess(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identity(w, r)
	if !ok {
		return
	}

	login := r.PathValue("login")

	var payload setAccessRequest
	if err := decodeJSON(r, &payload); err != nil {
		// Фиксированный слаг вместо err.Error(): текст парсера называет неизвестное поле и
		// форму тела, то есть рассказывает про внутреннее устройство ровно так же, как
		// ошибка репозитория, которую убрал Decision 17. Исходная ошибка — в лог.
		log.Printf("api error: status=%d code=invalid_request err=%v", http.StatusBadRequest, err)
		writeAPIError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if payload.Granted == nil {
		log.Printf("api error: status=%d code=invalid_request err=granted field is required", http.StatusBadRequest)
		writeAPIError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	// Незнакомый {login} даёт ErrNotFound из GetUser внутри SetAccess и уходит наружу как
	// 404 — не 500 и не «успех без эффекта».
	if err := h.access.SetAccess(r.Context(), login, *payload.Granted, identity.Login); err != nil {
		writeAPIDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// identity достаёт вызывающего из контекста. Пустой результат означает неправильно
// собранную цепочку (маршрут без RequireAuth впереди) — отвечаем как на неопознанную
// сессию, а не работаем с пустым логином: ошибка сборки маршрутов не должна превращаться
// в действие от имени никого.
func (h *AccessHandler) identity(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok || strings.TrimSpace(identity.Login) == "" {
		writeAPIError(w, http.StatusUnauthorized, sessionIdentityMissingCode)
		return Identity{}, false
	}
	return identity, true
}
