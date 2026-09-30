package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/claude"
	"github.com/magmc/server-projects/dashboard-api/internal/model"
)

const requestTimeout = 8 * time.Second

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleHost(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.host.Collect())
}

func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	if s.dockerErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "docker unavailable: "+s.dockerErr.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	containers, err := s.docker.Collect(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, containers)
}

func (s *Server) handleK3s(w http.ResponseWriter, r *http.Request) {
	if s.k3sErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "k3s unavailable: "+s.k3sErr.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	data, err := s.k3s.Collect(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleClaudeStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	st := s.claude.Status(ctx)
	st.ControlEnabled = s.token != ""
	writeJSON(w, http.StatusOK, st)
}

// screenHistory is how many scrollback lines the session viewer gets.
const screenHistory = 300

func (s *Server) handleClaudeScreen(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	text, err := s.claude.Capture(ctx, screenHistory)
	if err != nil {
		writeClaudeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ClaudeScreen{Text: text, Width: s.claude.Status(ctx).Width})
}

func (s *Server) handleClaudeStart(w http.ResponseWriter, r *http.Request) {
	s.claudeAction(w, r, s.claude.Start)
}

func (s *Server) handleClaudeStop(w http.ResponseWriter, r *http.Request) {
	s.claudeAction(w, r, s.claude.Stop)
}

// claudeAction runs start/stop and replies with the resulting status.
func (s *Server) claudeAction(w http.ResponseWriter, r *http.Request, act func(context.Context) error) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	if err := act(ctx); err != nil {
		writeClaudeErr(w, err)
		return
	}
	st := s.claude.Status(ctx)
	st.ControlEnabled = true
	writeJSON(w, http.StatusOK, st)
}

func writeClaudeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	if errors.Is(err, claude.ErrConflict) {
		status = http.StatusConflict
	}
	writeErr(w, status, err.Error())
}
