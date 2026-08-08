package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Role — роль пользователя в системе управления доступом.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Статусы заявки на доступ. Держим простыми строковыми константами, как costing.go делает
// с режимами калькулятора: отдельный enum-тип здесь ничего не защищает — значения приходят
// из БД и уходят в JSON строками.
const (
	requestStatusPending  = "pending"
	requestStatusApproved = "approved"
	requestStatusRejected = "rejected"
)

// AccessState — ответ на «есть ли у меня доступ» для GET /api/v1/access/me.
// HasAccess здесь — итоговый доступ (Decision 10: has_access || role == "admin"),
// а не сырой флаг из БД; сырой флаг живёт в UserRecord.HasAccess.
type AccessState struct {
	Login         string
	DisplayName   string
	Email         string
	Role          Role
	HasAccess     bool
	RequestStatus string // "" | "pending" | "approved" | "rejected"
}

// UserRecord — строка users вместе со статусом её заявки (join с access_requests).
// HasAccess — сырой флаг из БД; итоговое право доступа считает только AccessService.
//
// JSON-теги — потому что запись уходит клиенту как есть в GET /api/v1/admin/users
// (`{items:[UserRecord]}` из таблицы «Контракт API»); домен здесь несёт разметку так же,
// как её несут UserSettings и CalculationResult в costing.go.
type UserRecord struct {
	Login         string     `json:"login"`
	DisplayName   string     `json:"display_name"`
	Email         string     `json:"email"`
	Role          Role       `json:"role"`
	HasAccess     bool       `json:"has_access"`
	RequestStatus string     `json:"request_status"`
	RequestedAt   *time.Time `json:"requested_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AccessRequest — строка access_requests: одна на пользователя (Decision 5).
type AccessRequest struct {
	UserID    string
	Status    string // "pending" | "approved" | "rejected"
	CreatedAt time.Time
	DecidedAt *time.Time
	DecidedBy string
}

type UserRepository interface {
	// EnsureUser создаёт строку с role='user', has_access=false, а у существующей
	// обновляет только email и display_name (Decision 11).
	EnsureUser(ctx context.Context, login, email, displayName string) error
	// GetUser возвращает ErrNotFound, если логина нет.
	GetUser(ctx context.Context, login string) (UserRecord, error)
	ListUsers(ctx context.Context) ([]UserRecord, error)
	// SetAccess возвращает ErrNotFound, если логина нет.
	SetAccess(ctx context.Context, login string, granted bool) error
}

type AccessRequestRepository interface {
	// CreateRequest выполняет upsert заявки и возвращает ErrConflict, когда upsert
	// не задел ни одной строки — заявка уже pending (Decision 5). Контракт выражен
	// через ошибку, а не через дополнительный out-параметр: число задетых строк —
	// деталь реализации хранилища, наружу выходит только доменный смысл.
	CreateRequest(ctx context.Context, login string) error
	// GetRequest возвращает ErrNotFound, если заявки нет.
	GetRequest(ctx context.Context, login string) (AccessRequest, error)
	// DecideRequest переводит существующую заявку в approved/rejected. Отсутствие
	// строки заявки ошибкой не считается; AccessService такой вызов и не делает.
	DecideRequest(ctx context.Context, login, status, decidedBy string) error
}

// AccessService — единственное место, где живут правила доступа (Decisions 3, 5, 10, 11).
// Ветвление «кто имеет доступ», «когда звать DecideRequest» и «когда отвечать ErrConflict
// без обращения к репозиторию» держим здесь, чтобы handler'ы и репозитории не решали это
// каждый по-своему.
type AccessService struct {
	userRepo    UserRepository
	requestRepo AccessRequestRepository
}

func NewAccessService(userRepo UserRepository, requestRepo AccessRequestRepository) *AccessService {
	return &AccessService{
		userRepo:    userRepo,
		requestRepo: requestRepo,
	}
}

// EnsureUser вызывается на каждом входе. Неразрушающая семантика обновления —
// на стороне репозитория (Decision 11), сервис только делегирует.
func (s *AccessService) EnsureUser(ctx context.Context, login, email, displayName string) error {
	if err := requireLogin(login); err != nil {
		return err
	}
	if err := s.userRepo.EnsureUser(ctx, login, email, displayName); err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	return nil
}

// GetAccessState собирает состояние доступа для текущего пользователя.
func (s *AccessService) GetAccessState(ctx context.Context, login string) (AccessState, error) {
	if err := requireLogin(login); err != nil {
		return AccessState{}, err
	}
	user, err := s.userRepo.GetUser(ctx, login)
	if err != nil {
		return AccessState{}, fmt.Errorf("get user: %w", err)
	}
	return AccessState{
		Login:         user.Login,
		DisplayName:   user.DisplayName,
		Email:         user.Email,
		Role:          user.Role,
		HasAccess:     hasEffectiveAccess(user),
		RequestStatus: user.RequestStatus,
	}, nil
}

// ListUsers отдаёт всех, кто когда-либо входил, — список для администратора (US-7).
//
// Записи проходят насквозь, без применения Decision 10: HasAccess здесь остаётся сырой
// колонкой. Подмена её итоговым правом показала бы администратору собственную галочку
// выставленной, хотя в базе она снята, — и он не понимал бы, что именно снимает у себя и
// у второго администратора.
func (s *AccessService) ListUsers(ctx context.Context) ([]UserRecord, error) {
	users, err := s.userRepo.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// CreateRequest подаёт заявку на доступ. Пользователю, у которого доступ уже есть
// (включая доступ по роли администратора), заявка не нужна — ErrConflict без обращения
// к AccessRequestRepository. В остальных случаях решение принимает upsert репозитория:
// повторная подача при заявке на рассмотрении возвращает ErrConflict, подача после
// отказа — успех (Decision 5). Отдельной проверки «а не pending ли уже?» здесь нет
// намеренно: чтение перед записью создало бы гонку между двумя одновременными нажатиями.
func (s *AccessService) CreateRequest(ctx context.Context, login string) error {
	if err := requireLogin(login); err != nil {
		return err
	}
	user, err := s.userRepo.GetUser(ctx, login)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if hasEffectiveAccess(user) {
		return fmt.Errorf("access is already granted: %w", ErrConflict)
	}
	if err := s.requestRepo.CreateRequest(ctx, login); err != nil {
		return fmt.Errorf("create access request: %w", err)
	}
	return nil
}

// SetAccess — админская операция: переключает флаг доступа и фиксирует решение по заявке.
// Заявка решается только уже существующая: администратор вправе выдать доступ и тому, кто
// никогда не подавал заявку, и заводить ему заявку задним числом не нужно.
//
// Предусловие: метод НЕ проверяет, что вызывающий — администратор. Право вызова проверяет
// middleware RequireAdmin на маршруте /api/v1/admin/ (Task 4); decidedBy сюда приходит уже
// установленной личностью. Вызов из кода, не закрытого RequireAdmin, — эскалация привилегий.
func (s *AccessService) SetAccess(ctx context.Context, login string, granted bool, decidedBy string) error {
	if err := requireLogin(login); err != nil {
		return err
	}
	// decided_by — единственный след того, кто выдал или отозвал доступ; пустое значение
	// означало бы потерянную атрибуцию, поэтому это ошибка вызывающей стороны, а не норма.
	if strings.TrimSpace(decidedBy) == "" {
		return fmt.Errorf("deciding admin is required: %w", ErrInvalidArgument)
	}

	// Читаем пользователя до записи: так несуществующий логин даёт ErrNotFound, не создавая
	// строк, и заодно виден статус его заявки — второй запрос за ним не нужен.
	user, err := s.userRepo.GetUser(ctx, login)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if err := s.userRepo.SetAccess(ctx, login, granted); err != nil {
		return fmt.Errorf("set access: %w", err)
	}

	// Решаем по снимку, снятому GetUser выше, а не по свежему чтению заявки. Свежее чтение
	// гонку не закрывает — транзакции между вызовами здесь нет, — а лишний запрос добавляет.
	//
	// Что именно ломается в гонке (заявка, поданная между GetUser и DecideRequest):
	//   - снимок говорит «заявки нет» → DecideRequest не вызывается, и новая строка остаётся
	//     pending навсегда: доступ уже выдан, поэтому следующий CreateRequest замкнётся
	//     на ErrConflict выше и никогда её не перезапишет;
	//   - снимок говорит «заявка есть», а пользователь после снятия доступа успел подать
	//     новую → DecideRequest проставит ей approved/rejected с decided_by этого
	//     администратора, хотя её никто не рассматривал.
	// То есть пострадать может только строка заявки и точность decided_by (Decision 5).
	// На само право доступа это не влияет: флаг пишется ровно тем значением, которое запросил
	// администратор, независимо от свежести снимка, а источник истины о доступе — has_access
	// (Decision 3). Настоящее лечение — одна транзакция на уровне репозитория (Task 3),
	// а не второе чтение здесь.
	if user.RequestStatus == "" {
		return nil
	}

	status := requestStatusRejected
	if granted {
		status = requestStatusApproved
	}
	if err := s.requestRepo.DecideRequest(ctx, login, status, decidedBy); err != nil {
		return fmt.Errorf("decide access request: %w", err)
	}
	return nil
}

// hasEffectiveAccess — правило Decision 10 в одном месте: у администратора доступ есть
// всегда, флаг has_access у него игнорируется.
func hasEffectiveAccess(user UserRecord) bool {
	return user.HasAccess || user.Role == RoleAdmin
}

// maxLoginLength повторяет ширину users.id (VARCHAR(255), 002_costing_schema.up.sql:2).
// Считаем символы, а не байты — именно так эту ширину понимает MariaDB.
const maxLoginLength = 255

func requireLogin(login string) error {
	// Только проверка, без нормализации: приведение логина к канонической форме —
	// отдельное решение уровня схемы/входа, и молчаливая правка значения здесь
	// разошлась бы с тем, что реально лежит в users.id.
	if strings.TrimSpace(login) == "" {
		return fmt.Errorf("login is required: %w", ErrInvalidArgument)
	}
	// Отсекаем логин, который не помещается в ключевую колонку: усечение на стороне БД
	// склеило бы двух разных пользователей в одну строку users, то есть в одну личность.
	if utf8.RuneCountInString(login) > maxLoginLength {
		return fmt.Errorf("login is too long: %w", ErrInvalidArgument)
	}
	return nil
}
