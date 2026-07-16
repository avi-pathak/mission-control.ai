// Package auth provides password hashing and JWT issuance/verification for
// dashboard users. Agents authenticate separately via enrollment/agent keys.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Roles.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// ErrInvalidToken is returned when a JWT cannot be validated.
var ErrInvalidToken = errors.New("invalid token")

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Claims is the JWT payload for a dashboard session.
type Claims struct {
	UserID string `json:"uid"`
	OrgID  string `json:"org"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Manager issues and verifies JWTs with a shared secret.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager builds a token manager. ttl defaults to 7 days when zero.
func NewManager(secret string, ttl time.Duration) *Manager {
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &Manager{secret: []byte(secret), ttl: ttl}
}

// Issue mints a signed token for a user.
func (m *Manager) Issue(userID, orgID, role, email string, now time.Time) (string, error) {
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Verify parses and validates a token, returning its claims.
func (m *Manager) Verify(token string) (*Claims, error) {
	var claims Claims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}
