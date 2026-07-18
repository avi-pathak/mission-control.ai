// Package state maintains the in-memory live index of machines and sessions,
// hydrated from connected agents and partitioned per org. It is the source of
// truth for dashboard snapshots and diffs; the store handles history.
package state

import (
	"sort"
	"sync"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

const maxRecentEvents = 200

// orgState holds one tenant's live view.
type orgState struct {
	machines map[string]protocol.Machine
	sessions map[string]protocol.Session
	events   []protocol.Event // recent-events ring (newest last)
}

func newOrgState() *orgState {
	return &orgState{
		machines: make(map[string]protocol.Machine),
		sessions: make(map[string]protocol.Session),
	}
}

// Manager is a concurrency-safe, per-org live view of the fleet.
type Manager struct {
	mu   sync.RWMutex
	orgs map[string]*orgState
}

// New creates an empty state Manager.
func New() *Manager {
	return &Manager{orgs: make(map[string]*orgState)}
}

// org returns (creating if needed) the state partition for an org. Caller holds
// the write lock.
func (m *Manager) org(orgID string) *orgState {
	os, ok := m.orgs[orgID]
	if !ok {
		os = newOrgState()
		m.orgs[orgID] = os
	}
	return os
}

// AddEvent appends an event to an org's recent-events ring.
func (m *Manager) AddEvent(orgID string, e protocol.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	os.events = append(os.events, e)
	if len(os.events) > maxRecentEvents {
		os.events = os.events[len(os.events)-maxRecentEvents:]
	}
}

// SeedEvent pushes a single historical event (newest-first callers should feed
// oldest last) into an org ring, used for startup seeding.
func (m *Manager) SeedEvent(orgID string, e protocol.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	os.events = append(os.events, e)
	if len(os.events) > maxRecentEvents {
		os.events = os.events[len(os.events)-maxRecentEvents:]
	}
}

// UpsertMachine stores or updates a machine and returns the stored copy.
func (m *Manager) UpsertMachine(orgID string, mc protocol.Machine) protocol.Machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.org(orgID).machines[mc.ID] = mc
	return mc
}

// SetMachineOnline updates connectivity for a machine.
func (m *Manager) SetMachineOnline(orgID, id string, online bool) (protocol.Machine, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	mc, ok := os.machines[id]
	if !ok {
		return protocol.Machine{}, false
	}
	mc.Online = online
	os.machines[id] = mc
	return mc, true
}

// IsMachineOnline reports whether a machine is currently connected in an org.
func (m *Manager) IsMachineOnline(orgID, id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	os, ok := m.orgs[orgID]
	if !ok {
		return false
	}
	mc, ok := os.machines[id]
	return ok && mc.Online
}

// ApplyHeartbeat updates live host metrics for a machine.
func (m *Manager) ApplyHeartbeat(orgID string, hb protocol.AgentHeartbeat, ts int64) (protocol.Machine, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	mc, ok := os.machines[hb.AgentID]
	if !ok {
		return protocol.Machine{}, false
	}
	mc.CPUPct = hb.CPUPct
	mc.MemUsedBytes = hb.MemUsedBytes
	mc.Load = hb.Load
	mc.LastSeenAt = ts
	mc.Online = true
	os.machines[hb.AgentID] = mc
	return mc, true
}

// UpsertSession stores or updates a session.
func (m *Manager) UpsertSession(orgID string, s protocol.Session) protocol.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.org(orgID).sessions[s.ID] = s
	return s
}

// RemoveSession deletes a session from an org's live index.
func (m *Manager) RemoveSession(orgID, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.org(orgID).sessions, id)
}

// RemoveMachine deletes a machine from an org's live index.
func (m *Manager) RemoveMachine(orgID, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.org(orgID).machines, id)
}

// ApplyMetric updates cached per-session resource usage.
func (m *Manager) ApplyMetric(orgID string, ms protocol.MetricSample) (protocol.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	s, ok := os.sessions[ms.SessionID]
	if !ok {
		return protocol.Session{}, false
	}
	s.CPUPct = ms.CPUPct
	s.MemBytes = ms.MemBytes
	s.LastActivityAt = ms.TS
	os.sessions[ms.SessionID] = s
	return s, true
}

// RemoveMachineSessions removes all of an org's sessions for a machine and
// returns their IDs (used when an agent disconnects).
func (m *Manager) RemoveMachineSessions(orgID, machineID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	os := m.org(orgID)
	var removed []string
	for id, s := range os.sessions {
		if s.MachineID == machineID {
			removed = append(removed, id)
			delete(os.sessions, id)
		}
	}
	return removed
}

// Session returns a session by id within an org.
func (m *Manager) Session(orgID, id string) (protocol.Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	os, ok := m.orgs[orgID]
	if !ok {
		return protocol.Session{}, false
	}
	s, ok := os.sessions[id]
	return s, ok
}

// OrgSessions is a snapshot of one org's sessions, used by background scans.
type OrgSessions struct {
	OrgID    string
	Sessions []protocol.Session
}

// AllSessions returns a snapshot of every org's sessions (copied out under the
// read lock). Used by the blocked-session notifier.
func (m *Manager) AllSessions() []OrgSessions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OrgSessions, 0, len(m.orgs))
	for orgID, os := range m.orgs {
		if len(os.sessions) == 0 {
			continue
		}
		sessions := make([]protocol.Session, 0, len(os.sessions))
		for _, s := range os.sessions {
			sessions = append(sessions, s)
		}
		out = append(out, OrgSessions{OrgID: orgID, Sessions: sessions})
	}
	return out
}

// Snapshot returns a stable, sorted copy of an org's live state.
func (m *Manager) Snapshot(orgID string) protocol.Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snap := protocol.Snapshot{Machines: []protocol.Machine{}, Sessions: []protocol.Session{}, Events: []protocol.Event{}}
	os, ok := m.orgs[orgID]
	if !ok {
		return snap
	}
	for _, mc := range os.machines {
		snap.Machines = append(snap.Machines, mc)
	}
	for _, s := range os.sessions {
		snap.Sessions = append(snap.Sessions, s)
	}
	for i := len(os.events) - 1; i >= 0; i-- {
		snap.Events = append(snap.Events, os.events[i])
	}
	sort.Slice(snap.Machines, func(i, j int) bool { return snap.Machines[i].ID < snap.Machines[j].ID })
	sort.Slice(snap.Sessions, func(i, j int) bool { return snap.Sessions[i].StartedAt > snap.Sessions[j].StartedAt })
	return snap
}
