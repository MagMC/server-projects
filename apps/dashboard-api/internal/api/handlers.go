package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
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
