// Package config reads runtime configuration from the environment. Defaults
// target local dev (real /proc, /sys); the container overrides them to point at
// host mounts (/host/proc, /host/sys, ...).
package config

import (
	"os"
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

	// Claude Code tmux session on the host.
	TmuxSocket    string // host tmux server socket, seen through the / mount
	ClaudeSession string
	ClaudeWorkdir string // where a freshly created session starts
	ClaudeCmd     string // typed into the session's shell on start
	ControlToken  string // bearer token for claude screen/start/stop; empty disables them
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

		TmuxSocket:    env("TMUX_SOCKET", "/tmp/tmux-1000/default"),
		ClaudeSession: env("CLAUDE_SESSION", "claude"),
		ClaudeWorkdir: os.Getenv("CLAUDE_WORKDIR"),
		ClaudeCmd:     env("CLAUDE_CMD", "claude"),
		ControlToken:  os.Getenv("CLAUDE_CONTROL_TOKEN"),
	}
}
