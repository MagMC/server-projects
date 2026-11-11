// Package config reads runtime configuration from the environment. Defaults
// target local dev (real /proc, /sys); the container overrides them to point at
// host mounts (/host/proc, /host/sys, ...).
package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port       string
	HostProc   string
	HostSys    string
	DiskMounts []string
	DockerHost string
	Kubeconfig string
	K3sServer  string // overrides the kubeconfig cluster server URL when set

	// Claude Code tmux sessions on the host.
	TmuxSocket     string // host tmux server socket, seen through the / mount
	ClaudeSessions []ClaudeSession
	ControlToken   string // bearer token for the protected claude routes; empty disables them
	// Where Claude Code keeps its transcripts, seen through the / mount. Each
	// session's conversations are found here by its workdir.
	ClaudeProjectsDir string

	// The gluetun VPN container; the dashboard restarts it and then every
	// container using its network.
	GluetunContainer string
}

// ClaudeSession is one tmux session the dashboard shows and controls.
type ClaudeSession struct {
	Name    string // tmux session name
	Workdir string // where a freshly created session starts
	Cmd     string // typed into the session's shell on start
}

// parseSessions reads "name|workdir|command" entries separated by ";" or
// newlines. Workdir may be empty; command defaults to "claude".
func parseSessions(s string) []ClaudeSession {
	var out []ClaudeSession
	for _, entry := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' }) {
		f := strings.SplitN(entry, "|", 3)
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		if f[0] == "" {
			continue
		}
		sess := ClaudeSession{Name: f[0], Cmd: "claude"}
		if len(f) > 1 {
			sess.Workdir = f[1]
		}
		if len(f) > 2 && f[2] != "" {
			sess.Cmd = f[2]
		}
		out = append(out, sess)
	}
	return out
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

func Load() Config {
	mounts := strings.Split(env("DISK_MOUNTS", "/"), ",")
	clean := mounts[:0]
	for _, m := range mounts {
		if m = strings.TrimSpace(m); m != "" {
			clean = append(clean, m)
		}
	}
	return Config{
		Port:       env("PORT", "3001"),
		HostProc:   env("HOST_PROC", "/proc"),
		HostSys:    env("HOST_SYS", "/sys"),
		DiskMounts: clean,
		DockerHost: env("DOCKER_HOST", "unix:///var/run/docker.sock"),
		Kubeconfig: env("KUBECONFIG", "/etc/rancher/k3s/k3s.yaml"),
		K3sServer:  os.Getenv("K3S_SERVER"),

		TmuxSocket:     env("TMUX_SOCKET", "/tmp/tmux-1000/default"),
		ClaudeSessions: parseSessions(env("CLAUDE_SESSIONS", "claude")),
		ControlToken:   os.Getenv("CLAUDE_CONTROL_TOKEN"),

		ClaudeProjectsDir: env("CLAUDE_PROJECTS_DIR", defaultProjectsDir()),

		GluetunContainer: env("GLUETUN_CONTAINER", "gluetun"),
	}
}
