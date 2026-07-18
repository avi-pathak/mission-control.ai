package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/store"
	"go.uber.org/zap"
)

const defaultTokenTTL = 30 * time.Minute

// publicBase returns the externally reachable base URL, preferring the
// configured PublicURL and falling back to the request's scheme+host.
func (s *Server) publicBase(r *http.Request) string {
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// enrollCommand builds the copy-paste one-liner for a token.
func enrollCommand(base, token string) string {
	return fmt.Sprintf("curl -fsSL %s/install.sh | MC_SERVER_URL=%q MC_ENROLL_TOKEN=%q sh",
		base, base, token)
}

type createTokenReq struct {
	Label      string `json:"label"`
	TTLMinutes int    `json:"ttlMinutes"`
}

func (s *Server) handleCreateEnrollToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	ttl := defaultTokenTTL
	if req.TTLMinutes > 0 {
		ttl = time.Duration(req.TTLMinutes) * time.Minute
	}
	tok, err := s.store.CreateEnrollToken(orgID(r), req.Label, ttl, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	base := s.publicBase(r)
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":     tok.Token,
		"serverUrl": base,
		"expiresAt": tok.ExpiresAt.UnixMilli(),
		"command":   enrollCommand(base, tok.Token),
	})
}

func (s *Server) handleListEnrollTokens(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListEnrollTokens(orgID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	now := time.Now()
	out := make([]map[string]any, 0, len(rows))
	for _, t := range rows {
		out = append(out, map[string]any{
			"token":     t.Token,
			"label":     t.Label,
			"createdAt": t.CreatedAt.UnixMilli(),
			"expiresAt": t.ExpiresAt.UnixMilli(),
			"status":    string(t.Status(now)),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRevokeEnrollToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := s.store.RevokeEnrollToken(orgID(r), token); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEnroll is the PUBLIC exchange: an agent trades a valid enrollment token
// for a durable, per-machine agent key. Single-use and short-TTL by design.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req protocol.EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "token required")
		return
	}

	// Stable machine id from the agent (host fingerprint). Fall back to a random
	// id for older agents that don't send one.
	machineID := strings.TrimSpace(req.MachineID)
	if machineID == "" {
		machineID = "mc-" + uuid.NewString()
	}

	// Enforce one machine = one workspace. Look up the token's org first so we
	// can tell "same workspace re-install" (allowed) from "different workspace"
	// (rejected).
	tokOrg, terr := s.store.EnrollTokenOrg(req.Token)
	if terr != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "enrollment token is invalid, expired, used, or revoked")
		return
	}
	if existing, ok := s.store.AgentKeyByMachine(machineID); ok && existing.OrgID != tokOrg {
		s.log.Warn("enroll rejected: machine already in another workspace",
			zap.String("machineId", machineID), zap.String("have", existing.OrgID), zap.String("want", tokOrg))
		writeError(w, http.StatusConflict, "machine_already_registered",
			"This machine is already registered to another workspace. Ask an admin to reassign it (Admin → Machines), then re-enroll.")
		return
	}

	key, err := s.store.ConsumeEnrollToken(req.Token, machineID, req.Hostname, time.Now())
	if err != nil {
		if err == store.ErrTokenInvalid {
			writeError(w, http.StatusUnauthorized, "invalid_token", "enrollment token is invalid, expired, used, or revoked")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	s.reloadAgentKeys()
	// Create/refresh a machine row so it's immediately visible + manageable in
	// the admin view (even before the agent's first WebSocket hello).
	_ = s.store.UpsertMachine(key.OrgID, protocol.Machine{
		ID: machineID, Hostname: req.Hostname, OS: req.OS, Arch: req.Arch,
		LastSeenAt: protocol.NowMillis(),
	})
	s.log.Info("machine enrolled", zap.String("machineId", machineID), zap.String("host", req.Hostname))

	writeJSON(w, http.StatusOK, protocol.EnrollResponse{
		AgentID:   machineID,
		AgentKey:  key.Key,
		ServerURL: s.publicBase(r),
	})
}
