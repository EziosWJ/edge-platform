package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenConfig struct {
	SigningKey string
	Issuer     string
	Audience   string
	TTL        time.Duration
}

// TokenManager issues and verifies HS256 tokens. Claims deliberately contain
// only sub, jti, iat, exp, issuer, and audience; roles and menus remain in the
// database so changes take effect immediately.
type TokenManager struct {
	config TokenConfig
	now    func() time.Time
	newJTI func() (string, error)
}

type tokenClaims struct {
	jwt.RegisteredClaims
}

type issuedToken struct {
	Value     string
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

func NewTokenManager(config TokenConfig) (*TokenManager, error) {
	if strings.TrimSpace(config.SigningKey) == "" {
		return nil, errors.New("JWT signing key is required")
	}
	if strings.TrimSpace(config.Issuer) == "" {
		return nil, errors.New("JWT issuer is required")
	}
	if strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("JWT audience is required")
	}
	if config.TTL <= 0 {
		return nil, errors.New("JWT TTL must be greater than zero")
	}
	return &TokenManager{config: config, now: time.Now, newJTI: randomJTI}, nil
}

func (m *TokenManager) Issue(userID int64) (issuedToken, error) {
	if userID <= 0 {
		return issuedToken{}, errors.New("JWT subject must be a positive user ID")
	}
	jti, err := m.newJTI()
	if err != nil {
		return issuedToken{}, fmt.Errorf("create JWT ID: %w", err)
	}
	issuedAt := m.now().UTC()
	expiresAt := issuedAt.Add(m.config.TTL)
	claims := tokenClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    m.config.Issuer,
		Subject:   strconv.FormatInt(userID, 10),
		Audience:  jwt.ClaimStrings{m.config.Audience},
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ID:        jti,
	}}

	value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(m.config.SigningKey))
	if err != nil {
		return issuedToken{}, fmt.Errorf("sign JWT: %w", err)
	}
	return issuedToken{Value: value, JTI: jti, IssuedAt: issuedAt, ExpiresAt: expiresAt}, nil
}

func (m *TokenManager) Verify(value string) (Principal, error) {
	claims := new(tokenClaims)
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.config.Issuer),
		jwt.WithAudience(m.config.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	token, err := parser.ParseWithClaims(value, claims, func(token *jwt.Token) (any, error) {
		return []byte(m.config.SigningKey), nil
	})
	if err != nil || !token.Valid {
		return Principal{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil || claims.ID == "" {
		return Principal{}, fmt.Errorf("%w: required claims are missing", ErrInvalidToken)
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return Principal{}, fmt.Errorf("%w: invalid subject", ErrInvalidToken)
	}
	return Principal{UserID: userID, JTI: claims.ID, ExpiresAt: claims.ExpiresAt.Time}, nil
}

func randomJTI() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
