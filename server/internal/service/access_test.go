package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Стабы ниже повторяют семантику, которую Task 3 реализует в PostgresRepository/MemoryRepository:
// EnsureUser не трогает role и has_access у существующей строки (Decision 11), а CreateRequest
// ведёт себя как upsert из Decision 5 — повторная подача при статусе pending даёт ErrConflict,
// подача после решения перезаписывает статус, сохраняя decided_by/decided_at.

type ensureUserCall struct {
	login       string
	email       string
	displayName string
}

type setAccessCall struct {
	login   string
	granted bool
}

type userRepoStub struct {
	users map[string]UserRecord

	ensureCalls    []ensureUserCall
	setAccessCalls []setAccessCall
	getUserCalls   int
	listUsersCalls int

	ensureErr    error
	getUserErr   error
	setAccessErr error
}

func newUserRepoStub(records ...UserRecord) *userRepoStub {
	stub := &userRepoStub{users: make(map[string]UserRecord, len(records))}
	for _, record := range records {
		stub.users[record.Login] = record
	}
	return stub
}

func (u *userRepoStub) EnsureUser(_ context.Context, login, email, displayName string) error {
	u.ensureCalls = append(u.ensureCalls, ensureUserCall{login: login, email: email, displayName: displayName})
	if u.ensureErr != nil {
		return u.ensureErr
	}
	existing, ok := u.users[login]
	if !ok {
		u.users[login] = UserRecord{
			Login:       login,
			Email:       email,
			DisplayName: displayName,
			Role:        RoleUser,
			HasAccess:   false,
			CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		return nil
	}
	existing.Email = email
	existing.DisplayName = displayName
	u.users[login] = existing
	return nil
}

func (u *userRepoStub) GetUser(_ context.Context, login string) (UserRecord, error) {
	u.getUserCalls++
	if u.getUserErr != nil {
		return UserRecord{}, u.getUserErr
	}
	record, ok := u.users[login]
	if !ok {
		return UserRecord{}, ErrNotFound
	}
	return record, nil
}

func (u *userRepoStub) ListUsers(_ context.Context) ([]UserRecord, error) {
	u.listUsersCalls++
	records := make([]UserRecord, 0, len(u.users))
	for _, record := range u.users {
		records = append(records, record)
	}
	return records, nil
}

func (u *userRepoStub) SetAccess(_ context.Context, login string, granted bool) error {
	u.setAccessCalls = append(u.setAccessCalls, setAccessCall{login: login, granted: granted})
	if u.setAccessErr != nil {
		return u.setAccessErr
	}
	record, ok := u.users[login]
	if !ok {
		return ErrNotFound
	}
	record.HasAccess = granted
	u.users[login] = record
	return nil
}

type decideRequestCall struct {
	login     string
	status    string
	decidedBy string
}

type accessRequestRepoStub struct {
	requests map[string]AccessRequest

	createCalls []string
	decideCalls []decideRequestCall
	getCalls    int

	createErr error
	decideErr error
}

func newAccessRequestRepoStub(requests ...AccessRequest) *accessRequestRepoStub {
	stub := &accessRequestRepoStub{requests: make(map[string]AccessRequest, len(requests))}
	for _, request := range requests {
		stub.requests[request.UserID] = request
	}
	return stub
}

func (a *accessRequestRepoStub) CreateRequest(_ context.Context, login string) error {
	a.createCalls = append(a.createCalls, login)
	if a.createErr != nil {
		return a.createErr
	}
	existing, ok := a.requests[login]
	if ok && existing.Status == requestStatusPending {
		// Реальный репозиторий здесь получает RowsAffected() == 0 (Decision 5).
		return ErrConflict
	}
	created := AccessRequest{
		UserID:    login,
		Status:    requestStatusPending,
		CreatedAt: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
	}
	if ok {
		// decided_by/decided_at переживают повторную подачу (Decision 5).
		created.DecidedAt = existing.DecidedAt
		created.DecidedBy = existing.DecidedBy
	}
	a.requests[login] = created
	return nil
}

func (a *accessRequestRepoStub) GetRequest(_ context.Context, login string) (AccessRequest, error) {
	a.getCalls++
	request, ok := a.requests[login]
	if !ok {
		return AccessRequest{}, ErrNotFound
	}
	return request, nil
}

func (a *accessRequestRepoStub) DecideRequest(_ context.Context, login, status, decidedBy string) error {
	a.decideCalls = append(a.decideCalls, decideRequestCall{login: login, status: status, decidedBy: decidedBy})
	if a.decideErr != nil {
		return a.decideErr
	}
	request, ok := a.requests[login]
	if !ok {
		// Отсутствие заявки — не ошибка: администратор может выдать доступ и без неё.
		return nil
	}
	decidedAt := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	request.Status = status
	request.DecidedAt = &decidedAt
	request.DecidedBy = decidedBy
	a.requests[login] = request
	return nil
}

func TestAccessService_NoAccessNoRequest_ReturnsEmptyState(t *testing.T) {
	t.Parallel()

	userRepo := newUserRepoStub(UserRecord{
		Login:       "ivan",
		DisplayName: "Иван",
		Email:       "ivan@example.com",
		Role:        RoleUser,
		HasAccess:   false,
	})
	svc := NewAccessService(userRepo, newAccessRequestRepoStub())

	state, err := svc.GetAccessState(context.Background(), "ivan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.HasAccess {
		t.Fatalf("expected no access, got has_access=true")
	}
	if state.RequestStatus != "" {
		t.Fatalf("expected empty request status, got %q", state.RequestStatus)
	}
	if state.Login != "ivan" || state.DisplayName != "Иван" || state.Email != "ivan@example.com" {
		t.Fatalf("identity fields not carried over: %+v", state)
	}
	if state.Role != RoleUser {
		t.Fatalf("expected role %q, got %q", RoleUser, state.Role)
	}
}

func TestAccessService_AdminWithoutAccessFlag_HasAccess(t *testing.T) {
	t.Parallel()

	// Decision 10 / US-14: роль администратора важнее флага — строка, созданная входом
	// до миграции, остаётся с has_access=false, но доступ у неё есть.
	userRepo := newUserRepoStub(UserRecord{
		Login:     "rogogdbd",
		Role:      RoleAdmin,
		HasAccess: false,
	})
	svc := NewAccessService(userRepo, newAccessRequestRepoStub())

	state, err := svc.GetAccessState(context.Background(), "rogogdbd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !state.HasAccess {
		t.Fatalf("admin with has_access=false must pass the access check")
	}
	if state.Role != RoleAdmin {
		t.Fatalf("expected role %q, got %q", RoleAdmin, state.Role)
	}
}

func TestAccessService_GetAccessState_UnknownLogin_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	svc := NewAccessService(newUserRepoStub(), newAccessRequestRepoStub())

	state, err := svc.GetAccessState(context.Background(), "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if state.HasAccess {
		t.Fatalf("failed lookup must not report access")
	}
}

func TestAccessService_EnsureUser_DelegatesWithoutResettingRoleAndAccess(t *testing.T) {
	t.Parallel()

	// Decision 11: вход не должен разжаловать администратора. Сама неразрушающая семантика
	// живёт в репозитории (Task 3); здесь фиксируется, что сервис передаёт вызов как есть
	// и не пытается сам перезаписать роль или флаг.
	userRepo := newUserRepoStub(UserRecord{
		Login:       "rogogdbd",
		DisplayName: "Старое имя",
		Email:       "old@example.com",
		Role:        RoleAdmin,
		HasAccess:   true,
	})
	svc := NewAccessService(userRepo, newAccessRequestRepoStub())

	if err := svc.EnsureUser(context.Background(), "rogogdbd", "new@example.com", "Новое имя"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(userRepo.ensureCalls) != 1 {
		t.Fatalf("expected exactly one EnsureUser call, got %d", len(userRepo.ensureCalls))
	}
	want := ensureUserCall{login: "rogogdbd", email: "new@example.com", displayName: "Новое имя"}
	if userRepo.ensureCalls[0] != want {
		t.Fatalf("arguments not passed through: got %+v, want %+v", userRepo.ensureCalls[0], want)
	}
	stored := userRepo.users["rogogdbd"]
	if stored.Role != RoleAdmin || !stored.HasAccess {
		t.Fatalf("login must not reset role/has_access: %+v", stored)
	}
	if stored.Email != "new@example.com" || stored.DisplayName != "Новое имя" {
		t.Fatalf("email/display name not updated: %+v", stored)
	}
}

func TestAccessService_CreateRequest_ByUserWithoutAccess_CreatesPending(t *testing.T) {
	t.Parallel()

	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser, HasAccess: false})
	requestRepo := newAccessRequestRepoStub()
	svc := NewAccessService(userRepo, requestRepo)

	if err := svc.CreateRequest(context.Background(), "ivan"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(requestRepo.createCalls) != 1 || requestRepo.createCalls[0] != "ivan" {
		t.Fatalf("expected one CreateRequest call for ivan, got %v", requestRepo.createCalls)
	}
	stored, ok := requestRepo.requests["ivan"]
	if !ok {
		t.Fatalf("request row was not created")
	}
	if stored.Status != requestStatusPending {
		t.Fatalf("expected status %q, got %q", requestStatusPending, stored.Status)
	}
	if len(requestRepo.decideCalls) != 0 {
		t.Fatalf("creating a request must not decide it: %v", requestRepo.decideCalls)
	}
}

func TestAccessService_CreateRequest_RepoZeroRowsAffected_ReturnsErrConflict(t *testing.T) {
	t.Parallel()

	// Заявка уже pending: upsert из Decision 5 даёт RowsAffected() == 0, репозиторий
	// сообщает об этом через ErrConflict, а сервис доносит его до вызывающей стороны.
	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser, RequestStatus: requestStatusPending})
	existing := AccessRequest{
		UserID:    "ivan",
		Status:    requestStatusPending,
		CreatedAt: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	requestRepo := newAccessRequestRepoStub(existing)
	svc := NewAccessService(userRepo, requestRepo)

	err := svc.CreateRequest(context.Background(), "ivan")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if len(requestRepo.createCalls) != 1 {
		t.Fatalf("expected the service to reach the repository once, got %d calls", len(requestRepo.createCalls))
	}
	if got := requestRepo.requests["ivan"]; got != existing {
		t.Fatalf("rejected re-submit must not touch the stored request: got %+v, want %+v", got, existing)
	}
}

func TestAccessService_CreateRequest_RepositoryFailure_IsNotReportedAsConflict(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("dial tcp: connection refused")
	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser})
	requestRepo := newAccessRequestRepoStub()
	requestRepo.createErr = repoErr
	svc := NewAccessService(userRepo, requestRepo)

	err := svc.CreateRequest(context.Background(), "ivan")
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected the repository error to be wrapped, got %v", err)
	}
	if errors.Is(err, ErrConflict) {
		t.Fatalf("an infrastructure failure must not be reported as a conflict: %v", err)
	}
}

func TestAccessService_CreateRequest_UserAlreadyHasAccess_ReturnsErrConflictWithoutRepoCall(t *testing.T) {
	t.Parallel()

	cases := map[string]UserRecord{
		"через флаг has_access": {Login: "ivan", Role: RoleUser, HasAccess: true},
		"через роль admin":      {Login: "rogogdbd", Role: RoleAdmin, HasAccess: false},
	}

	for name, record := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			userRepo := newUserRepoStub(record)
			requestRepo := newAccessRequestRepoStub()
			svc := NewAccessService(userRepo, requestRepo)

			err := svc.CreateRequest(context.Background(), record.Login)
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("expected ErrConflict, got %v", err)
			}
			if len(requestRepo.createCalls) != 0 {
				t.Fatalf("AccessRequestRepository must not be called at all, got %v", requestRepo.createCalls)
			}
			if len(requestRepo.requests) != 0 {
				t.Fatalf("no request row may appear: %+v", requestRepo.requests)
			}
		})
	}
}

func TestAccessService_CreateRequest_AfterRejected_Allowed(t *testing.T) {
	t.Parallel()

	decidedAt := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
	userRepo := newUserRepoStub(UserRecord{
		Login:         "ivan",
		Role:          RoleUser,
		HasAccess:     false,
		RequestStatus: requestStatusRejected,
	})
	requestRepo := newAccessRequestRepoStub(AccessRequest{
		UserID:    "ivan",
		Status:    requestStatusRejected,
		CreatedAt: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		DecidedAt: &decidedAt,
		DecidedBy: "rogogdbd",
	})
	svc := NewAccessService(userRepo, requestRepo)

	if err := svc.CreateRequest(context.Background(), "ivan"); err != nil {
		t.Fatalf("re-submit after a rejection must be allowed, got %v", err)
	}

	stored := requestRepo.requests["ivan"]
	if stored.Status != requestStatusPending {
		t.Fatalf("expected status %q, got %q", requestStatusPending, stored.Status)
	}
	if stored.DecidedBy != "rogogdbd" || stored.DecidedAt == nil || !stored.DecidedAt.Equal(decidedAt) {
		t.Fatalf("previous decision must survive the re-submit: %+v", stored)
	}
}

func TestAccessService_SetAccess_Grant_SetsFlagAndApprovesRequestWithDecidedBy(t *testing.T) {
	t.Parallel()

	userRepo := newUserRepoStub(UserRecord{
		Login:         "ivan",
		Role:          RoleUser,
		HasAccess:     false,
		RequestStatus: requestStatusPending,
	})
	requestRepo := newAccessRequestRepoStub(AccessRequest{UserID: "ivan", Status: requestStatusPending})
	svc := NewAccessService(userRepo, requestRepo)

	if err := svc.SetAccess(context.Background(), "ivan", true, "rogogdbd"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !userRepo.users["ivan"].HasAccess {
		t.Fatalf("access flag was not set")
	}
	wantSet := setAccessCall{login: "ivan", granted: true}
	if len(userRepo.setAccessCalls) != 1 || userRepo.setAccessCalls[0] != wantSet {
		t.Fatalf("expected one SetAccess(true) call, got %+v", userRepo.setAccessCalls)
	}
	wantDecide := decideRequestCall{login: "ivan", status: requestStatusApproved, decidedBy: "rogogdbd"}
	if len(requestRepo.decideCalls) != 1 || requestRepo.decideCalls[0] != wantDecide {
		t.Fatalf("expected one DecideRequest(approved) call, got %+v", requestRepo.decideCalls)
	}
	stored := requestRepo.requests["ivan"]
	if stored.Status != requestStatusApproved || stored.DecidedBy != "rogogdbd" || stored.DecidedAt == nil {
		t.Fatalf("request was not approved with decided_by/decided_at: %+v", stored)
	}
}

func TestAccessService_SetAccess_Revoke_UnsetsFlagAndRejectsRequest(t *testing.T) {
	t.Parallel()

	userRepo := newUserRepoStub(UserRecord{
		Login:         "ivan",
		Role:          RoleUser,
		HasAccess:     true,
		RequestStatus: requestStatusApproved,
	})
	requestRepo := newAccessRequestRepoStub(AccessRequest{UserID: "ivan", Status: requestStatusApproved, DecidedBy: "rogogdbd"})
	svc := NewAccessService(userRepo, requestRepo)

	if err := svc.SetAccess(context.Background(), "ivan", false, "second-admin"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if userRepo.users["ivan"].HasAccess {
		t.Fatalf("access flag was not cleared")
	}
	wantSet := setAccessCall{login: "ivan", granted: false}
	if len(userRepo.setAccessCalls) != 1 || userRepo.setAccessCalls[0] != wantSet {
		t.Fatalf("expected one SetAccess(false) call, got %+v", userRepo.setAccessCalls)
	}
	wantDecide := decideRequestCall{login: "ivan", status: requestStatusRejected, decidedBy: "second-admin"}
	if len(requestRepo.decideCalls) != 1 || requestRepo.decideCalls[0] != wantDecide {
		t.Fatalf("expected one DecideRequest(rejected) call, got %+v", requestRepo.decideCalls)
	}
	stored := requestRepo.requests["ivan"]
	if stored.Status != requestStatusRejected || stored.DecidedBy != "second-admin" {
		t.Fatalf("request was not rejected with the deciding admin: %+v", stored)
	}
}

func TestAccessService_SetAccess_UnknownLogin_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser})
	requestRepo := newAccessRequestRepoStub()
	svc := NewAccessService(userRepo, requestRepo)

	err := svc.SetAccess(context.Background(), "ghost", true, "rogogdbd")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if len(userRepo.setAccessCalls) != 0 {
		t.Fatalf("unknown login must not reach SetAccess: %+v", userRepo.setAccessCalls)
	}
	if len(requestRepo.decideCalls) != 0 {
		t.Fatalf("unknown login must not reach DecideRequest: %+v", requestRepo.decideCalls)
	}
}

func TestAccessService_SetAccess_NoExistingRequest_DoesNotCallDecideRequest(t *testing.T) {
	t.Parallel()

	// Администратор может выдать доступ тому, кто заявку никогда не подавал.
	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser, RequestStatus: ""})
	requestRepo := newAccessRequestRepoStub()
	svc := NewAccessService(userRepo, requestRepo)

	if err := svc.SetAccess(context.Background(), "ivan", true, "rogogdbd"); err != nil {
		t.Fatalf("granting access without a request is not an error, got %v", err)
	}

	if !userRepo.users["ivan"].HasAccess {
		t.Fatalf("access flag was not set")
	}
	if len(requestRepo.decideCalls) != 0 {
		t.Fatalf("DecideRequest must not be called at all, got %+v", requestRepo.decideCalls)
	}
}

func TestAccessService_SetAccess_FlagUpdateFails_DoesNotDecideRequest(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("write failed")
	userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser, RequestStatus: requestStatusPending})
	userRepo.setAccessErr = repoErr
	requestRepo := newAccessRequestRepoStub(AccessRequest{UserID: "ivan", Status: requestStatusPending})
	svc := NewAccessService(userRepo, requestRepo)

	err := svc.SetAccess(context.Background(), "ivan", true, "rogogdbd")
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected the repository error to be wrapped, got %v", err)
	}
	if len(requestRepo.decideCalls) != 0 {
		t.Fatalf("a request must not be decided when the flag was not written: %+v", requestRepo.decideCalls)
	}
	if requestRepo.requests["ivan"].Status != requestStatusPending {
		t.Fatalf("request status changed despite the failure: %+v", requestRepo.requests["ivan"])
	}
}

func TestAccessService_InvalidArguments_ReturnErrInvalidArgumentWithoutRepoCalls(t *testing.T) {
	t.Parallel()

	cases := map[string]func(context.Context, *AccessService) error{
		"EnsureUser с пустым логином": func(ctx context.Context, svc *AccessService) error {
			return svc.EnsureUser(ctx, "", "ivan@example.com", "Иван")
		},
		"EnsureUser с пробельным логином": func(ctx context.Context, svc *AccessService) error {
			return svc.EnsureUser(ctx, "   ", "ivan@example.com", "Иван")
		},
		"GetAccessState с пустым логином": func(ctx context.Context, svc *AccessService) error {
			_, err := svc.GetAccessState(ctx, "")
			return err
		},
		"CreateRequest с пустым логином": func(ctx context.Context, svc *AccessService) error {
			return svc.CreateRequest(ctx, "")
		},
		"SetAccess с пустым логином": func(ctx context.Context, svc *AccessService) error {
			return svc.SetAccess(ctx, "", true, "rogogdbd")
		},
		"SetAccess без указания администратора": func(ctx context.Context, svc *AccessService) error {
			return svc.SetAccess(ctx, "ivan", true, "")
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			userRepo := newUserRepoStub(UserRecord{Login: "ivan", Role: RoleUser})
			requestRepo := newAccessRequestRepoStub()
			svc := NewAccessService(userRepo, requestRepo)

			if err := call(context.Background(), svc); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("expected ErrInvalidArgument, got %v", err)
			}
			if userRepo.getUserCalls != 0 || len(userRepo.ensureCalls) != 0 || len(userRepo.setAccessCalls) != 0 {
				t.Fatalf("invalid input must not reach UserRepository: %+v", userRepo)
			}
			if len(requestRepo.createCalls) != 0 || len(requestRepo.decideCalls) != 0 {
				t.Fatalf("invalid input must not reach AccessRequestRepository: %+v", requestRepo)
			}
		})
	}
}
