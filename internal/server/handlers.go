package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "ts": protocol.NowMillis()})
}

func (s *Server) handleListMachines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.state.Snapshot(orgID(r)).Machines)
}

// handleDeleteMachine removes a machine and its data from the caller's org.
// Refuses if the machine is currently online unless ?force=true.
func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	org := orgID(r)
	id := chi.URLParam(r, "id")

	// Guard against deleting a live agent by accident.
	if r.URL.Query().Get("force") != "true" {
		for _, m := range s.state.Snapshot(org).Machines {
			if m.ID == id && m.Online {
				writeError(w, http.StatusConflict, "machine_online",
					"machine is online; stop the agent first or pass force=true")
				return
			}
		}
	}

	if err := s.store.DeleteMachine(org, id); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "not_found", "machine not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}

	// Drop from live state + broadcast removals to the org's dashboards.
	for _, sid := range s.state.RemoveMachineSessions(org, id) {
		s.hub.EmitOrg(org, protocol.TypeSessionRemoved, protocol.SessionRemoved{SessionID: sid})
	}
	s.state.RemoveMachine(org, id)
	s.hub.EmitOrg(org, protocol.TypeMachineRemoved, protocol.MachineRemoved{MachineID: id})
	// Revoke the machine's keys in memory so a returning agent can't reappear
	// under the old identity without re-enrolling.
	_ = s.store.RevokeAgentKeysForMachine(id)
	s.reloadAgentKeys()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.state.Snapshot(orgID(r)).Sessions
	// Optional filters.
	q := r.URL.Query()
	status := q.Get("status")
	machine := q.Get("machine")
	repo := q.Get("repo")
	out := sessions[:0:0]
	for _, sess := range sessions {
		if status != "" && string(sess.Status) != status {
			continue
		}
		if machine != "" && sess.MachineID != machine {
			continue
		}
		if repo != "" && sess.Repo != repo {
			continue
		}
		out = append(out, sess)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, ok := s.state.Session(orgID(r), id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) sendCommand(w http.ResponseWriter, r *http.Request, action protocol.CommandAction) {
	org := orgID(r)
	id := chi.URLParam(r, "id")
	sess, ok := s.state.Session(org, id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	cmd := protocol.Command{RequestID: uuid.NewString(), SessionID: id, Action: action}
	data, err := protocol.Encode(protocol.TypeCommand, cmd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode", err.Error())
		return
	}
	if !s.hub.SendToAgent(sess.MachineID, data) {
		writeError(w, http.StatusServiceUnavailable, "agent_offline", "owning agent is not connected")
		return
	}
	s.emitServerEvent(org, sess.MachineID, id, protocol.EventCommandIssued,
		string(action)+" requested · "+sess.Repo, protocol.SeverityInfo)
	writeJSON(w, http.StatusAccepted, map[string]any{"requestId": cmd.RequestID, "action": action})
}

func (s *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	s.sendCommand(w, r, protocol.ActionStop)
}

func (s *Server) handleRestartSession(w http.ResponseWriter, r *http.Request) {
	s.sendCommand(w, r, protocol.ActionRestart)
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit := 500
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 2000 {
		limit = l
	}
	lines, err := s.store.LogPage(orgID(r), id, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	// Map to the wire LogAppend shape (lowercase, ts in millis) so the client
	// uses the same type for REST backfill and live WS messages.
	out := make([]protocol.LogAppend, 0, len(lines))
	for _, l := range lines {
		out = append(out, protocol.LogAppend{
			SessionID: l.SessionID,
			Seq:       l.Seq,
			Stream:    protocol.LogStream(l.Stream),
			Line:      l.Line,
			TS:        l.TS.UnixMilli(),
		})
	}
	var nextCursor int64
	if len(lines) > 0 {
		nextCursor = lines[len(lines)-1].Seq
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out, "nextCursor": nextCursor})
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	windowMin := 60
	if m, err := strconv.Atoi(r.URL.Query().Get("windowMinutes")); err == nil && m > 0 && m <= 1440 {
		windowMin = m
	}
	since := time.Now().Add(-time.Duration(windowMin) * time.Minute)
	samples, err := s.store.Metrics(orgID(r), id, since, 2000)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	out := make([]protocol.MetricSample, 0, len(samples))
	for _, m := range samples {
		out = append(out, protocol.MetricSample{
			SessionID: m.SessionID,
			CPUPct:    m.CPUPct,
			MemBytes:  m.MemBytes,
			TS:        m.TS.UnixMilli(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
