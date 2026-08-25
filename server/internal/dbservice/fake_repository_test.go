package dbservice

import (
	"context"
	"database/sql"
	"time"

	"github.com/RoGogDBD/PoshivOn/internal/auth"
	"github.com/RoGogDBD/PoshivOn/internal/service"
)

// fakeRepository — единственная реализация всех пяти интерфейсов репозитория для тестов
// handlers.go. Не мок с заранее заданными ожиданиями — простой stateful stub: тест кладёт
// нужный результат в поля до вызова и проверяет поля last* после, чтобы убедиться, что
// HTTP-обёртка правильно достала параметры из запроса и вызвала правильный метод с
// правильным порядком аргументов (это и есть риск, который призвана ловить эта обёртка —
// сама бизнес-логика уже покрыта тестами internal/repository и internal/service).
type fakeRepository struct {
	userRecord   service.UserRecord
	userRecords  []service.UserRecord
	accessReq    service.AccessRequest
	settings     service.UserSettings
	chat         service.Chat
	chats        []service.Chat
	calculations []service.CalculationResult
	err          error

	lastLogin       string
	lastEmail       string
	lastDisplayName string
	lastGranted     bool
	lastStatus      string
	lastDecidedBy   string
	lastUserID      string
	lastChatID      string
	lastDeletedBy   string
	lastHard        bool
	lastSettings    service.UserSettings
	lastChat        service.Chat
	lastCalculation service.CalculationResult
}

func (f *fakeRepository) EnsureUser(_ context.Context, login, email, displayName string) error {
	f.lastLogin, f.lastEmail, f.lastDisplayName = login, email, displayName
	return f.err
}

func (f *fakeRepository) GetUser(_ context.Context, login string) (service.UserRecord, error) {
	f.lastLogin = login
	return f.userRecord, f.err
}

func (f *fakeRepository) ListUsers(_ context.Context) ([]service.UserRecord, error) {
	return f.userRecords, f.err
}

func (f *fakeRepository) SetAccess(_ context.Context, login string, granted bool) error {
	f.lastLogin, f.lastGranted = login, granted
	return f.err
}

func (f *fakeRepository) CreateRequest(_ context.Context, login string) error {
	f.lastLogin = login
	return f.err
}

func (f *fakeRepository) GetRequest(_ context.Context, login string) (service.AccessRequest, error) {
	f.lastLogin = login
	return f.accessReq, f.err
}

func (f *fakeRepository) DecideRequest(_ context.Context, login, status, decidedBy string) error {
	f.lastLogin, f.lastStatus, f.lastDecidedBy = login, status, decidedBy
	return f.err
}

func (f *fakeRepository) UpsertSettings(_ context.Context, userID string, settings service.UserSettings) error {
	f.lastUserID, f.lastSettings = userID, settings
	return f.err
}

func (f *fakeRepository) GetSettings(_ context.Context, userID string) (service.UserSettings, error) {
	f.lastUserID = userID
	return f.settings, f.err
}

func (f *fakeRepository) CreateChat(_ context.Context, chat service.Chat) (service.Chat, error) {
	f.lastChat = chat
	return f.chat, f.err
}

func (f *fakeRepository) ListChats(_ context.Context, userID string) ([]service.Chat, error) {
	f.lastUserID = userID
	return f.chats, f.err
}

func (f *fakeRepository) DeleteChat(_ context.Context, userID, chatID, deletedBy string, hard bool) error {
	f.lastUserID, f.lastChatID, f.lastDeletedBy, f.lastHard = userID, chatID, deletedBy, hard
	return f.err
}

func (f *fakeRepository) RestoreChat(_ context.Context, userID, chatID string) error {
	f.lastUserID, f.lastChatID = userID, chatID
	return f.err
}

func (f *fakeRepository) AppendCalculation(_ context.Context, result service.CalculationResult) error {
	f.lastCalculation = result
	return f.err
}

func (f *fakeRepository) ListCalculations(_ context.Context, userID, chatID string) ([]service.CalculationResult, error) {
	f.lastUserID, f.lastChatID = userID, chatID
	return f.calculations, f.err
}

var (
	_ service.UserRepository            = (*fakeRepository)(nil)
	_ service.AccessRequestRepository   = (*fakeRepository)(nil)
	_ service.UserSettingsRepository    = (*fakeRepository)(nil)
	_ service.ChatRepository            = (*fakeRepository)(nil)
	_ service.ChatCalculationRepository = (*fakeRepository)(nil)
)

func newTestDeps(repo *fakeRepository) Deps {
	return newTestDepsWithSessions(repo, &fakeSessionRepository{})
}

// newTestDepsWithSessions — вариант для тестов SessionRepository: им нужен контроль над
// fakeSessionRepository (заранее заданный stored/err), а не над остальными пятью.
func newTestDepsWithSessions(repo *fakeRepository, sessions *fakeSessionRepository) Deps {
	return Deps{
		Users:        repo,
		AccessReqs:   repo,
		Settings:     repo,
		Chats:        repo,
		Calculations: repo,
		Sessions:     sessions,
	}
}

// fakeSessionRepository — тот же stateful-stub приём, что и fakeRepository выше, отдельным
// типом: форма сессии (auth.Session, sql.Null*-поля) не похожа ни на один из пяти
// репозиториев, смешивать в одну структуру больше запутывало бы, чем экономило.
type fakeSessionRepository struct {
	stored auth.Session
	err    error

	lastCreated          auth.Session
	lastRefreshHash      string
	lastSessionID        uint64
	lastAccessToken      string
	lastRefreshToken     string
	lastAccessExpiresAt  time.Time
	lastRefreshExpiresAt time.Time
}

func (f *fakeSessionRepository) CreateSession(session *auth.Session) error {
	f.lastCreated = *session
	if f.err != nil {
		return f.err
	}
	session.ID = 42
	return nil
}

func (f *fakeSessionRepository) FindByRefreshHash(refreshHash string) (*auth.Session, error) {
	f.lastRefreshHash = refreshHash
	if f.err != nil {
		return nil, f.err
	}
	return &f.stored, nil
}

func (f *fakeSessionRepository) UpdateSessionTokens(sessionID uint64, refreshHash string, accessToken string, refreshToken sql.NullString, accessExpiresAt time.Time, refreshExpiresAt time.Time) error {
	f.lastSessionID, f.lastRefreshHash, f.lastAccessToken = sessionID, refreshHash, accessToken
	f.lastRefreshToken = refreshToken.String
	f.lastAccessExpiresAt, f.lastRefreshExpiresAt = accessExpiresAt, refreshExpiresAt
	return f.err
}

func (f *fakeSessionRepository) RevokeByRefreshHash(refreshHash string) error {
	f.lastRefreshHash = refreshHash
	return f.err
}

var _ SessionRepository = (*fakeSessionRepository)(nil)
