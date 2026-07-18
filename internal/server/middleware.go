package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/avi-pathak/mission-control.ai/internal/auth"
	"go.uber.org/zap"
)

type ctxKey int

const claimsKey ctxKey = iota

// userClaims pulls the authenticated user's claims from the request context.
func userClaims(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

// orgID is a convenience for the authenticated user's org.
func orgID(r *http.Request) string {
	if c := userClaims(r); c != nil {
		return c.OrgID
	}
	return ""
}

// bearer extracts a bearer token from the Authorization header or `token` query.
func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}

// requireSuperadmin gates a route to platform-level superadmins. Must be
// chained after requireUser (reads claims from context).
func (s *Server) requireSuperadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := userClaims(r)
		if c == nil || !c.PlatformAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "platform admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireUser validates the JWT and injects claims into the request context.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := s.auth.Verify(bearer(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing token")
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authAgentKey resolves an agent key (X-API-Key or ?key=) to its org + machine.
func (s *Server) authAgentKey(r *http.Request) (agentKeyInfo, bool) {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	s.keysMu.RLock()
	info, ok := s.agentKeys[key]
	s.keysMu.RUnlock()
	return info, ok
}

// lookupAgentKey resolves a raw key string to its org + machine.
func (s *Server) lookupAgentKey(key string) (agentKeyInfo, bool) {
	s.keysMu.RLock()
	info, ok := s.agentKeys[key]
	s.keysMu.RUnlock()
	return info, ok
}

func requestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Debug("http",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("took", time.Since(start)),
			)
		})
	}
}
