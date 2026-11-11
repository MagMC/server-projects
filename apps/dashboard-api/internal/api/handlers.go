package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/claude"
	"github.com/magmc/server-projects/dashboard-api/internal/model"
	"github.com/magmc/server-projects/dashboard-api/internal/vpn"
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
	out := make([]model.ClaudeStatus, 0, len(s.claude))
	for _, c := range s.claude {
		st := c.Status(ctx)
		st.ControlEnabled = s.token != ""
		out = append(out, st)
	}
	writeJSON(w, http.StatusOK, out)
}

// claudeSession resolves the {session} path value to its controller, replying
// 404 (and returning nil) if no such session is configured.
func (s *Server) claudeSession(w http.ResponseWriter, r *http.Request) *claude.Controller {
	name := r.PathValue("session")
	for _, c := range s.claude {
		if c.Session() == name {
			return c
		}
	}
	writeErr(w, http.StatusNotFound, "unknown session: "+name)
	return nil
}

// screenHistory is how many scrollback lines the session viewer gets.
const screenHistory = 300

func (s *Server) handleClaudeScreen(w http.ResponseWriter, r *http.Request) {
	c := s.claudeSession(w, r)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	text, err := c.Capture(ctx, screenHistory)
	if err != nil {
		writeClaudeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model.ClaudeScreen{Text: text, Width: c.Status(ctx).Width})
}

func (s *Server) handleClaudeConversations(w http.ResponseWriter, r *http.Request) {
	c := s.claudeSession(w, r)
	if c == nil {
		return
	}
	writeJSON(w, http.StatusOK, c.Conversations())
}

func (s *Server) handleClaudeConversation(w http.ResponseWriter, r *http.Request) {
	c := s.claudeSession(w, r)
	if c == nil {
		return
	}
	conv, err := c.Conversation(r.PathValue("id"))
	if err != nil {
		writeClaudeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

func (s *Server) handleClaudeStart(w http.ResponseWriter, r *http.Request) {
	s.claudeAction(w, r, (*claude.Controller).Start)
}

func (s *Server) handleClaudeStop(w http.ResponseWriter, r *http.Request) {
	s.claudeAction(w, r, (*claude.Controller).Stop)
}

// claudeAction runs start/stop on the requested session and replies with the
// resulting status.
func (s *Server) claudeAction(w http.ResponseWriter, r *http.Request, act func(*claude.Controller, context.Context) error) {
	c := s.claudeSession(w, r)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	if err := act(c, ctx); err != nil {
		writeClaudeErr(w, err)
		return
	}
	st := c.Status(ctx)
	st.ControlEnabled = true
	writeJSON(w, http.StatusOK, st)
}

func writeClaudeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, claude.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, claude.ErrNotFound):
		status = http.StatusNotFound
	}
	writeErr(w, status, err.Error())
}

func (s *Server) handleVPNStatus(w http.ResponseWriter, r *http.Request) {
	if s.vpnErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "docker unavailable: "+s.vpnErr.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	st := s.vpn.Status(ctx)
	st.ControlEnabled = s.token != ""
	writeJSON(w, http.StatusOK, st)
}

// handleVPNRestart kicks off the restart and returns at once (it takes minutes,
// far past the proxy timeout); the client follows progress via GET /vpn.
func (s *Server) handleVPNRestart(w http.ResponseWriter, r *http.Request) {
	if s.vpnErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "docker unavailable: "+s.vpnErr.Error())
		return
	}
	if err := s.vpn.Start(); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, vpn.ErrBusy) {
			status = http.StatusConflict
		}
		writeErr(w, status, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	st := s.vpn.Status(ctx)
	st.ControlEnabled = true
	writeJSON(w, http.StatusAccepted, st)
}
