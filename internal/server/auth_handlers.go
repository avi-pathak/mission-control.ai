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
	n, err := s.store.CountUsers()
	if err != nil || n > 0 {
		return
	}
	now := time.Now()
	org, err := s.store.CreateOrg("Default", "default", now)
	if err != nil {
		s.log.Error("bootstrap org", zapErr(err))
		return
	}
	hash, err := auth.HashPassword(s.cfg.AdminPass)
	if err != nil {
		s.log.Error("bootstrap hash", zapErr(err))
		return
	}
	if _, err := s.store.CreateUser(store.User{
		OrgID: org.ID, Email: s.cfg.AdminEmail, PasswordHash: hash, Role: auth.RoleOwner,
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
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
	OrgID string `json:"orgId"`
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// handleRegister creates a new org and its owner (self-serve signup).
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Password = strings.TrimSpace(req.Password)
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "bad_request", "email and 8+ char password required")
		return
	}
	if _, err := s.store.UserByEmail(req.Email); err == nil {
		writeError(w, http.StatusConflict, "email_taken", "an account with that email exists")
		return
	}
	now := time.Now()
	orgName := req.OrgName
	if orgName == "" {
		orgName = req.Email
	}
	org, err := s.store.CreateOrg(orgName, slugify(orgName)+"-"+shortID(), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	hash, _ := auth.HashPassword(req.Password)
	user, err := s.store.CreateUser(store.User{
		OrgID: org.ID, Email: req.Email, PasswordHash: hash, Role: auth.RoleOwner,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	s.issueAuth(w, user)
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
	tok, err := s.auth.Issue(u.ID, u.OrgID, u.Role, u.Email, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authResp{
		Token: tok,
		User:  userView{ID: u.ID, Email: u.Email, Role: u.Role, OrgID: u.OrgID},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	org, _ := s.store.GetOrg(c.OrgID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userView{ID: c.UserID, Email: c.Email, Role: c.Role, OrgID: c.OrgID},
		"org":  map[string]string{"id": org.ID, "name": org.Name, "slug": org.Slug},
	})
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

type createInviteReq struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	if c.Role != auth.RoleOwner && c.Role != auth.RoleAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "only owners/admins can invite")
		return
	}
	var req createInviteReq
	if err := decodeJSON(r, &req); err != nil || req.Email == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "email required")
		return
	}
	role := req.Role
	if role != auth.RoleAdmin && role != auth.RoleMember {
		role = auth.RoleMember
	}
	inv, err := s.store.CreateInvite(c.OrgID, strings.ToLower(req.Email), role, c.UserID, 7*24*time.Hour, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     inv.Token,
		"email":     inv.Email,
		"role":      inv.Role,
		"expiresAt": inv.ExpiresAt.UnixMilli(),
		"link":      s.publicBase(r) + "/accept-invite?token=" + inv.Token,
	})
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
