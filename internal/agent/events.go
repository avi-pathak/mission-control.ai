package agent

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// emitEvent stamps and sends an activity event to the server.
func (r *Runtime) emitEvent(kind, sessionID, message string, sev protocol.EventSeverity, meta map[string]string) {
	ev := protocol.Event{
		ID:        uuid.NewString(),
		MachineID: r.cfg.AgentID,
		SessionID: sessionID,
		Kind:      kind,
		Message:   message,
		Severity:  sev,
		TS:        protocol.NowMillis(),
		Meta:      meta,
	}
	_ = r.ws.send(protocol.TypeEventReport, protocol.EventReport{Event: ev})
}

// label is a short human name for a session used in event messages.
func sessionLabel(s protocol.Session) string {
	if s.Repo != "" {
		if s.Branch != "" {
			return fmt.Sprintf("%s (%s)", s.Repo, s.Branch)
		}
		return s.Repo
	}
	if s.CWD != "" {
		return s.CWD
	}
	return s.ID
}

// severityForStatus maps a session status to an event severity.
func severityForStatus(status protocol.SessionStatus) protocol.EventSeverity {
	switch status {
	case protocol.StatusError:
		return protocol.SeverityError
	case protocol.StatusWaitingApproval:
		return protocol.SeverityWarn
	case protocol.StatusFinished:
		return protocol.SeveritySuccess
	default:
		return protocol.SeverityInfo
	}
}

// detectSessionEvents compares a freshly discovered session against the last
// known copy and emits an event for each meaningful transition. `prev` is the
// zero value when the session is newly discovered (existed == false).
func (r *Runtime) detectSessionEvents(prev protocol.Session, cur protocol.Session, existed bool) {
	for _, ev := range diffSessionEvents(prev, cur, existed) {
		r.emitEvent(ev.Kind, ev.SessionID, ev.Message, ev.Severity, ev.Meta)
	}
}

// diffSessionEvents is the pure detection logic: given the previous and current
// session (and whether it already existed), return the events to emit. It does
// not stamp id/ts/machineId — emitEvent does that.
func diffSessionEvents(prev, cur protocol.Session, existed bool) []protocol.Event {
	label := sessionLabel(cur)
	var out []protocol.Event
	add := func(kind, msg string, sev protocol.EventSeverity, meta map[string]string) {
		out = append(out, protocol.Event{Kind: kind, SessionID: cur.ID, Message: msg, Severity: sev, Meta: meta})
	}

	if !existed {
		add(protocol.EventSessionStarted,
			fmt.Sprintf("Session started · %s", label),
			protocol.SeveritySuccess,
			map[string]string{"repo": cur.Repo, "branch": cur.Branch, "pid": fmt.Sprint(cur.PID)})
		return out
	}

	if prev.Status != cur.Status {
		add(protocol.EventStatusChanged,
			fmt.Sprintf("%s → %s · %s", prev.Status, cur.Status, label),
			severityForStatus(cur.Status),
			map[string]string{"from": string(prev.Status), "to": string(cur.Status)})
	}

	if prev.Branch != cur.Branch && cur.Branch != "" {
		add(protocol.EventBranchChanged,
			fmt.Sprintf("Switched branch %s → %s · %s", orNone(prev.Branch), cur.Branch, cur.Repo),
			protocol.SeverityInfo,
			map[string]string{"from": prev.Branch, "to": cur.Branch})
	}

	if h := newestCommit(cur); h != "" && h != newestCommit(prev) {
		add(protocol.EventCommitCreated,
			fmt.Sprintf("New commit on %s · %s", cur.Branch, commitSubject(cur)),
			protocol.SeveritySuccess,
			map[string]string{"hash": h, "branch": cur.Branch})
	}
	return out
}

func newestCommit(s protocol.Session) string {
	if s.Git == nil || len(s.Git.RecentCommits) == 0 {
		return ""
	}
	return s.Git.RecentCommits[0].Hash
}

func commitSubject(s protocol.Session) string {
	if s.Git == nil || len(s.Git.RecentCommits) == 0 {
		return ""
	}
	return s.Git.RecentCommits[0].Message
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
