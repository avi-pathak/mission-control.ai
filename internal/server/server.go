// Package server wires the HTTP API, WebSocket endpoint, live state and store
// into a runnable Mission Control control plane.
package server

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/avi-pathak/mission-control.ai/internal/auth"
	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/hub"
	"github.com/avi-pathak/mission-control.ai/internal/state"
	"github.com/avi-pathak/mission-control.ai/internal/store"
	"go.uber.org/zap"
)

// agentKeyInfo is the org + machine an agent key maps to.
type agentKeyInfo struct {
	OrgID     string
	MachineID string
}

// Server is the top-level control plane.
type Server struct {
	cfg   config.Server
	log   *zap.Logger
	hub   *hub.Hub
	state *state.Manager
	store *store.Store
	auth  *auth.Manager
	http  *http.Server

	keysMu    sync.RWMutex
	agentKeys map[string]agentKeyInfo // minted agent key -> {org, machine}

	termRouter *terminalRouter
}

// New constructs a Server and its dependencies.
func New(cfg config.Server, log *zap.Logger, st *store.Store) *Server {
	secret := cfg.JWTSecret
	if secret == "" {
		secret = "insecure-dev-secret-change-me"
		log.Warn("JWT_SECRET not set; using an insecure development secret")
	}
	s := &Server{
		cfg:       cfg,
		log:       log,
		hub:       hub.New(log),
		state:     state.New(),
		store:     st,
		auth:      auth.NewManager(secret, 0),
		agentKeys: make(map[string]agentKeyInfo),
		termRouter: newTerminalRouter(),
	}
	s.reloadAgentKeys()
	// Seed per-org event rings with recent history so first snapshots include a
	// timeline. Events come newest-first; feed oldest-last per org.
	if evs, orgs, err := st.AllRecentEvents(1000); err == nil {
		for i := len(evs) - 1; i >= 0; i-- {
			s.state.SeedEvent(orgs[i], evs[i])
		}
	}
	s.bootstrapAdmin()
	s.registerHubHandlers()
	s.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// reloadAgentKeys refreshes the in-memory minted-key set from the store.
func (s *Server) reloadAgentKeys() {
	rows, err := s.store.ActiveAgentKeys()
	if err != nil {
		s.log.Error("load agent keys", zap.Error(err))
		return
	}
	m := make(map[string]agentKeyInfo, len(rows))
	for _, k := range rows {
		m[k.Key] = agentKeyInfo{OrgID: k.OrgID, MachineID: k.MachineID}
	}
	s.keysMu.Lock()
	s.agentKeys = m
	s.keysMu.Unlock()
}

func (s *Server) router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(s.log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-API-Key", "X-MC-Session", "X-MC-Filename"},
		AllowCredentials: false,
	}))

	r.Get("/healthz", s.handleHealth)
	r.Get("/ws", s.handleWS)
	r.Get("/install.sh", s.handleInstallScript)
	r.Get("/install.ps1", s.handleInstallPowershell)
	r.Get("/download/{name}", func(w http.ResponseWriter, req *http.Request) {
		s.handleDownloadBinary(w, req, chi.URLParam(req, "name"))
	})

	// Public auth endpoints.
	r.Post("/api/auth/register", s.handleRegister)
	r.Post("/api/auth/login", s.handleLogin)
	r.Post("/api/auth/accept-invite", s.handleAcceptInvite)

	// Agent-authenticated endpoints (agent key via X-API-Key).
	r.Post("/api/enroll", s.handleEnroll)
	r.Post("/api/publish", s.handlePublish)

	// User (JWT) authenticated, org-scoped endpoints.
	r.Route("/api", func(r chi.Router) {
		r.Use(s.requireUser)
		r.Get("/me", s.handleMe)
		r.Get("/machines", s.handleListMachines)
		r.Delete("/machines/{id}", s.handleDeleteMachine)
		r.Get("/sessions", s.handleListSessions)
		r.Get("/sessions/{id}", s.handleGetSession)
		r.Post("/sessions/{id}/stop", s.handleStopSession)
		r.Post("/sessions/{id}/restart", s.handleRestartSession)
		r.Get("/logs/{id}", s.handleGetLogs)
		r.Get("/metrics/{id}", s.handleGetMetrics)
		r.Get("/events", s.handleListEvents)
		r.Get("/files", s.handleListFiles)
		r.Get("/files/{id}", s.handleDownloadFile)
		r.Get("/enroll-tokens", s.handleListEnrollTokens)
		r.Post("/enroll-tokens", s.handleCreateEnrollToken)
		r.Delete("/enroll-tokens/{token}", s.handleRevokeEnrollToken)
		// Org administration.
		r.Get("/org/users", s.handleListOrgUsers)
		r.Get("/org/invites", s.handleListInvites)
		r.Post("/org/invites", s.handleCreateInvite)
		r.Delete("/org/invites/{token}", s.handleRevokeInvite)
	})

	// Serve the built dashboard SPA (with history fallback) for everything else.
	// Registered last so /api, /ws, /healthz and /install.sh take priority.
	if s.cfg.StaticDir != "" {
		r.NotFound(spaHandler(s.cfg.StaticDir))
		s.log.Info("serving dashboard", zap.String("dir", s.cfg.StaticDir))
	}
	return r
}

// Run starts the hub loop, retention loop and HTTP server, blocking until
// ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	go s.hub.Run()
	go s.retentionLoop(ctx)

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server listening", zap.String("addr", s.cfg.ListenAddr), zap.Bool("tls", s.cfg.TLS.Enabled))
		if s.cfg.TLS.Enabled {
			errCh <- s.http.ListenAndServeTLS(s.cfg.TLS.CertFile, s.cfg.TLS.KeyFile)
		} else {
			errCh <- s.http.ListenAndServe()
		}
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.http.Shutdown(shutCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) retentionLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.cfg.Retention.LogHours > 0 {
				cut := time.Now().Add(-time.Duration(s.cfg.Retention.LogHours) * time.Hour)
				if n, err := s.store.PruneLogs(cut); err == nil && n > 0 {
					s.log.Info("pruned logs", zap.Int64("rows", n))
				}
			}
			if s.cfg.Retention.MetricHours > 0 {
				cut := time.Now().Add(-time.Duration(s.cfg.Retention.MetricHours) * time.Hour)
				if n, err := s.store.PruneMetrics(cut); err == nil && n > 0 {
					s.log.Info("pruned metrics", zap.Int64("rows", n))
				}
			}
			// Expired, never-used enrollment tokens are cleaned up hourly.
			if n, err := s.store.PruneExpiredTokens(time.Now().Add(-time.Hour)); err == nil && n > 0 {
				s.log.Info("pruned enrollment tokens", zap.Int64("rows", n))
			}
			// Activity events share the log retention window.
			if s.cfg.Retention.LogHours > 0 {
				cut := time.Now().Add(-time.Duration(s.cfg.Retention.LogHours) * time.Hour)
				if n, err := s.store.PruneEvents(cut); err == nil && n > 0 {
					s.log.Info("pruned events", zap.Int64("rows", n))
				}
			}
		}
	}
}
