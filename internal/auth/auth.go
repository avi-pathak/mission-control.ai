// Package auth provides password hashing and JWT issuance/verification for
// dashboard users. Agents authenticate separately via enrollment/agent keys.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Roles. The model is two-tier: admin (manages the workspace, members, and
// machines) and member (uses shared machines). RoleOwner is a deprecated alias
// kept only for migration of legacy rows; treat it as admin.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleOwner  = "owner" // deprecated: legacy, migrated to admin
)

// IsAdmin reports whether a role has workspace-admin privileges.
func IsAdmin(role string) bool {
	return role == RoleAdmin || role == RoleOwner
}

// NormalizeRole coerces any role string to the two-tier model.
func NormalizeRole(role string) string {
	if role == RoleMember {
		return RoleMember
	}
	return RoleAdmin
}

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
	UserID        string `json:"uid"`
	OrgID         string `json:"org"`
	Role          string `json:"role"`
	Email         string `json:"email"`
	PlatformAdmin bool   `json:"padmin,omitempty"`
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
	return m.IssueWithAdmin(userID, orgID, role, email, false, now)
}

// IssueWithAdmin mints a signed token, optionally flagging the user as a
// platform superadmin.
func (m *Manager) IssueWithAdmin(userID, orgID, role, email string, platformAdmin bool, now time.Time) (string, error) {
	claims := Claims{
		UserID:        userID,
		OrgID:         orgID,
		Role:          role,
		Email:         email,
		PlatformAdmin: platformAdmin,
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
