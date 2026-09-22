package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const dummyPasswordHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Service contains authentication rules. It only depends on context.Context,
// Store, and TokenManager; it has no Gin, GORM, or concrete driver dependency.
type Service struct {
	store      Store
	tokens     *TokenManager
	loginGuard *LoginGuard
	now        func() time.Time
}

var _ Authenticator = (*Service)(nil)

func NewService(store Store, tokens *TokenManager, guards ...*LoginGuard) (*Service, error) {
	if store == nil {
		return nil, errors.New("auth store is required")
	}
	if tokens == nil {
		return nil, errors.New("JWT token manager is required")
	}
	var loginGuard *LoginGuard
	if len(guards) > 0 {
		loginGuard = guards[0]
	}
	return &Service{store: store, tokens: tokens, loginGuard: loginGuard, now: time.Now}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput, metadata LoginMetadata) (LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if s.loginGuard != nil {
		if err := s.loginGuard.Admit(metadata.ClientIP, username); err != nil {
			loginAttempts.WithLabelValues(loginAttemptOutcomeGuardRejected).Inc()
			return LoginResult{}, err
		}
	}

	user, err := s.store.FindUserByUsername(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(input.Password))
		return LoginResult{}, s.failLogin(ctx, username, metadata, "用户不存在", ErrInvalidCredentials)
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("find login user: %w", err)
	}
	if user.Status != UserStatusEnabled {
		return LoginResult{}, s.failLogin(ctx, username, metadata, "用户已禁用", ErrUserDisabled)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return LoginResult{}, s.failLogin(ctx, username, metadata, "密码错误", ErrInvalidCredentials)
	}

	issued, err := s.tokens.Issue(user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	loginLog := LoginLog{
		Username:    user.Username,
		LoginStatus: LoginStatusSuccess,
		LoginIP:     metadata.ClientIP,
		UserAgent:   metadata.UserAgent,
		Message:     "登录成功",
		LoginTime:   issued.IssuedAt,
		CreateTime:  issued.IssuedAt,
	}
	session := AuthSession{
		UserID:     user.ID,
		JTI:        issued.JTI,
		ExpiresAt:  issued.ExpiresAt,
		CreateTime: issued.IssuedAt,
	}
	if err := s.store.CompleteLogin(ctx, user.ID, issued.IssuedAt, metadata.ClientIP, session, loginLog); err != nil {
		return LoginResult{}, fmt.Errorf("complete login: %w", err)
	}
	if s.loginGuard != nil {
		s.loginGuard.RecordSuccess(username)
	}
	loginAttempts.WithLabelValues(loginAttemptOutcomeSuccess).Inc()

	return LoginResult{
		TokenName:  "Authorization",
		TokenValue: "Bearer " + issued.Value,
		ExpiresIn:  int64(issued.ExpiresAt.Sub(issued.IssuedAt).Seconds()),
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, tokenValue string) (Principal, error) {
	principal, err := s.tokens.Verify(strings.TrimPrefix(strings.TrimSpace(tokenValue), "Bearer "))
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if err := s.ValidateSession(ctx, principal); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// ValidateSession re-checks the server-side session state for a principal
// that was authenticated earlier. It is deliberately independent of JWT
// parsing so long-lived connections can observe revoke and expiry semantics.
func (s *Service) ValidateSession(ctx context.Context, principal Principal) error {
	if principal.UserID <= 0 || principal.JTI == "" {
		return ErrUnauthenticated
	}
	now := s.now().UTC()
	if principal.ExpiresAt.IsZero() || !principal.ExpiresAt.After(now) {
		return ErrUnauthenticated
	}
	active, err := s.store.IsSessionActive(ctx, principal.UserID, principal.JTI, now)
	if err != nil {
		return fmt.Errorf("check authentication session: %w", err)
	}
	if !active {
		return ErrUnauthenticated
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, principal Principal) error {
	if principal.UserID <= 0 || principal.JTI == "" {
		return ErrUnauthenticated
	}
	if err := s.store.RevokeSession(ctx, principal.UserID, principal.JTI, s.now().UTC()); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// RevokeUserSessions is called by future user management flows after a user is
// disabled, deleted, or has their password reset.
func (s *Service) RevokeUserSessions(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errors.New("user ID must be positive")
	}
	if err := s.store.RevokeSessionsByUserID(ctx, userID, s.now().UTC()); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (s *Service) CurrentUser(ctx context.Context, principal Principal) (*CurrentUser, error) {
	if principal.UserID <= 0 {
		return nil, ErrUnauthenticated
	}
	user, err := s.store.FindCurrentUser(ctx, principal.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("find current user: %w", err)
	}
	return user, nil
}

func (s *Service) CurrentUserMenus(ctx context.Context, principal Principal) ([]CurrentUserMenu, error) {
	if principal.UserID <= 0 {
		return nil, ErrUnauthenticated
	}
	menus, err := s.store.FindVisibleMenusByUserID(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("find current user menus: %w", err)
	}
	return buildMenuTree(menus), nil
}

func buildMenuTree(menus []CurrentUserMenu) []CurrentUserMenu {
	type menuNode struct {
		menu     CurrentUserMenu
		children []*menuNode
	}
	byID := make(map[int64]*menuNode, len(menus))
	for _, menu := range menus {
		menu.Children = []CurrentUserMenu{}
		byID[menu.ID] = &menuNode{menu: menu}
	}

	roots := make([]*menuNode, 0)
	for _, menu := range menus {
		node := byID[menu.ID]
		parent, exists := byID[node.menu.ParentID]
		if node.menu.ParentID == 0 || !exists {
			roots = append(roots, node)
			continue
		}
		parent.children = append(parent.children, node)
	}

	var sortNodes func([]*menuNode)
	sortNodes = func(nodes []*menuNode) {
		sort.Slice(nodes, func(left, right int) bool {
			return menuComesFirst(nodes[left].menu, nodes[right].menu)
		})
		for _, node := range nodes {
			sortNodes(node.children)
		}
	}
	var asMenu func(*menuNode) CurrentUserMenu
	asMenu = func(node *menuNode) CurrentUserMenu {
		menu := node.menu
		menu.Children = make([]CurrentUserMenu, len(node.children))
		for index, child := range node.children {
			menu.Children[index] = asMenu(child)
		}
		return menu
	}

	sortNodes(roots)
	result := make([]CurrentUserMenu, len(roots))
	for index, root := range roots {
		result[index] = asMenu(root)
	}
	return result
}

func menuComesFirst(left, right CurrentUserMenu) bool {
	if left.SortOrder == right.SortOrder {
		return left.ID < right.ID
	}
	return left.SortOrder < right.SortOrder
}

func (s *Service) failLogin(ctx context.Context, username string, metadata LoginMetadata, message string, result error) error {
	if s.loginGuard != nil {
		s.loginGuard.RecordFailure(metadata.ClientIP, username)
	}
	loginAttempts.WithLabelValues(loginAttemptOutcomeCredentialFailed).Inc()
	now := s.now().UTC()
	log := LoginLog{
		Username:    username,
		LoginStatus: LoginStatusFailure,
		LoginIP:     metadata.ClientIP,
		UserAgent:   metadata.UserAgent,
		Message:     message,
		LoginTime:   now,
		CreateTime:  now,
	}
	if err := s.store.RecordLoginFailure(ctx, log); err != nil {
		return fmt.Errorf("record failed login: %w", err)
	}
	return result
}
