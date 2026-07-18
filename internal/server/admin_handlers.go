package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/avi-pathak/mission-control.ai/internal/auth"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

// pendingUserView is a signup awaiting approval.
type pendingUserView struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	CreatedAt int64  `json:"createdAt"`
}

// handleListPendingUsers returns every user awaiting approval (all orgs).
func (s *Server) handleListPendingUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListPendingUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]pendingUserView, len(users))
	for i, u := range users {
		out[i] = pendingUserView{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt.UnixMilli()}
	}
	writeJSON(w, http.StatusOK, out)
}

// orgView is a namespace an admin can assign a user to.
type orgView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleListOrgs returns all orgs so an admin can pick an assignment target.
func (s *Server) handleAdminListOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.store.ListOrgs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]orgView, len(orgs))
	for i, o := range orgs {
		out[i] = orgView{ID: o.ID, Name: o.Name, Slug: o.Slug}
	}
	writeJSON(w, http.StatusOK, out)
}

type approveUserReq struct {
	OrgID      string `json:"orgId"`      // assign to an existing org, OR
	NewOrgName string `json:"newOrgName"` // create a new org with this name
	Role       string `json:"role"`       // owner | admin | member
}

// handleApproveUser activates a pending user and assigns them to a namespace:
// either an existing org (OrgID) or a freshly created one (NewOrgName).
func (s *Server) handleApproveUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var req approveUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	role := auth.NormalizeRole(req.Role)

	now := time.Now()
	orgID := strings.TrimSpace(req.OrgID)
	if orgID == "" {
		name := strings.TrimSpace(req.NewOrgName)
		if name == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "orgId or newOrgName required")
			return
		}
		org, err := s.store.CreateOrg(name, slugify(name)+"-"+shortID(), now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db", err.Error())
			return
		}
		orgID = org.ID
	} else if _, err := s.store.GetOrg(orgID); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown orgId")
		return
	}

	if err := s.store.ApproveUser(userID, orgID, role, now); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "no pending user with that id")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": store.UserStatusActive, "orgId": orgID, "role": role})
}

// handleRejectUser denies and removes a pending signup.
func (s *Server) handleRejectUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RejectUser(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminMachineView is a machine plus its workspace + live status.
type adminMachineView struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OrgID    string `json:"orgId"`
	OrgName  string `json:"orgName"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Online   bool   `json:"online"`
}

// handleAdminListMachines returns every machine across all workspaces.
func (s *Server) handleAdminListMachines(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAllMachines()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]adminMachineView, len(rows))
	for i, m := range rows {
		out[i] = adminMachineView{
			ID: m.ID, Hostname: m.Hostname, OrgID: m.OrgID, OrgName: m.OrgName,
			OS: m.OS, Arch: m.Arch,
			Online: s.state.IsMachineOnline(m.OrgID, m.ID),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type reassignMachineReq struct {
	OrgID string `json:"orgId"`
}

// handleReassignMachine moves a machine to another workspace (the sanctioned
// way to relocate one; keeps tenant isolation intact).
func (s *Server) handleReassignMachine(w http.ResponseWriter, r *http.Request) {
	machineID := chi.URLParam(r, "id")
	var req reassignMachineReq
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.OrgID) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "orgId required")
		return
	}
	if err := s.store.ReassignMachine(machineID, req.OrgID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "machine or target workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	// Refresh in-memory agent-key org mapping so the agent's next messages route
	// to the new workspace.
	s.reloadAgentKeys()
	writeJSON(w, http.StatusOK, map[string]any{"id": machineID, "orgId": req.OrgID})
}
