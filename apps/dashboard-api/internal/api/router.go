// Package api wires HTTP routes to collectors. Routes are registered with
// explicit methods so write endpoints (start/stop/restart, behind auth) can be
// added later without restructuring; see middleware seam below.
package api

import (
	"log"
	"net/http"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/collect"
)

type Server struct {
	host      *collect.HostCollector
	docker    *collect.DockerCollector
	dockerErr error
	k3s       *collect.K3sCollector
	k3sErr    error
}

func NewServer(host *collect.HostCollector, docker *collect.DockerCollector, dockerErr error, k3s *collect.K3sCollector, k3sErr error) *Server {
	return &Server{host: host, docker: docker, dockerErr: dockerErr, k3s: k3s, k3sErr: k3sErr}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Read-only surface. Future: mux.HandleFunc("POST /docker/{id}/restart", auth(s.handleRestart)).
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /host", s.handleHost)
	mux.HandleFunc("GET /docker/containers", s.handleDockerContainers)
	mux.HandleFunc("GET /k3s", s.handleK3s)
	return logRequests(mux)
}

// logRequests is the outermost middleware. Auth middleware for write routes
// would compose here (e.g. return logRequests(auth(mux))).
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
