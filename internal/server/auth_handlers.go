package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/avi-pathak/mission-control.ai/internal/auth"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

// bootstrapAdmin creates a default org + owner from ADMIN_EMAIL/ADMIN_PASSWORD
// when the database has no users yet. Also backfills legacy rows into that org.
func (s *Server) bootstrapAdmin() {
	if s.cfg.AdminEmail == "" || s.cfg.AdminPass == "" {
		return
	}
	// If the admin user already exists (e.g. created before the approval flow),
	// ensure it is a platform superadmin and active, then stop.
	if existing, err := s.store.UserByEmail(strings.ToLower(s.cfg.AdminEmail)); err == nil {
		if !existing.PlatformAdmin {
			_ = s.store.SetPlatformAdmin(existing.ID, true)
			s.log.Info("promoted existing admin to platform superadmin", zapStr("email", existing.Email))
		}
		return
	}
	n, err := s.store.CountUsers()
	if err != nil || n > 0 {
		// Users exist but none match ADMIN_EMAIL: still create the platform admin
		// below so approvals are possible.
	}
	now := time.Now()
	// Reuse an existing "default" org if present (idempotent across restarts).
	org, err := s.store.OrgBySlug("default")
	if err != nil {
		org, err = s.store.CreateOrg("Default", "default", now)
		if err != nil {
			s.log.Error("bootstrap org", zapErr(err))
			return
		}
	}
	hash, err := auth.HashPassword(s.cfg.AdminPass)
	if err != nil {
		s.log.Error("bootstrap hash", zapErr(err))
		return
	}
	if _, err := s.store.CreateUser(store.User{
		OrgID: org.ID, Email: s.cfg.AdminEmail, PasswordHash: hash, Role: auth.RoleAdmin,
		Status: store.UserStatusActive, PlatformAdmin: true,
	}); err != nil {
		s.log.Error("bootstrap user", zapErr(err))
		return
	}
	// Adopt any pre-tenancy rows into the default org.
	_ = s.store.BackfillOrg(org.ID)
	s.reloadAgentKeys()
	s.log.Info("bootstrapped admin user", zapStr("email", s.cfg.AdminEmail))
}

type registerReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"orgName"`
}

type authResp struct {
	Token string   `json:"token"`
	User  userView `json:"user"`
}

type userView struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	OrgID         string `json:"orgId"`
	PlatformAdmin bool   `json:"platformAdmin,omitempty"`
	Status        string `json:"status,omitempty"`
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// handleRegister creates a PENDING account. New signups get no org and no
// access until a platform admin approves them and assigns a namespace. This
// prevents the "empty workspace" confusion where a self-created org has no
// agents. No token is issued here.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Password = strings.TrimSpace(req.Password)
	if !validEmail(req.Email) {
		writeError(w, http.StatusBadRequest, "bad_email", "enter a valid email address")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "bad_request", "password must be 8+ characters")
		return
	}
	if _, err := s.store.UserByEmail(req.Email); err == nil {
		writeError(w, http.StatusConflict, "email_taken", "an account with that email exists")
		return
	}
	hash, _ := auth.HashPassword(req.Password)
	if _, err := s.store.CreateUser(store.User{
		Email: req.Email, PasswordHash: hash, Role: auth.RoleMember,
		Status: store.UserStatusPending,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	// 202: accepted but not yet active. Client shows a "pending approval" screen.
	writeJSON(w, http.StatusAccepted, map[string]any{"status": store.UserStatusPending})
}

// validEmail is a pragmatic email check (server-side mirror of the client rule).
func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	dot := strings.LastIndexByte(s, '.')
	return dot > at+1 && dot < len(s)-1 && !strings.ContainsAny(s, " \t")
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	// Trim surrounding whitespace (autofill/paste often add a leading space) and
	// normalize email case — mirrors registration so credentials round-trip.
	email := strings.TrimSpace(strings.ToLower(req.Email))
	password := strings.TrimSpace(req.Password)
	user, err := s.store.UserByEmail(email)
	if err != nil || !auth.CheckPassword(user.PasswordHash, password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}
	if user.Status == store.UserStatusPending {
		writeError(w, http.StatusForbidden, "pending_approval",
			"your account is awaiting admin approval")
		return
	}
	s.issueAuth(w, user)
}

type acceptInviteReq struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	inv, err := s.store.GetInvite(req.Token)
	if err != nil || inv.AcceptedAt != nil || time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid_invite", "invite is invalid or expired")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "bad_request", "8+ char password required")
		return
	}
	hash, _ := auth.HashPassword(req.Password)
	user, err := s.store.CreateUser(store.User{
		OrgID: inv.OrgID, Email: inv.Email, PasswordHash: hash, Role: inv.Role,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "email_taken", "account already exists")
		return
	}
	_ = s.store.AcceptInvite(inv.Token, time.Now())
	s.issueAuth(w, user)
}

func (s *Server) issueAuth(w http.ResponseWriter, u store.User) {
	tok, err := s.auth.IssueWithAdmin(u.ID, u.OrgID, u.Role, u.Email, u.PlatformAdmin, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authResp{
		Token: tok,
		User:  userView{ID: u.ID, Email: u.Email, Role: u.Role, OrgID: u.OrgID, PlatformAdmin: u.PlatformAdmin},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	org, _ := s.store.GetOrg(c.OrgID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userView{ID: c.UserID, Email: c.Email, Role: c.Role, OrgID: c.OrgID, PlatformAdmin: c.PlatformAdmin},
		"org":  map[string]string{"id": org.ID, "name": org.Name, "slug": org.Slug},
	})
}

type changePasswordReq struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// handleChangePassword lets a signed-in user change their own password after
// re-verifying their current one.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	var req changePasswordReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if len(strings.TrimSpace(req.NewPassword)) < 8 {
		writeError(w, http.StatusBadRequest, "bad_request", "new password must be 8+ characters")
		return
	}
	u, err := s.store.UserByID(c.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "user not found")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "wrong_password", "current password is incorrect")
		return
	}
	hash, _ := auth.HashPassword(strings.TrimSpace(req.NewPassword))
	if err := s.store.UpdatePassword(c.UserID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Org administration ---

func (s *Server) handleListOrgUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListOrgUsers(orgID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]userView, len(users))
	for i, u := range users {
		out[i] = userView{ID: u.ID, Email: u.Email, Role: u.Role, OrgID: u.OrgID}
	}
	writeJSON(w, http.StatusOK, out)
}

type setRoleReq struct {
	Role string `json:"role"`
}

// handleSetUserRole changes a member's role (admin only). An admin cannot demote
// themselves (avoids locking a workspace out of admin).
func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	if !auth.IsAdmin(c.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can change roles")
		return
	}
	targetID := chi.URLParam(r, "id")
	if targetID == c.UserID {
		writeError(w, http.StatusBadRequest, "bad_request", "you cannot change your own role")
		return
	}
	var req setRoleReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if err := s.store.SetUserRole(c.OrgID, targetID, auth.NormalizeRole(req.Role)); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "no such member")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveUser removes a member from the workspace (admin only). An admin
// cannot remove themselves.
func (s *Server) handleRemoveUser(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	if !auth.IsAdmin(c.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can remove members")
		return
	}
	targetID := chi.URLParam(r, "id")
	if targetID == c.UserID {
		writeError(w, http.StatusBadRequest, "bad_request", "you cannot remove yourself")
		return
	}
	if err := s.store.RemoveUser(c.OrgID, targetID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "no such member")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createInviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	if !auth.IsAdmin(c.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "only admins can invite")
		return
	}
	var req createInviteReq
	if err := decodeJSON(r, &req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email required")
		return
	}
	role := auth.NormalizeRole(req.Role)
	inv, err := s.store.CreateInvite(c.OrgID, strings.ToLower(req.Email), role, c.UserID, 7*24*time.Hour, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	link := s.publicBase(r) + "/accept-invite?token=" + inv.Token

	// If SMTP is configured, email the invite; otherwise the link is returned
	// below and the admin shares it manually.
	emailed := false
	if s.email.Enabled() {
		org, _ := s.store.GetOrg(c.OrgID)
		body := inviteEmailHTML(org.Name, link)
		if err := s.email.Send(inv.Email, "You're invited to "+org.Name+" on Mission Control", body); err != nil {
			s.log.Warn("invite email failed", zapErr(err))
		} else {
			emailed = true
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     inv.Token,
		"email":     inv.Email,
		"role":      inv.Role,
		"expiresAt": inv.ExpiresAt.UnixMilli(),
		"link":      link,
		"emailed":   emailed,
	})
}

// inviteEmailHTML renders a minimal invite email body.
func inviteEmailHTML(orgName, link string) string {
	return `<div style="font-family:sans-serif;line-height:1.5">` +
		`<h2>You've been invited to ` + orgName + `</h2>` +
		`<p>You've been invited to join the <strong>` + orgName + `</strong> workspace on Mission Control.</p>` +
		`<p><a href="` + link + `" style="background:#4f46e5;color:#fff;padding:10px 16px;border-radius:8px;text-decoration:none;display:inline-block">Accept invite</a></p>` +
		`<p style="color:#666;font-size:13px">Or paste this link into your browser:<br>` + link + `</p>` +
		`</div>`
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	invs, err := s.store.ListInvites(orgID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]map[string]any, len(invs))
	for i, inv := range invs {
		out[i] = map[string]any{
			"token": inv.Token, "email": inv.Email, "role": inv.Role,
			"expiresAt": inv.ExpiresAt.UnixMilli(),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeInvite(orgID(r), chi.URLParam(r, "token")); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
