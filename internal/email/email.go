// Package email provides an optional SMTP sender. When no host is configured
// the sender is disabled and Send is a no-op — callers should still surface any
// invite link directly so the product works with or without email.
package email

import (
	"fmt"
	"net/smtp"
	"strings"
)

// Config holds SMTP settings.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Sender sends mail via SMTP. The zero value / empty-host config is disabled.
type Sender struct {
	cfg Config
}

// New builds a Sender from config.
func New(cfg Config) *Sender { return &Sender{cfg: cfg} }

// Enabled reports whether SMTP is configured.
func (s *Sender) Enabled() bool {
	return s != nil && strings.TrimSpace(s.cfg.Host) != "" && s.cfg.Port != 0 && strings.TrimSpace(s.cfg.From) != ""
}

// Send delivers a simple HTML email. No-op (returns nil) when disabled.
func (s *Sender) Send(to, subject, htmlBody string) error {
	if !s.Enabled() {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	msg := buildMessage(s.cfg.From, to, subject, htmlBody)
	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, msg)
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}
