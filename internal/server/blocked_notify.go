package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/push"
	"go.uber.org/zap"
)

// blockedTracker remembers, per session, when it entered waiting_approval and
// whether we've already notified for the current blocked episode. Pure state —
// no locking needed because it's only touched from the single notify loop.
type blockedTracker struct {
	since    map[string]time.Time // sessionID -> when it became blocked
	notified map[string]bool      // sessionID -> already notified this episode
}

func newBlockedTracker() *blockedTracker {
	return &blockedTracker{since: map[string]time.Time{}, notified: map[string]bool{}}
}

// reconcile updates the tracker against the current sessions and returns the
// sessions that just crossed the threshold and should be notified now. It also
// garbage-collects sessions that are gone or no longer blocked. Pure/testable:
// takes the current sessions, the threshold, and now.
func (t *blockedTracker) reconcile(sessions []protocol.Session, threshold time.Duration, now time.Time) []protocol.Session {
	live := make(map[string]bool, len(sessions))
	var toNotify []protocol.Session

	for _, s := range sessions {
		live[s.ID] = true
		if s.Status != protocol.StatusWaitingApproval {
			// Not blocked → clear any episode state so a future block re-arms.
			delete(t.since, s.ID)
			delete(t.notified, s.ID)
			continue
		}
		if _, ok := t.since[s.ID]; !ok {
			t.since[s.ID] = now
		}
		if !t.notified[s.ID] && now.Sub(t.since[s.ID]) >= threshold {
			t.notified[s.ID] = true
			toNotify = append(toNotify, s)
		}
	}

	// Drop tracking for sessions that vanished.
	for id := range t.since {
		if !live[id] {
			delete(t.since, id)
			delete(t.notified, id)
		}
	}
	for id := range t.notified {
		if !live[id] {
			delete(t.notified, id)
		}
	}
	return toNotify
}

// blockedNotifyLoop periodically scans live sessions and pushes a notification
// when one has been waiting for approval past the configured threshold. No-op
// when disabled (threshold 0 or push not configured).
func (s *Server) blockedNotifyLoop(ctx context.Context) {
	if s.cfg.BlockedNotifySeconds <= 0 || !s.push.Enabled() {
		return
	}
	threshold := time.Duration(s.cfg.BlockedNotifySeconds) * time.Second
	tracker := newBlockedTracker()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.log.Info("blocked-session notifications enabled",
		zap.Int("thresholdSeconds", s.cfg.BlockedNotifySeconds))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for _, org := range s.state.AllSessions() {
				for _, sess := range tracker.reconcile(org.Sessions, threshold, now) {
					s.notifyBlocked(org.OrgID, sess)
				}
			}
		}
	}
}

// notifyBlocked pushes a "session blocked" notification to every subscription in
// the org and records an in-app event. Dead subscriptions are pruned.
func (s *Server) notifyBlocked(orgID string, sess protocol.Session) {
	label := sessionDisplayLabel(sess)
	payload := push.Payload{
		Title: "Session blocked",
		Body:  fmt.Sprintf("%s has been waiting for approval for %ds", label, s.cfg.BlockedNotifySeconds),
		URL:   "/sessions/" + sess.ID,
		Tag:   "blocked-" + sess.ID,
	}

	subs, err := s.store.ListPushSubscriptionsForOrg(orgID)
	if err != nil {
		s.log.Warn("list push subscriptions", zap.Error(err))
		return
	}
	sent := 0
	for _, sub := range subs {
		err := s.push.Send(push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, payload)
		var gone *push.ErrGone
		if errors.As(err, &gone) {
			_ = s.store.DeletePushSubscription(sub.Endpoint)
			continue
		}
		if err != nil {
			s.log.Debug("push send failed", zap.Error(err))
			continue
		}
		sent++
	}
	// Surface in the activity timeline too.
	s.emitServerEvent(orgID, sess.MachineID, sess.ID, protocol.EventStatusChanged,
		"Blocked "+fmt.Sprint(s.cfg.BlockedNotifySeconds)+"s · "+label, protocol.SeverityWarn)
	s.log.Info("blocked-session notified", zap.String("session", sess.ID), zap.Int("subs", sent))
}

// sessionDisplayLabel is a short human label for a session (repo/branch or cwd).
func sessionDisplayLabel(s protocol.Session) string {
	if s.Repo != "" {
		if s.Branch != "" {
			return s.Repo + " (" + s.Branch + ")"
		}
		return s.Repo
	}
	if s.CWD != "" {
		return s.CWD
	}
	return s.ID
}
