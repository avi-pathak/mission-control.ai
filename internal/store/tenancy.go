package store

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// --- Orgs ---

// CreateOrg creates a new org with a generated id.
func (s *Store) CreateOrg(name, slug string, now time.Time) (Org, error) {
	org := Org{ID: uuid.NewString(), Name: name, Slug: slug, CreatedAt: now}
	if err := s.db.Create(&org).Error; err != nil {
		return Org{}, err
	}
	return org, nil
}

// GetOrg fetches an org by id.
func (s *Store) GetOrg(id string) (Org, error) {
	var org Org
	if err := s.db.Where("id = ?", id).First(&org).Error; err != nil {
		return Org{}, ErrNotFound
	}
	return org, nil
}

// CountUsers returns the total number of users (used for bootstrap detection).
func (s *Store) CountUsers() (int64, error) {
	var n int64
	return n, s.db.Model(&User{}).Count(&n).Error
}

// DeleteMachine removes a machine and all its dependent rows within an org
// (sessions, logs, metrics, events, agent keys, published files). Scoped to the
// org so one tenant can never delete another's data. Returns ErrNotFound if the
// machine doesn't exist in the org.
func (s *Store) DeleteMachine(orgID, machineID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var m Machine
		if err := tx.Where("org_id = ? AND id = ?", orgID, machineID).First(&m).Error; err != nil {
			return ErrNotFound
		}
		// Sessions on this machine (needed to cascade session-scoped rows).
		var sessionIDs []string
		tx.Model(&Session{}).Where("org_id = ? AND machine_id = ?", orgID, machineID).
			Pluck("id", &sessionIDs)

		if len(sessionIDs) > 0 {
			tx.Where("org_id = ? AND session_id IN ?", orgID, sessionIDs).Delete(&LogLine{})
			tx.Where("org_id = ? AND session_id IN ?", orgID, sessionIDs).Delete(&MetricSample{})
		}
		tx.Where("org_id = ? AND machine_id = ?", orgID, machineID).Delete(&Session{})
		tx.Where("org_id = ? AND machine_id = ?", orgID, machineID).Delete(&Event{})
		tx.Where("org_id = ? AND machine_id = ?", orgID, machineID).Delete(&PublishedFile{})
		tx.Where("org_id = ? AND machine_id = ?", orgID, machineID).Delete(&AgentKey{})
		return tx.Where("org_id = ? AND id = ?", orgID, machineID).Delete(&Machine{}).Error
	})
}

// BackfillOrg assigns a default org to any legacy rows that predate multi-
// tenancy (org_id == ""). Idempotent; safe to run on every boot.
func (s *Store) BackfillOrg(orgID string) error {
	models := []any{&Machine{}, &Session{}, &LogLine{}, &MetricSample{}, &Event{}, &EnrollmentToken{}, &AgentKey{}, &PublishedFile{}}
	for _, m := range models {
		if err := s.db.Model(m).Where("org_id = ? OR org_id IS NULL", "").Update("org_id", orgID).Error; err != nil {
			return err
		}
	}
	return nil
}

// --- Users ---

// CreateUser inserts a user.
func (s *Store) CreateUser(u User) (User, error) {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	if err := s.db.Create(&u).Error; err != nil {
		return User{}, err
	}
	return u, nil
}

// UserByEmail fetches a user by email (case-insensitive).
func (s *Store) UserByEmail(email string) (User, error) {
	var u User
	if err := s.db.Where("lower(email) = lower(?)", email).First(&u).Error; err != nil {
		return User{}, ErrNotFound
	}
	return u, nil
}

// UserByID fetches a user by id.
func (s *Store) UserByID(id string) (User, error) {
	var u User
	if err := s.db.Where("id = ?", id).First(&u).Error; err != nil {
		return User{}, ErrNotFound
	}
	return u, nil
}

// ListOrgUsers returns all users in an org.
func (s *Store) ListOrgUsers(orgID string) ([]User, error) {
	var users []User
	return users, s.db.Where("org_id = ?", orgID).Order("created_at asc").Find(&users).Error
}

// --- Invites ---

// CreateInvite mints an invite token for an email + role in an org.
func (s *Store) CreateInvite(orgID, email, role, invitedBy string, ttl time.Duration, now time.Time) (Invite, error) {
	tok, err := randToken(32)
	if err != nil {
		return Invite{}, err
	}
	inv := Invite{
		Token:     tok,
		OrgID:     orgID,
		Email:     email,
		Role:      role,
		InvitedBy: invitedBy,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	if err := s.db.Create(&inv).Error; err != nil {
		return Invite{}, err
	}
	return inv, nil
}

// GetInvite fetches an invite by token.
func (s *Store) GetInvite(token string) (Invite, error) {
	var inv Invite
	if err := s.db.Where("token = ?", token).First(&inv).Error; err != nil {
		return Invite{}, ErrNotFound
	}
	return inv, nil
}

// ListInvites returns pending invites for an org.
func (s *Store) ListInvites(orgID string) ([]Invite, error) {
	var invs []Invite
	return invs, s.db.Where("org_id = ? AND accepted_at IS NULL", orgID).
		Order("created_at desc").Find(&invs).Error
}

// AcceptInvite marks an invite accepted (idempotent guard in the caller).
func (s *Store) AcceptInvite(token string, now time.Time) error {
	return s.db.Model(&Invite{}).Where("token = ?", token).
		Update("accepted_at", now).Error
}

// RevokeInvite deletes a pending invite.
func (s *Store) RevokeInvite(orgID, token string) error {
	return s.db.Where("org_id = ? AND token = ?", orgID, token).Delete(&Invite{}).Error
}

// --- Published files ---

// SaveFile persists an uploaded file blob.
func (s *Store) SaveFile(f PublishedFile) (PublishedFile, error) {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	if err := s.db.Create(&f).Error; err != nil {
		return PublishedFile{}, err
	}
	return f, nil
}

// FileMeta is file metadata without the blob payload.
type FileMeta struct {
	ID          string    `json:"id"`
	MachineID   string    `json:"machineId"`
	SessionID   string    `json:"sessionId"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ListFiles returns file metadata (no blobs) for an org, optionally filtered.
func (s *Store) ListFiles(orgID, sessionID, machineID string, limit int) ([]FileMeta, error) {
	q := s.db.Model(&PublishedFile{}).Where("org_id = ?", orgID)
	if sessionID != "" {
		q = q.Where("session_id = ?", sessionID)
	}
	if machineID != "" {
		q = q.Where("machine_id = ?", machineID)
	}
	var out []FileMeta
	return out, q.Order("created_at desc").Limit(limit).Find(&out).Error
}

// GetFile fetches a file (with blob) scoped to an org.
func (s *Store) GetFile(orgID, id string) (PublishedFile, error) {
	var f PublishedFile
	if err := s.db.Where("org_id = ? AND id = ?", orgID, id).First(&f).Error; err != nil {
		return PublishedFile{}, ErrNotFound
	}
	return f, nil
}

// FileMeta is derived from PublishedFile for GORM's model resolution.
func (PublishedFile) TableName() string { return "published_files" }
