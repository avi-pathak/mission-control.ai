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

// OrgBySlug fetches an org by slug.
func (s *Store) OrgBySlug(slug string) (Org, error) {
	var org Org
	if err := s.db.Where("slug = ?", slug).First(&org).Error; err != nil {
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

// UpdatePassword sets a user's password hash.
func (s *Store) UpdatePassword(userID, newHash string) error {
	res := s.db.Model(&User{}).Where("id = ?", userID).Update("password_hash", newHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserRole changes a user's role within an org.
func (s *Store) SetUserRole(orgID, userID, role string) error {
	res := s.db.Model(&User{}).Where("org_id = ? AND id = ?", orgID, userID).
		Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveUser deletes a user from an org.
func (s *Store) RemoveUser(orgID, userID string) error {
	res := s.db.Where("org_id = ? AND id = ?", orgID, userID).Delete(&User{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOrgUsers returns all users in an org.
func (s *Store) ListOrgUsers(orgID string) ([]User, error) {
	var users []User
	return users, s.db.Where("org_id = ?", orgID).Order("created_at asc").Find(&users).Error
}

// ListPendingUsers returns all users awaiting approval, across every org.
// Platform-admin scoped.
func (s *Store) ListPendingUsers() ([]User, error) {
	var users []User
	return users, s.db.Where("status = ?", UserStatusPending).
		Order("created_at asc").Find(&users).Error
}

// CountPendingUsers returns how many users await approval.
func (s *Store) CountPendingUsers() (int64, error) {
	var n int64
	return n, s.db.Model(&User{}).Where("status = ?", UserStatusPending).Count(&n).Error
}

// ApproveUser activates a pending user, assigning them to an org with a role.
func (s *Store) ApproveUser(userID, orgID, role string, now time.Time) error {
	res := s.db.Model(&User{}).Where("id = ? AND status = ?", userID, UserStatusPending).
		Updates(map[string]any{
			"org_id": orgID,
			"role":   role,
			"status": UserStatusActive,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RejectUser removes a pending signup.
func (s *Store) RejectUser(userID string) error {
	return s.db.Where("id = ? AND status = ?", userID, UserStatusPending).Delete(&User{}).Error
}

// ListOrgs returns all orgs (platform-admin scoped; used to pick an assignment).
func (s *Store) ListOrgs() ([]Org, error) {
	var orgs []Org
	return orgs, s.db.Order("created_at asc").Find(&orgs).Error
}

// MachineWithOrg is a machine plus its owning org's name (superadmin view).
type MachineWithOrg struct {
	ID         string    `json:"id"`
	Hostname   string    `json:"hostname"`
	OrgID      string    `json:"orgId"`
	OrgName    string    `json:"orgName"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	LastSeenAt time.Time `json:"-"`
}

// ListAllMachines returns every machine across all workspaces with its org
// name. Platform-admin scoped.
func (s *Store) ListAllMachines() ([]MachineWithOrg, error) {
	var out []MachineWithOrg
	err := s.db.Model(&Machine{}).
		Select("machines.id, machines.hostname, machines.org_id, machines.os, machines.arch, machines.last_seen_at, orgs.name as org_name").
		Joins("left join orgs on orgs.id = machines.org_id").
		Order("machines.hostname asc").
		Scan(&out).Error
	return out, err
}

// ReassignMachine moves a machine and all its dependent rows (agent keys,
// sessions, logs, metrics, events, published files) to a new org, in one
// transaction. The sanctioned way to relocate a machine between workspaces.
func (s *Store) ReassignMachine(machineID, newOrgID string) error {
	if _, err := s.GetOrg(newOrgID); err != nil {
		return ErrNotFound
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var m Machine
		if err := tx.Where("id = ?", machineID).First(&m).Error; err != nil {
			return ErrNotFound
		}
		// Session ids on this machine (to cascade session-scoped rows).
		var sessionIDs []string
		tx.Model(&Session{}).Where("machine_id = ?", machineID).Pluck("id", &sessionIDs)
		if len(sessionIDs) > 0 {
			tx.Model(&LogLine{}).Where("session_id IN ?", sessionIDs).Update("org_id", newOrgID)
			tx.Model(&MetricSample{}).Where("session_id IN ?", sessionIDs).Update("org_id", newOrgID)
		}
		tx.Model(&Session{}).Where("machine_id = ?", machineID).Update("org_id", newOrgID)
		tx.Model(&Event{}).Where("machine_id = ?", machineID).Update("org_id", newOrgID)
		tx.Model(&PublishedFile{}).Where("machine_id = ?", machineID).Update("org_id", newOrgID)
		tx.Model(&AgentKey{}).Where("machine_id = ?", machineID).Update("org_id", newOrgID)
		return tx.Model(&Machine{}).Where("id = ?", machineID).Update("org_id", newOrgID).Error
	})
}

// SetPlatformAdmin marks a user as (or not) a platform superadmin.
func (s *Store) SetPlatformAdmin(userID string, isAdmin bool) error {
	return s.db.Model(&User{}).Where("id = ?", userID).
		Update("platform_admin", isAdmin).Error
}

// ActivateLegacyUsers backfills any user with an empty status to active (they
// predate the approval flow). Idempotent; safe on every boot.
func (s *Store) ActivateLegacyUsers() error {
	return s.db.Model(&User{}).Where("status = ? OR status IS NULL", "").
		Update("status", UserStatusActive).Error
}

// MigrateOwnersToAdmin collapses the legacy "owner" role into "admin" (two-tier
// role model). Idempotent; safe on every boot.
func (s *Store) MigrateOwnersToAdmin() error {
	return s.db.Model(&User{}).Where("role = ?", "owner").
		Update("role", "admin").Error
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

// TableName pins PublishedFile to the published_files table for GORM.
func (PublishedFile) TableName() string { return "published_files" }
