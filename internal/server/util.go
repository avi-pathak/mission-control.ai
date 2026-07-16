package server

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func zapErr(err error) zap.Field { return zap.Error(err) }
func zapStr(k, v string) zap.Field { return zap.String(k, v) }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify produces a URL-safe slug from a name.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "org"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// shortID returns a short random suffix for slug uniqueness.
func shortID() string {
	return uuid.NewString()[:8]
}
