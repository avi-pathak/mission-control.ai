package server

import (
	"errors"
	"net/http"

	"github.com/avi-pathak/mission-control.ai/internal/push"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

// handlePushVAPIDKey returns the server's VAPID public key so browsers can
// subscribe. Empty when push is not configured.
func (s *Server) handlePushVAPIDKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"publicKey": s.push.PublicKey(),
		"enabled":   s.push.Enabled(),
	})
}

type pushSubscribeReq struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// handlePushSubscribe stores the caller's browser push subscription.
func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	var req pushSubscribeReq
	if err := decodeJSON(r, &req); err != nil || req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "endpoint and keys required")
		return
	}
	if err := s.store.SavePushSubscription(store.PushSubscription{
		OrgID: c.OrgID, UserID: c.UserID, Endpoint: req.Endpoint,
		P256dh: req.Keys.P256dh, Auth: req.Keys.Auth,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pushUnsubscribeReq struct {
	Endpoint string `json:"endpoint"`
}

// handlePushUnsubscribe removes a subscription by endpoint.
func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeReq
	if err := decodeJSON(r, &req); err != nil || req.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "endpoint required")
		return
	}
	if err := s.store.DeletePushSubscription(req.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePushTest sends a test notification to all of the caller's subscriptions.
func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	c := userClaims(r)
	if !s.push.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "push_disabled", "web push is not configured on this server")
		return
	}
	subs, err := s.store.ListPushSubscriptionsForUser(c.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	if len(subs) == 0 {
		writeError(w, http.StatusBadRequest, "no_subscription", "no push subscription for this account; enable notifications first")
		return
	}
	sent := 0
	for _, sub := range subs {
		if err := s.push.Send(push.Subscription{Endpoint: sub.Endpoint, P256dh: sub.P256dh, Auth: sub.Auth}, push.Payload{
			Title: "Test notification",
			Body:  "Notifications are working. You'll be alerted when a session is blocked.",
			URL:   "/",
			Tag:   "mc-test",
		}); err == nil {
			sent++
		} else {
			var gone *push.ErrGone
			if errors.As(err, &gone) {
				_ = s.store.DeletePushSubscription(sub.Endpoint)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent})
}
