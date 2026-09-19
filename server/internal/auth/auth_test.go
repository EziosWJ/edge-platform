package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type memoryStore struct {
	users       map[string]User
	sessions    map[string]AuthSession
	failureLogs []LoginLog
	successLogs []LoginLog
	menus       []CurrentUserMenu
	findCalls   int
}

func newMemoryStore(users ...User) *memoryStore {
	byUsername := make(map[string]User, len(users))
	for _, user := range users {
		byUsername[user.Username] = user
	}
	return &memoryStore{users: byUsername, sessions: map[string]AuthSession{}}
}

func (s *memoryStore) FindUserByUsername(_ context.Context, username string) (*User, error) {
	s.findCalls++
	user, ok := s.users[username]
	if !ok {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (s *memoryStore) RecordLoginFailure(_ context.Context, log LoginLog) error {
	s.failureLogs = append(s.failureLogs, log)
	return nil
}

func (s *memoryStore) CompleteLogin(_ context.Context, userID int64, loginTime time.Time, loginIP string, session AuthSession, log LoginLog) error {
	for username, user := range s.users {
		if user.ID == userID {
			user.LastLoginTime = &loginTime
			user.LastLoginIP = &loginIP
			s.users[username] = user
			break
		}
	}
	s.sessions[session.JTI] = session
	s.successLogs = append(s.successLogs, log)
	return nil
}

func (s *memoryStore) IsSessionActive(_ context.Context, userID int64, jti string, now time.Time) (bool, error) {
	session, ok := s.sessions[jti]
	return ok && session.UserID == userID && session.RevokedAt == nil && session.ExpiresAt.After(now), nil
}

func (s *memoryStore) RevokeSession(_ context.Context, userID int64, jti string, revokedAt time.Time) error {
	session, ok := s.sessions[jti]
	if ok && session.UserID == userID && session.RevokedAt == nil {
		session.RevokedAt = &revokedAt
		s.sessions[jti] = session
	}
	return nil
}

func (s *memoryStore) RevokeSessionsByUserID(_ context.Context, userID int64, revokedAt time.Time) error {
	for jti, session := range s.sessions {
		if session.UserID == userID && session.RevokedAt == nil {
			session.RevokedAt = &revokedAt
			s.sessions[jti] = session
		}
	}
	return nil
}

func (s *memoryStore) FindCurrentUser(_ context.Context, userID int64) (*CurrentUser, error) {
	for _, user := range s.users {
		if user.ID == userID {
			return &CurrentUser{
				ID: user.ID, Username: user.Username, Nickname: user.Nickname,
				Roles: []CurrentUserRole{{ID: 1, RoleName: "管理员", RoleCode: "ADMIN"}},
			}, nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *memoryStore) FindVisibleMenusByUserID(_ context.Context, _ int64) ([]CurrentUserMenu, error) {
	return append([]CurrentUserMenu(nil), s.menus...), nil
}

func TestLoginCreatesSessionAndWritesOnlyRequiredJWTClaims(t *testing.T) {
	passwordHash := passwordHash(t, "correct-password")
	store := newMemoryStore(User{ID: 7, Username: "admin", PasswordHash: passwordHash, Status: UserStatusEnabled})
	service, manager := testService(t, store)

	result, err := service.Login(context.Background(), LoginInput{Username: " admin ", Password: "correct-password"}, LoginMetadata{ClientIP: "127.0.0.1", UserAgent: "test-agent"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.TokenName != "Authorization" || !strings.HasPrefix(result.TokenValue, "Bearer ") || result.ExpiresIn != 7200 {
		t.Fatalf("login result = %+v", result)
	}
	if len(store.sessions) != 1 || len(store.successLogs) != 1 || len(store.failureLogs) != 0 {
		t.Fatalf("sessions=%d successLogs=%d failureLogs=%d", len(store.sessions), len(store.successLogs), len(store.failureLogs))
	}
	if store.successLogs[0].LoginStatus != LoginStatusSuccess || store.successLogs[0].Message != "登录成功" {
		t.Fatalf("success log = %+v", store.successLogs[0])
	}

	principal, err := service.Authenticate(context.Background(), result.TokenValue)
	if err != nil || principal.UserID != 7 || principal.JTI == "" {
		t.Fatalf("Authenticate() = %+v, %v", principal, err)
	}

	parsed, err := jwt.Parse(strings.TrimPrefix(result.TokenValue, "Bearer "), func(*jwt.Token) (any, error) {
		return []byte(manager.config.SigningKey), nil
	})
	if err != nil {
		t.Fatalf("parse issued JWT: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	for _, expected := range []string{"sub", "jti", "iat", "exp", "iss", "aud"} {
		if _, ok := claims[expected]; !ok {
			t.Fatalf("JWT misses %q: %v", expected, claims)
		}
	}
	if len(claims) != 6 {
		t.Fatalf("JWT contains unapproved claims: %v", claims)
	}
}

func TestLoginFailuresAreAudited(t *testing.T) {
	disabled := User{ID: 8, Username: "disabled", PasswordHash: passwordHash(t, "correct-password"), Status: 0}
	store := newMemoryStore(disabled)
	service, _ := testService(t, store)

	_, err := service.Login(context.Background(), LoginInput{Username: "missing", Password: "wrong"}, LoginMetadata{ClientIP: "192.0.2.3"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("missing user Login() error = %v", err)
	}
	_, err = service.Login(context.Background(), LoginInput{Username: "disabled", Password: "correct-password"}, LoginMetadata{})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled user Login() error = %v", err)
	}
	if len(store.failureLogs) != 2 {
		t.Fatalf("failure logs = %d, want 2", len(store.failureLogs))
	}
	for _, log := range store.failureLogs {
		if log.LoginStatus != LoginStatusFailure || log.LoginTime.IsZero() || log.Message == "" {
			t.Fatalf("invalid failure log: %+v", log)
		}
	}
}

func TestLoginGuardRejectsBeforeUserLookupAndFailureLog(t *testing.T) {
	store := newMemoryStore(User{ID: 8, Username: "admin", PasswordHash: passwordHash(t, "correct-password"), Status: UserStatusEnabled})
	guard := newTestLoginGuard(t, LoginGuardConfig{
		IPWindow:       time.Minute,
		IPMaxAttempts:  1,
		BackoffInitial: time.Second,
		BackoffMax:     2 * time.Second,
		LockDuration:   time.Minute,
		MaxEntries:     10,
	})
	service, _ := testService(t, store, guard)

	_, err := service.Login(context.Background(), LoginInput{Username: "admin", Password: "wrong"}, LoginMetadata{ClientIP: "203.0.113.8"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("first Login() error = %v, want invalid credentials", err)
	}
	_, err = service.Login(context.Background(), LoginInput{Username: "admin", Password: "wrong"}, LoginMetadata{ClientIP: "203.0.113.8"})
	if !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("second Login() error = %v, want rate limited", err)
	}
	if store.findCalls != 1 {
		t.Fatalf("user lookup calls = %d, want 1", store.findCalls)
	}
	if len(store.failureLogs) != 1 {
		t.Fatalf("failure logs = %d, want 1", len(store.failureLogs))
	}
}

func TestEmptyLoginInputDoesNotCreateCredentialFailureLog(t *testing.T) {
	store := newMemoryStore()
	service, _ := testService(t, store)

	_, err := service.Login(context.Background(), LoginInput{Username: " ", Password: ""}, LoginMetadata{ClientIP: "203.0.113.8"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("empty Login() error = %v, want invalid credentials", err)
	}
	if len(store.failureLogs) != 0 {
		t.Fatalf("failure logs = %d, want 0", len(store.failureLogs))
	}
}

func TestLogoutAndUserWideRevocationInvalidateConcurrentSessions(t *testing.T) {
	store := newMemoryStore(User{ID: 9, Username: "user", PasswordHash: passwordHash(t, "password"), Status: UserStatusEnabled})
	service, _ := testService(t, store)

	first, err := service.Login(context.Background(), LoginInput{Username: "user", Password: "password"}, LoginMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(context.Background(), LoginInput{Username: "user", Password: "password"}, LoginMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	firstPrincipal, err := service.Authenticate(context.Background(), first.TokenValue)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), firstPrincipal); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), first.TokenValue); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("logged-out token error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), second.TokenValue); err != nil {
		t.Fatalf("other concurrent session should remain valid: %v", err)
	}
	if err := service.RevokeUserSessions(context.Background(), 9); err != nil {
		t.Fatalf("RevokeUserSessions() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), second.TokenValue); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("user-wide revoked token error = %v", err)
	}
}

func TestTokenManagerRejectsWrongAlgorithm(t *testing.T) {
	manager, err := NewTokenManager(TokenConfig{SigningKey: "test-signing-key", Issuer: "issuer", Audience: "audience", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	claims := tokenClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "issuer", Subject: "1", Audience: jwt.ClaimStrings{"audience"}, ID: "jti",
		IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte("test-signing-key"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Verify(wrongAlgorithm)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Verify() wrong algorithm error = %v", err)
	}
}

func TestCurrentUserMenusBuildsAndSortsTree(t *testing.T) {
	store := newMemoryStore(User{ID: 10, Username: "user", Status: UserStatusEnabled})
	store.menus = []CurrentUserMenu{
		{ID: 6, ParentID: 2, MenuName: "grandchild", SortOrder: 1},
		{ID: 4, ParentID: 1, MenuName: "third", SortOrder: 2},
		{ID: 3, ParentID: 1, MenuName: "first by ID", SortOrder: 1},
		{ID: 2, ParentID: 1, MenuName: "second by ID", SortOrder: 1},
		{ID: 1, ParentID: 0, MenuName: "root", SortOrder: 2},
		{ID: 5, ParentID: 999, MenuName: "orphan root", SortOrder: 1},
	}
	service, _ := testService(t, store)
	menus, err := service.CurrentUserMenus(context.Background(), Principal{UserID: 10})
	if err != nil {
		t.Fatalf("CurrentUserMenus() error = %v", err)
	}
	if len(menus) != 2 || menus[0].ID != 5 || menus[1].ID != 1 {
		t.Fatalf("root menus = %+v", menus)
	}
	children := menus[1].Children
	if len(children) != 3 || children[0].ID != 2 || children[1].ID != 3 || children[2].ID != 4 {
		t.Fatalf("sorted children = %+v", children)
	}
	if len(children[0].Children) != 1 || children[0].Children[0].ID != 6 {
		t.Fatalf("nested children = %+v", children[0].Children)
	}

	user, err := service.CurrentUser(context.Background(), Principal{UserID: 10})
	if err != nil || user.Username != "user" || len(user.Roles) != 1 {
		t.Fatalf("CurrentUser() = %+v, %v", user, err)
	}
}

func TestPrincipalContext(t *testing.T) {
	ctx := ContextWithPrincipal(context.Background(), Principal{UserID: 11, JTI: "session"})
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.UserID != 11 || principal.JTI != "session" {
		t.Fatalf("PrincipalFromContext() = %+v, %t", principal, ok)
	}
}

func TestModelTableNames(t *testing.T) {
	if (User{}).TableName() != "sys_user" || (AuthSession{}).TableName() != "auth_session" || (LoginLog{}).TableName() != "sys_login_log" {
		t.Fatal("authentication model table mapping changed")
	}
}

func TestResponseDTOJSONNamesMatchFrontendContract(t *testing.T) {
	encoded, err := json.Marshal(LoginResult{TokenName: "Authorization", TokenValue: "Bearer token", ExpiresIn: 7200})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"tokenName":"Authorization","tokenValue":"Bearer token","expiresIn":7200}` {
		t.Fatalf("login JSON = %s", encoded)
	}
}

func testService(t *testing.T, store Store, guards ...*LoginGuard) (*Service, *TokenManager) {
	t.Helper()
	manager, err := NewTokenManager(TokenConfig{SigningKey: "test-signing-key", Issuer: "issuer", Audience: "audience", TTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	sequence := 0
	manager.now = func() time.Time { return now }
	manager.newJTI = func() (string, error) {
		sequence++
		return "test-jti-" + string(rune('0'+sequence)), nil
	}
	service, err := NewService(store, manager, guards...)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, manager
}

func passwordHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}
