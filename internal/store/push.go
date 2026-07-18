package store

import (
	"time"

	"github.com/google/uuid"
)

// PushSubscription is a browser Web Push subscription owned by a user, used to
// deliver blocked-session notifications. Endpoint is unique per browser/device.
type PushSubscription struct {
	ID        string `gorm:"primaryKey"`
	OrgID     string `gorm:"index"`
	UserID    string `gorm:"index"`
	Endpoint  string `gorm:"uniqueIndex"`
	P256dh    string
	Auth      string
	CreatedAt time.Time
}

// SavePushSubscription upserts a subscription by endpoint (idempotent — the same
// browser re-subscribing refreshes its keys/owner).
func (s *Store) SavePushSubscription(sub PushSubscription) error {
	var existing PushSubscription
	if err := s.db.Where("endpoint = ?", sub.Endpoint).First(&existing).Error; err == nil {
		return s.db.Model(&existing).Updates(map[string]any{
			"org_id": sub.OrgID, "user_id": sub.UserID,
			"p256dh": sub.P256dh, "auth": sub.Auth,
		}).Error
	}
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	return s.db.Create(&sub).Error
}

// ListPushSubscriptionsForOrg returns all subscriptions in an org.
func (s *Store) ListPushSubscriptionsForOrg(orgID string) ([]PushSubscription, error) {
	var subs []PushSubscription
	return subs, s.db.Where("org_id = ?", orgID).Find(&subs).Error
}

// ListPushSubscriptionsForUser returns a user's subscriptions.
func (s *Store) ListPushSubscriptionsForUser(userID string) ([]PushSubscription, error) {
	var subs []PushSubscription
	return subs, s.db.Where("user_id = ?", userID).Find(&subs).Error
}

// DeletePushSubscription removes a subscription by endpoint. Idempotent.
func (s *Store) DeletePushSubscription(endpoint string) error {
	return s.db.Where("endpoint = ?", endpoint).Delete(&PushSubscription{}).Error
}
