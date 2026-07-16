package store

import (
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"gorm.io/gorm"
)

// ErrTokenInvalid is returned when an enrollment token cannot be consumed.
var ErrTokenInvalid = errors.New("enrollment token is invalid, expired, used, or revoked")

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// randToken returns a cryptographically random base62 string of length n.
func randToken(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(base62)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = base62[idx.Int64()]
	}
	return string(b), nil
}

// TokenStatus is the derived lifecycle state of an enrollment token.
type TokenStatus string

const (
	TokenActive  TokenStatus = "active"
	TokenUsed    TokenStatus = "used"
	TokenExpired TokenStatus = "expired"
	TokenRevoked TokenStatus = "revoked"
)

// Status computes the current status of a token relative to now.
func (t EnrollmentToken) Status(now time.Time) TokenStatus {
	switch {
	case t.Revoked:
		return TokenRevoked
	case t.UsedAt != nil:
		return TokenUsed
	case now.After(t.ExpiresAt):
		return TokenExpired
	default:
		return TokenActive
	}
}

// CreateEnrollToken mints a new enrollment token valid for ttl (org-scoped).
func (s *Store) CreateEnrollToken(orgID, label string, ttl time.Duration, now time.Time) (EnrollmentToken, error) {
	tok, err := randToken(32)
	if err != nil {
		return EnrollmentToken{}, err
	}
	rec := EnrollmentToken{
		Token:     tok,
		OrgID:     orgID,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.db.Create(&rec).Error; err != nil {
		return EnrollmentToken{}, err
	}
	return rec, nil
}

// ListEnrollTokens returns an org's enrollment tokens, newest first.
func (s *Store) ListEnrollTokens(orgID string) ([]EnrollmentToken, error) {
	var out []EnrollmentToken
	return out, s.db.Where("org_id = ?", orgID).Order("created_at desc").Find(&out).Error
}

// RevokeEnrollToken marks a token revoked within an org. Idempotent.
func (s *Store) RevokeEnrollToken(orgID, token string) error {
	return s.db.Model(&EnrollmentToken{}).
		Where("org_id = ? AND token = ?", orgID, token).
		Update("revoked", true).Error
}

// ConsumeEnrollToken atomically validates and consumes a token, minting and
// returning a durable AgentKey bound to machineID and the token's org. It fails
// if the token is expired, already used, or revoked.
func (s *Store) ConsumeEnrollToken(token, machineID, label string, now time.Time) (AgentKey, error) {
	var key AgentKey
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var rec EnrollmentToken
		if err := tx.Where("token = ?", token).First(&rec).Error; err != nil {
			return ErrTokenInvalid
		}
		if rec.Status(now) != TokenActive {
			return ErrTokenInvalid
		}
		// Mark used.
		rec.UsedAt = &now
		rec.UsedByID = machineID
		if err := tx.Save(&rec).Error; err != nil {
			return err
		}
		// Mint the durable agent key, inheriting the token's org.
		raw, err := randToken(40)
		if err != nil {
			return err
		}
		key = AgentKey{Key: raw, OrgID: rec.OrgID, MachineID: machineID, Label: label, CreatedAt: now}
		return tx.Create(&key).Error
	})
	if err != nil {
		return AgentKey{}, err
	}
	return key, nil
}

// ActiveAgentKeys returns all non-revoked agent keys.
func (s *Store) ActiveAgentKeys() ([]AgentKey, error) {
	var out []AgentKey
	return out, s.db.Where("revoked = ?", false).Find(&out).Error
}

// RevokeAgentKeysForMachine revokes every key bound to a machine.
func (s *Store) RevokeAgentKeysForMachine(machineID string) error {
	return s.db.Model(&AgentKey{}).
		Where("machine_id = ?", machineID).
		Update("revoked", true).Error
}

// PruneExpiredTokens deletes tokens that expired before the cutoff and were
// never used (used tokens are kept for audit).
func (s *Store) PruneExpiredTokens(before time.Time) (int64, error) {
	res := s.db.Where("used_at IS NULL AND expires_at < ?", before).Delete(&EnrollmentToken{})
	return res.RowsAffected, res.Error
}
