package server

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

// recordEvent persists an event, adds it to the org's live ring, and broadcasts
// it to that org's dashboards.
func (s *Server) recordEvent(org string, e protocol.Event) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.TS == 0 {
		e.TS = protocol.NowMillis()
	}
	if e.Severity == "" {
		e.Severity = protocol.SeverityInfo
	}
	_ = s.store.SaveEvent(org, e)
	s.state.AddEvent(org, e)
	s.hub.EmitOrg(org, protocol.TypeEventAppend, protocol.EventAppend{Event: e})
}

// emitServerEvent is a convenience for server-generated activities.
func (s *Server) emitServerEvent(org, machineID, sessionID, kind, message string, sev protocol.EventSeverity) {
	s.recordEvent(org, protocol.Event{
		MachineID: machineID,
		SessionID: sessionID,
		Kind:      kind,
		Message:   message,
		Severity:  sev,
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 200
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	events, err := s.store.ListEvents(orgID(r), store.EventFilter{
		MachineID: q.Get("machine"),
		SessionID: q.Get("session"),
		Kind:      q.Get("kind"),
	}, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}
