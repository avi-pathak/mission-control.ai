package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/store"
)

const maxPublishBytes = 32 << 20 // 32 MiB

// handlePublish accepts a file upload from an agent (authenticated by its agent
// key via X-API-Key). The file is stored as a blob scoped to the key's org.
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	info, ok := s.authAgentKey(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid agent key")
		return
	}
	name := r.Header.Get("X-MC-Filename")
	if name == "" {
		name = "upload.bin"
	}
	sessionID := r.Header.Get("X-MC-Session")

	body := http.MaxBytesReader(w, r.Body, maxPublishBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds 32 MiB limit")
		return
	}
	sum := sha256.Sum256(data)
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	f, err := s.store.SaveFile(store.PublishedFile{
		OrgID:       info.OrgID,
		MachineID:   info.MachineID,
		SessionID:   sessionID,
		Name:        name,
		Size:        int64(len(data)),
		ContentType: ct,
		SHA256:      hex.EncodeToString(sum[:]),
		Data:        data,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}

	// Surface as an activity event.
	s.emitServerEvent(info.OrgID, info.MachineID, sessionID, protocol.EventFilePublished,
		fmt.Sprintf("Published %s (%d bytes)", name, len(data)), protocol.SeverityInfo)

	writeJSON(w, http.StatusCreated, map[string]any{"id": f.ID, "name": f.Name, "size": f.Size})
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 200
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}
	files, err := s.store.ListFiles(orgID(r), q.Get("session"), q.Get("machine"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.store.GetFile(orgID(r), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	_, _ = w.Write(f.Data)
}
