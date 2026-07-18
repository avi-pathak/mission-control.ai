// Package push sends Web Push notifications (RFC 8291/8292) via VAPID. When no
// VAPID keys are configured the sender is disabled and Send is a no-op.
package push

import (
	"encoding/json"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Config holds VAPID settings.
type Config struct {
	VAPIDPublic  string
	VAPIDPrivate string
	Subject      string // mailto: or https: contact
}

// Subscription is a browser push subscription.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Payload is the notification body delivered to the service worker.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// Sender delivers push messages. The zero value / empty keys is disabled.
type Sender struct {
	cfg Config
}

// New builds a Sender.
func New(cfg Config) *Sender { return &Sender{cfg: cfg} }

// Enabled reports whether VAPID keys are configured.
func (s *Sender) Enabled() bool {
	return s != nil && s.cfg.VAPIDPublic != "" && s.cfg.VAPIDPrivate != ""
}

// PublicKey returns the VAPID public key (for the browser to subscribe).
func (s *Sender) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.cfg.VAPIDPublic
}

// ErrGone indicates the push service reported the subscription as expired/invalid
// (HTTP 404/410); the caller should delete it.
type ErrGone struct{ Endpoint string }

func (e *ErrGone) Error() string { return "push subscription gone: " + e.Endpoint }

// Send delivers a payload to one subscription. Returns *ErrGone if the
// subscription is dead. No-op (nil) when disabled.
func (s *Sender) Send(sub Subscription, p Payload) error {
	if !s.Enabled() {
		return nil
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotification(body, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      s.cfg.Subject,
		VAPIDPublicKey:  s.cfg.VAPIDPublic,
		VAPIDPrivateKey: s.cfg.VAPIDPrivate,
		TTL:             60,
		// High urgency so the push service wakes the browser and delivers
		// immediately (a blocked session needs prompt attention).
		Urgency: webpush.UrgencyHigh,
		// Cap the record size. The library otherwise pads every payload to 4096
		// bytes; a tight bound avoids unnecessary over-padding.
		RecordSize: 2048,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return &ErrGone{Endpoint: sub.Endpoint}
	}
	return nil
}
