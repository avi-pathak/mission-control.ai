package store

import (
	"encoding/json"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"gorm.io/gorm/clause"
)

func msToTime(ms int64) time.Time { return time.UnixMilli(ms) }

// UpsertMachine persists machine metadata from the live model (org-scoped).
func (s *Store) UpsertMachine(orgID string, m protocol.Machine) error {
	rec := Machine{
		ID:           m.ID,
		OrgID:        orgID,
		Hostname:     m.Hostname,
		OS:           m.OS,
		Arch:         m.Arch,
		CPUCores:     m.CPUCores,
		TotalMem:     m.TotalMem,
		AgentVersion: m.AgentVersion,
		LastSeenAt:   msToTime(m.LastSeenAt),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&rec).Error
}

// UpsertSession persists a session from the live model (org-scoped).
func (s *Store) UpsertSession(orgID string, sess protocol.Session) error {
	rec := Session{
		ID:             sess.ID,
		OrgID:          orgID,
		MachineID:      sess.MachineID,
		Provider:       sess.Provider,
		Status:         string(sess.Status),
		Repo:           sess.Repo,
		Branch:         sess.Branch,
		CWD:            sess.CWD,
		PID:            sess.PID,
		CurrentCommand: sess.CurrentCommand,
		Version:        sess.Version,
		TmuxSession:    sess.TmuxSession,
		StartedAt:      msToTime(sess.StartedAt),
		LastActivityAt: msToTime(sess.LastActivityAt),
	}
	if sess.Status == protocol.StatusFinished || sess.Status == protocol.StatusError {
		t := msToTime(sess.LastActivityAt)
		rec.EndedAt = &t
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&rec).Error
}

// AppendLog persists a single log line (org-scoped).
func (s *Store) AppendLog(orgID string, l protocol.LogAppend) error {
	return s.db.Create(&LogLine{
		OrgID:     orgID,
		SessionID: l.SessionID,
		Seq:       l.Seq,
		Stream:    string(l.Stream),
		Line:      l.Line,
		TS:        msToTime(l.TS),
	}).Error
}

// AppendMetric persists a single metric sample (org-scoped).
func (s *Store) AppendMetric(orgID string, m protocol.MetricSample) error {
	return s.db.Create(&MetricSample{
		OrgID:     orgID,
		SessionID: m.SessionID,
		CPUPct:    m.CPUPct,
		MemBytes:  m.MemBytes,
		TS:        msToTime(m.TS),
	}).Error
}

// LogPage returns log lines for a session after the given seq cursor (org-scoped).
func (s *Store) LogPage(orgID, sessionID string, afterSeq int64, limit int) ([]LogLine, error) {
	var out []LogLine
	q := s.db.Where("org_id = ? AND session_id = ? AND seq > ?", orgID, sessionID, afterSeq).
		Order("seq asc").Limit(limit)
	return out, q.Find(&out).Error
}

// Metrics returns metric samples for a session within the last window (org-scoped).
func (s *Store) Metrics(orgID, sessionID string, since time.Time, limit int) ([]MetricSample, error) {
	var out []MetricSample
	q := s.db.Where("org_id = ? AND session_id = ? AND ts >= ?", orgID, sessionID, since).
		Order("ts asc").Limit(limit)
	return out, q.Find(&out).Error
}

// SaveEvent persists a protocol event (idempotent on EventID, org-scoped).
func (s *Store) SaveEvent(orgID string, e protocol.Event) error {
	meta, _ := json.Marshal(e.Meta)
	rec := Event{
		OrgID:     orgID,
		EventID:   e.ID,
		MachineID: e.MachineID,
		SessionID: e.SessionID,
		Kind:      e.Kind,
		Message:   e.Message,
		Severity:  string(e.Severity),
		Meta:      string(meta),
		TS:        msToTime(e.TS),
	}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).Create(&rec).Error
}

func (e Event) toProtocol() protocol.Event {
	var meta map[string]string
	if e.Meta != "" {
		_ = json.Unmarshal([]byte(e.Meta), &meta)
	}
	return protocol.Event{
		ID:        e.EventID,
		MachineID: e.MachineID,
		SessionID: e.SessionID,
		Kind:      e.Kind,
		Message:   e.Message,
		Severity:  protocol.EventSeverity(e.Severity),
		TS:        e.TS.UnixMilli(),
		Meta:      meta,
	}
}

// EventFilter narrows an event query. Empty fields are ignored.
type EventFilter struct {
	MachineID string
	SessionID string
	Kind      string
}

// ListEvents returns events matching the filter, newest first (org-scoped).
func (s *Store) ListEvents(orgID string, f EventFilter, limit int) ([]protocol.Event, error) {
	q := s.db.Model(&Event{}).Where("org_id = ?", orgID)
	if f.MachineID != "" {
		q = q.Where("machine_id = ?", f.MachineID)
	}
	if f.SessionID != "" {
		q = q.Where("session_id = ?", f.SessionID)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	var rows []Event
	if err := q.Order("ts desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]protocol.Event, len(rows))
	for i, r := range rows {
		out[i] = r.toProtocol()
	}
	return out, nil
}

// RecentEvents returns the newest events for an org (for snapshots).
func (s *Store) RecentEvents(orgID string, limit int) ([]protocol.Event, error) {
	return s.ListEvents(orgID, EventFilter{}, limit)
}

// AllRecentEvents returns the newest events across every org (for startup
// seeding of per-org state rings).
func (s *Store) AllRecentEvents(limit int) ([]protocol.Event, []string, error) {
	var rows []Event
	if err := s.db.Model(&Event{}).Order("ts desc").Limit(limit).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	out := make([]protocol.Event, len(rows))
	orgs := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.toProtocol()
		orgs[i] = r.OrgID
	}
	return out, orgs, nil
}

// PruneEvents deletes events older than the cutoff.
func (s *Store) PruneEvents(before time.Time) (int64, error) {
	res := s.db.Where("ts < ?", before).Delete(&Event{})
	return res.RowsAffected, res.Error
}

// PruneLogs deletes log lines older than the cutoff.
func (s *Store) PruneLogs(before time.Time) (int64, error) {
	res := s.db.Where("ts < ?", before).Delete(&LogLine{})
	return res.RowsAffected, res.Error
}

// PruneMetrics deletes metric samples older than the cutoff.
func (s *Store) PruneMetrics(before time.Time) (int64, error) {
	res := s.db.Where("ts < ?", before).Delete(&MetricSample{})
	return res.RowsAffected, res.Error
}
