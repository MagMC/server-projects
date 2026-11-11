// Package api wires HTTP routes to collectors. Routes are registered with
// explicit methods so write endpoints (start/stop/restart, behind auth) can be
// added later without restructuring; see middleware seam below.
package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/magmc/server-projects/dashboard-api/internal/claude"
	"github.com/magmc/server-projects/dashboard-api/internal/collect"
	"github.com/magmc/server-projects/dashboard-api/internal/vpn"
)

type Server struct {
	host      *collect.HostCollector
	docker    *collect.DockerCollector
	dockerErr error
	k3s       *collect.K3sCollector
	k3sErr    error
	claude    []*claude.Controller // one per configured tmux session
	vpn       *vpn.Restarter       // gluetun stack restarts; nil if docker is unreachable
	vpnErr    error
	token     string // control token for claude + vpn routes; empty disables them
}

func NewServer(host *collect.HostCollector, docker *collect.DockerCollector, dockerErr error, k3s *collect.K3sCollector, k3sErr error, cl []*claude.Controller, vr *vpn.Restarter, vrErr error, token string) *Server {
	return &Server{host: host, docker: docker, dockerErr: dockerErr, k3s: k3s, k3sErr: k3sErr, claude: cl, vpn: vr, vpnErr: vrErr, token: token}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Read-only surface. Future: mux.HandleFunc("POST /docker/{id}/restart", auth(s.handleRestart)).
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /host", s.handleHost)
	mux.HandleFunc("GET /docker/containers", s.handleDockerContainers)
	mux.HandleFunc("GET /k3s", s.handleK3s)

	// Claude tmux sessions. Status (of all of them) is public like the rest; a
	// screen or conversation can show secrets and start/stop change state, so
	// those need the control token.
	mux.HandleFunc("GET /claude/status", s.handleClaudeStatus)
	mux.HandleFunc("GET /claude/{session}/screen", s.requireToken(s.handleClaudeScreen))
	mux.HandleFunc("GET /claude/{session}/conversations", s.requireToken(s.handleClaudeConversations))
	mux.HandleFunc("GET /claude/{session}/conversations/{id}", s.requireToken(s.handleClaudeConversation))
	mux.HandleFunc("POST /claude/{session}/start", s.requireToken(s.handleClaudeStart))
	mux.HandleFunc("POST /claude/{session}/stop", s.requireToken(s.handleClaudeStop))

	// gluetun VPN stack: status + restart progress is public; the restart
	// itself (gluetun, then everything on its network) needs the token.
	mux.HandleFunc("GET /vpn", s.handleVPNStatus)
	mux.HandleFunc("POST /vpn/restart", s.requireToken(s.handleVPNRestart))
	return logRequests(mux)
}

// requireToken checks "Authorization: Bearer <token>" in constant time. With
// no token configured the route is disabled rather than open.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			writeErr(w, http.StatusServiceUnavailable, "control disabled: CLAUDE_CONTROL_TOKEN not set")
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, r)
	}
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
